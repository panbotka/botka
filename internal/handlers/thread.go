package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/claude"
	"botka/internal/models"
)

// ThreadHandler handles HTTP requests for thread resources.
type ThreadHandler struct {
	db              *gorm.DB
	defaultModel    string
	availableModels []string
}

// NewThreadHandler creates a new ThreadHandler with the given dependencies.
func NewThreadHandler(db *gorm.DB, defaultModel string, availableModels []string) *ThreadHandler {
	return &ThreadHandler{
		db:              db,
		defaultModel:    defaultModel,
		availableModels: availableModels,
	}
}

// RegisterThreadRoutes attaches thread endpoints to the given router group.
func RegisterThreadRoutes(rg *gin.RouterGroup, h *ThreadHandler) {
	rg.GET("/threads", h.List)
	rg.POST("/threads", h.Create)
	rg.GET("/threads/:id", h.GetByID)
	rg.PUT("/threads/:id", h.Rename)
	rg.DELETE("/threads/:id", h.Delete)
	rg.PUT("/threads/:id/pin", h.Pin)
	rg.DELETE("/threads/:id/pin", h.Unpin)
	rg.PUT("/threads/:id/archive", h.Archive)
	rg.DELETE("/threads/:id/archive", h.Unarchive)
	rg.PUT("/threads/:id/model", h.UpdateModel)
	rg.PUT("/threads/:id/project", h.SetProject)
	rg.GET("/threads/:id/usage", h.Usage)
	rg.DELETE("/threads/:id/messages", h.ClearMessages)
	rg.PUT("/threads/:id/tags", h.SetTags)
}

// threadListRow is used for the list query which includes a last message preview.
type threadListRow struct {
	models.Thread
	LastMessagePreview *string    `json:"last_message_preview"`
	LastMessageAt      *time.Time `json:"last_message_at"`
}

// List returns all threads, optionally including archived ones.
func (h *ThreadHandler) List(c *gin.Context) {
	includeArchived := c.Query("archived") == "true"

	var rows []threadListRow
	query := h.db.Table("threads t").
		Select(`t.*, lm.content AS last_message_preview, lm.created_at AS last_message_at`).
		Joins(`LEFT JOIN LATERAL (
			SELECT LEFT(content, 150) AS content, created_at
			FROM messages WHERE thread_id = t.id
			ORDER BY created_at DESC LIMIT 1
		) lm ON true`)

	if !includeArchived {
		query = query.Where("t.archived = ?", false)
	}
	query = query.Order("t.pinned DESC, COALESCE(lm.created_at, t.created_at) DESC")

	if err := query.Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list threads")
		return
	}

	if rows == nil {
		rows = []threadListRow{}
	}

	// Load tags for all threads
	if len(rows) > 0 {
		threadIDs := make([]int64, len(rows))
		for i, r := range rows {
			threadIDs[i] = r.ID
		}

		type threadTagRow struct {
			ThreadID int64
			models.Tag
		}
		var tagRows []threadTagRow
		h.db.Raw(`SELECT tt.thread_id, t.* FROM thread_tags tt
			JOIN tags t ON t.id = tt.tag_id
			WHERE tt.thread_id IN ?`, threadIDs).Scan(&tagRows)

		tagsByThread := make(map[int64][]models.Tag)
		for _, tr := range tagRows {
			tagsByThread[tr.ThreadID] = append(tagsByThread[tr.ThreadID], tr.Tag)
		}
		for i := range rows {
			if tags, ok := tagsByThread[rows[i].ID]; ok {
				rows[i].Tags = tags
			}
		}
	}

	respondList(c, rows, int64(len(rows)))
}

type createThreadRequest struct {
	PersonaID *int64     `json:"persona_id,omitempty"`
	Model     string     `json:"model,omitempty"`
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
}

