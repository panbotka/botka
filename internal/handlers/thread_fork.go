package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// maxForkMessageCount caps how many messages a single fork operation will
// copy. Forking a runaway thread (10k+ messages) would lock the messages
// table and produce an unusable copy; surface a clear error instead.
const maxForkMessageCount = 1000

type ForkThreadRequest struct {
	FromMessageID int64  `json:"from_message_id"`
	NewTitle      string `json:"new_title,omitempty"`
}

// Fork creates a new thread copying the source thread's settings, tags,
// sources, and all messages up to and including from_message_id.
// The new thread starts with no Claude session and no branch selections.
func (h *ThreadHandler) Fork(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req ForkThreadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FromMessageID <= 0 {
		respondError(c, http.StatusBadRequest, "from_message_id is required")
		return
	}
	if msg := validateMaxLength("new_title", req.NewTitle, maxTitleLength); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	newThread, status, err := ForkThread(h.db, id, req.FromMessageID, req.NewTitle)
	if err != nil {
		respondError(c, status, err.Error())
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": newThread})
}

// ListForks returns threads that were forked from this thread.
func (h *ThreadHandler) ListForks(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var forks []models.Thread
	if err := h.db.Where("parent_thread_id = ?", id).
		Order("created_at DESC").
		Find(&forks).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list forks")
		return
	}

	if forks == nil {
		forks = []models.Thread{}
	}
	respondList(c, forks, int64(len(forks)))
}