// Create creates a new thread, optionally with a persona or project.
func (h *ThreadHandler) Create(c *gin.Context) {
	var req createThreadRequest
	_ = c.ShouldBindJSON(&req)

	thread := models.Thread{
		Title: "New Chat",
	}

	if req.PersonaID != nil {
		var persona models.Persona
		if err := h.db.First(&persona, *req.PersonaID).Error; err != nil {
			respondError(c, http.StatusBadRequest, "persona not found")
			return
		}
		aiModel := h.defaultModel
		if persona.DefaultModel != nil && *persona.DefaultModel != "" {
			aiModel = *persona.DefaultModel
		}
		thread.Model = &aiModel
		thread.Title = persona.Name
		thread.SystemPrompt = persona.SystemPrompt
		thread.PersonaID = &persona.ID
		thread.PersonaName = persona.Name
	} else {
		model := req.Model
		if model == "" {
			model = h.defaultModel
		}
		thread.Model = &model
	}

	if req.ProjectID != nil {
		thread.ProjectID = req.ProjectID
	}

	if err := h.db.Create(&thread).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create thread")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": thread})
}

// GetByID returns a thread with its active message path and fork points.
func (h *ThreadHandler) GetByID(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	messages, forkPoints, err := getActivePath(h.db, id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to load messages")
		return
	}
	if messages == nil {
		messages = []models.Message{}
	}

	// Load attachments for path messages
	if len(messages) > 0 {
		msgIDs := make([]int64, len(messages))
		for i, m := range messages {
			msgIDs[i] = m.ID
		}
		var attachments []models.Attachment
		h.db.Where("message_id IN ?", msgIDs).Find(&attachments)

		attachByMsg := make(map[int64][]models.Attachment)
		for _, a := range attachments {
			attachByMsg[a.MessageID] = append(attachByMsg[a.MessageID], a)
		}
		for i := range messages {
			if atts, ok := attachByMsg[messages[i].ID]; ok {
				messages[i].Attachments = atts
			}
		}
	}

	// Convert fork_points keys to strings for JSON
	forkPointsJSON := make(map[string]models.ForkPoint)
	for msgID, fp := range forkPoints {
		forkPointsJSON[fmt.Sprintf("%d", msgID)] = fp
	}

	respondOK(c, gin.H{
		"thread":      thread,
		"messages":    messages,
		"fork_points": forkPointsJSON,
	})
}

type renameRequest struct {
	Title string `json:"title"`
}

// Rename updates a thread's title.
func (h *ThreadHandler) Rename(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req renameRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" {
		respondError(c, http.StatusBadRequest, "title is required")
		return
	}

	if err := h.db.Model(&models.Thread{}).Where("id = ?", id).Updates(map[string]interface{}{
		"title":      req.Title,
		"updated_at": time.Now(),
	}).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to rename thread")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}

// Delete removes a thread and all associated data (CASCADE handles messages).
func (h *ThreadHandler) Delete(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	claude.Pool.Evict(id)

	// Delete branch_selections, messages (with attachments), thread_tags, then thread
	tx := h.db.Begin()
	tx.Where("thread_id = ?", id).Delete(&models.BranchSelection{})
	tx.Where("message_id IN (SELECT id FROM messages WHERE thread_id = ?)", id).Delete(&models.Attachment{})
	tx.Where("thread_id = ?", id).Delete(&models.Message{})
	tx.Exec("DELETE FROM thread_tags WHERE thread_id = ?", id)
	if err := tx.Delete(&models.Thread{}, id).Error; err != nil {
		tx.Rollback()
		respondError(c, http.StatusInternalServerError, "failed to delete thread")
		return
	}
	tx.Commit()

	respondOK(c, gin.H{"status": "ok"})
}

// Pin marks a thread as pinned.
func (h *ThreadHandler) Pin(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var count int64
	h.db.Model(&models.Thread{}).Where("pinned = ?", true).Count(&count)
	if count >= 10 {
		respondError(c, http.StatusConflict, "maximum of 10 pinned threads")
		return
	}

	h.db.Model(&models.Thread{}).Where("id = ?", id).Updates(map[string]interface{}{
		"pinned":     true,
		"updated_at": time.Now(),
	})

	respondOK(c, gin.H{"status": "ok"})
}

// Unpin removes the pinned flag from a thread.
func (h *ThreadHandler) Unpin(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	h.db.Model(&models.Thread{}).Where("id = ?", id).Updates(map[string]interface{}{
		"pinned":     false,
		"updated_at": time.Now(),
	})

	respondOK(c, gin.H{"status": "ok"})
}

// Archive marks a thread as archived (also unpins it).
func (h *ThreadHandler) Archive(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	h.db.Model(&models.Thread{}).Where("id = ?", id).Updates(map[string]interface{}{
		"pinned":     false,
		"archived":   true,
		"updated_at": time.Now(),
	})

	respondOK(c, gin.H{"status": "ok"})
}

// Unarchive removes the archived flag from a thread.
func (h *ThreadHandler) Unarchive(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	h.db.Model(&models.Thread{}).Where("id = ?", id).Updates(map[string]interface{}{
		"archived":   false,
		"updated_at": time.Now(),
	})

	respondOK(c, gin.H{"status": "ok"})
}

type updateModelRequest struct {
	Model string `json:"model"`
}

// UpdateModel changes the AI model for a thread.
func (h *ThreadHandler) UpdateModel(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req updateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	claude.Pool.Evict(id)
	h.db.Model(&models.Thread{}).Where("id = ?", id).Updates(map[string]interface{}{
		"model":      req.Model,
		"updated_at": time.Now(),
	})

	respondOK(c, gin.H{"status": "ok"})
}

type setProjectRequest struct {
	ProjectID *uuid.UUID `json:"project_id"`
}

// SetProject assigns or clears the project for a thread.
func (h *ThreadHandler) SetProject(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req setProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	claude.Pool.Evict(id)
	// Clear session ID — the old session was created for a different directory/context
	h.db.Model(&models.Thread{}).Where("id = ?", id).Updates(map[string]interface{}{
		"project_id":        req.ProjectID,
		"claude_session_id": nil,
		"updated_at":        time.Now(),
	})

	respondOK(c, gin.H{"status": "ok"})
}

// Usage returns token usage statistics for a thread.
func (h *ThreadHandler) Usage(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var result struct {
		PromptTokens     int64 `json:"total_prompt_tokens"`
		CompletionTokens int64 `json:"total_completion_tokens"`
		MessageCount     int64 `json:"message_count"`
	}
	h.db.Raw(`SELECT COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
		COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
		COUNT(*) AS message_count FROM messages WHERE thread_id = ?`, id).Scan(&result)

	respondOK(c, gin.H{
		"thread_id":               id,
		"total_prompt_tokens":     result.PromptTokens,
		"total_completion_tokens": result.CompletionTokens,
		"total_tokens":            result.PromptTokens + result.CompletionTokens,
		"message_count":           result.MessageCount,
	})
}