// ForkThread performs the deep copy in a single transaction. It returns the
// new thread, an HTTP status hint for error mapping, and the error itself.
// Used by both the HTTP handler and the MCP tool so the copy logic stays in
// one place.
func ForkThread(db *gorm.DB, sourceID, fromMessageID int64, newTitle string) (*models.Thread, int, error) {
	// Resolve source thread first so we have its title for the default new title.
	var source models.Thread
	if err := db.First(&source, sourceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, http.StatusNotFound, errors.New("thread not found")
		}
		return nil, http.StatusInternalServerError, fmt.Errorf("load source thread: %w", err)
	}

	// Verify the fork point belongs to the source thread (and is not soft-deleted).
	var forkMsg models.Message
	if err := db.Where("id = ? AND thread_id = ?", fromMessageID, sourceID).
		First(&forkMsg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, http.StatusBadRequest, errors.New("from_message_id not found in this thread")
		}
		return nil, http.StatusInternalServerError, fmt.Errorf("load fork message: %w", err)
	}

	// Build the message chain from the root to the fork point by walking
	// parent_id upward. This guarantees a coherent slice even when the source
	// thread has multiple branches — we only follow the path that leads to
	// the chosen message.
	chain, err := messageChainTo(db, sourceID, fromMessageID)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("trace message chain: %w", err)
	}
	if len(chain) > maxForkMessageCount {
		return nil, http.StatusBadRequest,
			fmt.Errorf("fork would copy %d messages (limit %d); choose an earlier fork point",
				len(chain), maxForkMessageCount)
	}

	title := newTitle
	if title == "" {
		title = source.Title + " (fork)"
		if len(title) > maxTitleLength {
			title = title[:maxTitleLength]
		}
	}

	newThread := models.Thread{
		Title:               title,
		Model:               source.Model,
		SystemPrompt:        source.SystemPrompt,
		CustomContext:       source.CustomContext,
		PersonaID:           source.PersonaID,
		PersonaName:         source.PersonaName,
		ProjectID:           source.ProjectID,
		FolderID:            source.FolderID,
		Color:               source.Color,
		ParentThreadID:      &sourceID,
		ForkedFromMessageID: &fromMessageID,
		// Intentionally NOT copied: Pinned, Archived, ClaudeSessionID — fork starts fresh.
	}

	txErr := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&newThread).Error; err != nil {
			return fmt.Errorf("create thread: %w", err)
		}

		// Copy tag assignments via the junction table.
		if err := tx.Exec(`INSERT INTO thread_tags (thread_id, tag_id)
			SELECT ?, tag_id FROM thread_tags WHERE thread_id = ?`,
			newThread.ID, sourceID).Error; err != nil {
			return fmt.Errorf("copy tags: %w", err)
		}

		// Copy URL sources, preserving position. Created/updated timestamps
		// are reset by GORM on Create.
		var sources []models.ThreadSource
		if err := tx.Where("thread_id = ?", sourceID).Order("position ASC").Find(&sources).Error; err != nil {
			return fmt.Errorf("load sources: %w", err)
		}
		for i := range sources {
			newSrc := models.ThreadSource{
				ThreadID: newThread.ID,
				URL:      sources[i].URL,
				Label:    sources[i].Label,
				Position: sources[i].Position,
			}
			if err := tx.Create(&newSrc).Error; err != nil {
				return fmt.Errorf("create source: %w", err)
			}
		}

		// Copy messages in chain order, remapping parent_id to the new IDs.
		// We rebuild the linear path: the new thread has no branches.
		idMap := make(map[int64]int64, len(chain))
		var prevNewID *int64
		baseTime := time.Now()
		for i := range chain {
			old := &chain[i]
			var parentID *int64
			if prevNewID != nil {
				p := *prevNewID
				parentID = &p
			}
			newMsg := models.Message{
				ThreadID:           newThread.ID,
				Role:               old.Role,
				Content:            old.Content,
				ParentID:           parentID,
				Thinking:           old.Thinking,
				ThinkingDurationMs: old.ThinkingDurationMs,
				PromptTokens:       old.PromptTokens,
				CompletionTokens:   old.CompletionTokens,
				CostUSD:            old.CostUSD,
				ToolCalls:          old.ToolCalls,
				Hidden:             old.Hidden,
				// Preserve relative ordering by spacing CreatedAt out at
				// nanosecond granularity. Tests and the active-path walker
				// order by created_at then id so monotonic spacing is enough.
				CreatedAt: baseTime.Add(time.Duration(i) * time.Nanosecond),
			}
			if err := tx.Create(&newMsg).Error; err != nil {
				return fmt.Errorf("create message: %w", err)
			}
			idMap[old.ID] = newMsg.ID
			prevNewID = &newMsg.ID
		}

		// Copy attachments by reference: same StoredName, same OriginalName.
		// The underlying file on disk is shared. There is no automated GC of
		// upload files in the codebase today, so this is safe; if GC is added
		// later it must check for any non-soft-deleted attachment row that
		// references the StoredName before deleting the file.
		if len(chain) > 0 {
			oldIDs := make([]int64, 0, len(chain))
			for i := range chain {
				oldIDs = append(oldIDs, chain[i].ID)
			}
			var oldAtts []models.Attachment
			if err := tx.Where("message_id IN ?", oldIDs).Find(&oldAtts).Error; err != nil {
				return fmt.Errorf("load attachments: %w", err)
			}
			for i := range oldAtts {
				newMsgID, ok := idMap[oldAtts[i].MessageID]
				if !ok {
					continue
				}
				newAtt := models.Attachment{
					MessageID:    newMsgID,
					StoredName:   oldAtts[i].StoredName,
					OriginalName: oldAtts[i].OriginalName,
					MimeType:     oldAtts[i].MimeType,
					Size:         oldAtts[i].Size,
				}
				if err := tx.Create(&newAtt).Error; err != nil {
					return fmt.Errorf("create attachment: %w", err)
				}
			}
		}

		return nil
	})
	if txErr != nil {
		return nil, http.StatusInternalServerError, txErr
	}

	return &newThread, http.StatusCreated, nil
}

// messageChainTo returns the messages from the root to fromMessageID in
// chronological order, following parent_id upward from the chosen message.
// Soft-deleted messages are excluded — GORM applies the deleted_at filter.
func messageChainTo(db *gorm.DB, threadID, fromMessageID int64) ([]models.Message, error) {
	// Load all live messages in one query and walk the chain in memory. The
	// max-cap check downstream protects against pathological cases.
	var all []models.Message
	if err := db.Where("thread_id = ?", threadID).Find(&all).Error; err != nil {
		return nil, err
	}
	byID := make(map[int64]models.Message, len(all))
	for _, m := range all {
		byID[m.ID] = m
	}
	var reversed []models.Message
	cur, ok := byID[fromMessageID]
	if !ok {
		return nil, errors.New("fork message not found")
	}
	for {
		reversed = append(reversed, cur)
		if cur.ParentID == nil {
			break
		}
		next, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		cur = next
		// Defensive bound: a corrupt parent loop shouldn't hang the request.
		if len(reversed) > maxForkMessageCount+1 {
			return nil, fmt.Errorf("message chain exceeds %d (possible cycle)", maxForkMessageCount)
		}
	}
	// Reverse into chronological order.
	chain := make([]models.Message, len(reversed))
	for i := range reversed {
		chain[len(reversed)-1-i] = reversed[i]
	}
	return chain, nil
}