// ClearMessages deletes all messages in a thread.
func (h *ThreadHandler) ClearMessages(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	if err := h.db.First(&models.Thread{}, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	tx := h.db.Begin()
	tx.Where("thread_id = ?", id).Delete(&models.BranchSelection{})
	tx.Where("message_id IN (SELECT id FROM messages WHERE thread_id = ?)", id).Delete(&models.Attachment{})
	if err := tx.Where("thread_id = ?", id).Delete(&models.Message{}).Error; err != nil {
		tx.Rollback()
		respondError(c, http.StatusInternalServerError, "failed to clear messages")
		return
	}
	tx.Commit()

	// Clear session since messages are gone
	claude.Pool.Evict(id)
	h.db.Model(&models.Thread{}).Where("id = ?", id).Update("claude_session_id", nil)

	respondOK(c, gin.H{"status": "ok"})
}

type setTagsRequest struct {
	TagIDs []int64 `json:"tag_ids"`
}

// SetTags replaces the tags for a thread.
func (h *ThreadHandler) SetTags(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req setTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	tx := h.db.Begin()
	// Remove existing tags
	tx.Exec("DELETE FROM thread_tags WHERE thread_id = ?", id)
	// Insert new tags
	for _, tagID := range req.TagIDs {
		tx.Exec("INSERT INTO thread_tags (thread_id, tag_id) VALUES (?, ?)", id, tagID)
	}
	if err := tx.Commit().Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to set tags")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}

// --- Helper functions ---

// paramInt64 parses a route parameter as int64.
func paramInt64(c *gin.Context, name string) (int64, error) {
	v := c.Param(name)
	return strconv.ParseInt(v, 10, 64)
}

// getActivePath returns messages along the active branch path for a thread.
// At each fork point (message with multiple children), it follows the selected branch
// or defaults to the most recently created child.
func getActivePath(db *gorm.DB, threadID int64) ([]models.Message, map[int64]models.ForkPoint, error) {
	var allMessages []models.Message
	if err := db.Where("thread_id = ?", threadID).
		Order("created_at ASC, id ASC").
		Find(&allMessages).Error; err != nil {
		return nil, nil, err
	}
	if len(allMessages) == 0 {
		return []models.Message{}, nil, nil
	}

	// Load branch selections
	var selections []models.BranchSelection
	db.Where("thread_id = ?", threadID).Find(&selections)
	selMap := make(map[int64]int64) // fork_message_id -> selected_child_id
	for _, s := range selections {
		selMap[s.ForkMessageID] = s.SelectedChildID
	}

	// Build children map
	childrenMap := make(map[int64][]models.Message)
	var roots []models.Message

	for _, m := range allMessages {
		if m.ParentID == nil {
			roots = append(roots, m)
		} else {
			childrenMap[*m.ParentID] = append(childrenMap[*m.ParentID], m)
		}
	}

	// Identify fork points
	forkPoints := make(map[int64]models.ForkPoint)
	for parentID, children := range childrenMap {
		if len(children) > 1 {
			fp := models.ForkPoint{}
			for _, child := range children {
				preview := child.Content
				if len(preview) > 80 {
					preview = preview[:80] + "..."
				}
				fp.Children = append(fp.Children, models.ForkChild{
					ID:        child.ID,
					Preview:   preview,
					Role:      child.Role,
					CreatedAt: child.CreatedAt,
				})
			}
			if selectedID, ok := selMap[parentID]; ok {
				for i, child := range fp.Children {
					if child.ID == selectedID {
						fp.ActiveIndex = i
						break
					}
				}
			} else {
				fp.ActiveIndex = len(fp.Children) - 1
			}
			forkPoints[parentID] = fp
		}
	}

	// Handle multiple roots as fork point (sentinel: 0)
	if len(roots) > 1 {
		fp := models.ForkPoint{}
		for _, root := range roots {
			preview := root.Content
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			fp.Children = append(fp.Children, models.ForkChild{
				ID:        root.ID,
				Preview:   preview,
				Role:      root.Role,
				CreatedAt: root.CreatedAt,
			})
		}
		if selectedID, ok := selMap[0]; ok {
			for i, child := range fp.Children {
				if child.ID == selectedID {
					fp.ActiveIndex = i
					break
				}
			}
		} else {
			fp.ActiveIndex = len(fp.Children) - 1
		}
		forkPoints[0] = fp
	}

	// Walk tree from root following active branches
	var path []models.Message
	var current *models.Message

	if len(roots) > 1 {
		fp := forkPoints[0]
		active := roots[fp.ActiveIndex]
		current = &active
	} else if len(roots) > 0 {
		current = &roots[0]
	} else {
		return allMessages, forkPoints, nil
	}

	for current != nil {
		path = append(path, *current)
		children := childrenMap[current.ID]
		if len(children) == 0 {
			break
		}
		if len(children) == 1 {
			c := children[0]
			current = &c
		} else {
			fp := forkPoints[current.ID]
			activeChild := children[fp.ActiveIndex]
			current = &activeChild
		}
	}

	return path, forkPoints, nil
}

// getLastMessageInPath returns the last message in the active branch path.
func getLastMessageInPath(db *gorm.DB, threadID int64) *models.Message {
	path, _, err := getActivePath(db, threadID)
	if err != nil || len(path) == 0 {
		return nil
	}
	last := path[len(path)-1]
	return &last
}
