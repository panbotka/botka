package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/claude"
	"botka/internal/models"
)

var allowedMimeTypes = map[string]bool{
	"image/png":       true,
	"image/jpeg":      true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
	"text/plain":      true,
}

const maxUploadSize = 10 << 20 // 10 MB

// ChatHandler handles chat message sending and streaming.
type ChatHandler struct {
	db         *gorm.DB
	model      string
	uploadDir  string
	claudeCfg  claude.RunConfig
	contextCfg claude.ContextConfig
	defaultDir string
}

// NewChatHandler creates a new ChatHandler with the given dependencies.
func NewChatHandler(db *gorm.DB, model, uploadDir string, claudeCfg claude.RunConfig, contextCfg claude.ContextConfig, defaultDir string) *ChatHandler {
	return &ChatHandler{
		db:         db,
		model:      model,
		uploadDir:  uploadDir,
		claudeCfg:  claudeCfg,
		contextCfg: contextCfg,
		defaultDir: defaultDir,
	}
}

// RegisterChatRoutes attaches chat endpoints to the given router group.
func RegisterChatRoutes(rg *gin.RouterGroup, h *ChatHandler) {
	rg.POST("/threads/:id/messages", h.SendMessage)
	rg.POST("/threads/:id/regenerate", h.Regenerate)
	rg.POST("/threads/:id/messages/:messageId/edit", h.EditMessage)
	rg.POST("/threads/:id/branch", h.Branch)
	rg.PUT("/threads/:id/active-branch", h.SwitchBranch)
	rg.POST("/threads/:id/session/clear", h.ClearSession)
	rg.POST("/threads/:id/session/new", h.NewSession)
	rg.GET("/threads/:id/stream/subscribe", h.SubscribeStream)
	rg.GET("/threads/:id/session-health", h.SessionHealth)
}

type sendMessageRequest struct {
	Content  string `json:"content"`
	PlanMode bool   `json:"plan_mode,omitempty"`
}

// SendMessage handles sending a user message and streaming the assistant response.
func (h *ChatHandler) SendMessage(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, threadID).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	var content string
	var planMode bool
	var files []*multipart.FileHeader

	ct := c.ContentType()
	if strings.HasPrefix(ct, "multipart/form-data") {
		form, err := c.MultipartForm()
		if err != nil {
			respondError(c, http.StatusBadRequest, "failed to parse upload")
			return
		}
		content = strings.TrimSpace(c.PostForm("content"))
		planMode = c.PostForm("plan_mode") == "true"
		files = form.File["files"]
	} else {
		var req sendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "invalid request")
			return
		}
		content = strings.TrimSpace(req.Content)
		planMode = req.PlanMode
	}

	if content == "" && len(files) == 0 {
		respondError(c, http.StatusBadRequest, "content or files required")
		return
	}

	if content == "" {
		content = "(attached files)"
	}

	// Find parent: last message in active path
	var parentID *int64
	if lastMsg := getLastMessageInPath(h.db, threadID); lastMsg != nil {
		parentID = &lastMsg.ID
	}

	msg := models.Message{
		ThreadID: threadID,
		Role:     "user",
		Content:  content,
		ParentID: parentID,
	}
	if err := h.db.Create(&msg).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to save message")
		return
	}
	h.db.Model(&models.Thread{}).Where("id = ?", threadID).Update("updated_at", time.Now())

	// Save uploaded files and collect successful attachments
	var attachments []*models.Attachment
	for _, fh := range files {
		att, err := h.saveUploadedFile(msg.ID, fh)
		if err != nil {
			log.Printf("failed to save uploaded file %s: %v", fh.Filename, err)
			continue
		}
		attachments = append(attachments, att)
	}

	prompt := content
	if len(attachments) > 0 {
		prompt = h.buildPromptWithAttachments(content, attachments)
	}
	if planMode {
		prompt = "[PLAN MODE] You are in PLAN mode. Only analyze, read, search, and discuss. Do NOT edit, write, or create files. Do NOT run destructive commands. Plan your approach and explain what you would do.\n\n" + prompt
	}

	h.streamResponse(c, &thread, prompt, &msg.ID)
}

// Regenerate deletes the last assistant message and re-streams a response.
func (h *ChatHandler) Regenerate(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, threadID).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	pathMessages, _, err := getActivePath(h.db, threadID)
	if err != nil || len(pathMessages) == 0 {
		respondError(c, http.StatusBadRequest, "no messages to regenerate")
		return
	}

	lastMsg := pathMessages[len(pathMessages)-1]
	if lastMsg.Role == "assistant" {
		// Delete the assistant message branch
		h.db.Exec(`WITH RECURSIVE descendants AS (
			SELECT id FROM messages WHERE id = ? AND thread_id = ?
			UNION ALL
			SELECT m.id FROM messages m JOIN descendants d ON m.parent_id = d.id
		) DELETE FROM messages WHERE id IN (SELECT id FROM descendants)`, lastMsg.ID, threadID)
	} else if lastMsg.Role != "user" {
		respondError(c, http.StatusBadRequest, "unexpected last message role")
		return
	}

	var lastUserContent string
	var parentID *int64
	for i := len(pathMessages) - 1; i >= 0; i-- {
		if pathMessages[i].Role == "user" {
			lastUserContent = pathMessages[i].Content
			parentID = &pathMessages[i].ID
			break
		}
	}

	h.streamResponse(c, &thread, lastUserContent, parentID)
}

// EditMessage creates a branch at an edited message and streams a new response.
func (h *ChatHandler) EditMessage(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	messageID, err := paramInt64(c, "messageId")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid message id")
		return
	}

	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		respondError(c, http.StatusBadRequest, "content is required")
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, threadID).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	var editedMsg models.Message
	if err := h.db.First(&editedMsg, messageID).Error; err != nil || editedMsg.ThreadID != threadID {
		respondError(c, http.StatusBadRequest, "message not found in this thread")
		return
	}

	// Create new user message as sibling (same parent_id) — creates a branch
	newMsg := models.Message{
		ThreadID: threadID,
		Role:     "user",
		Content:  strings.TrimSpace(req.Content),
		ParentID: editedMsg.ParentID,
	}
	if err := h.db.Create(&newMsg).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to save message")
		return
	}

	// Set branch selection to the new branch
	var forkID int64
	if editedMsg.ParentID != nil {
		forkID = *editedMsg.ParentID
	}
	h.db.Where(models.BranchSelection{ThreadID: threadID, ForkMessageID: forkID}).
		Assign(models.BranchSelection{SelectedChildID: newMsg.ID}).
		FirstOrCreate(&models.BranchSelection{})
	h.db.Model(&models.Thread{}).Where("id = ?", threadID).Update("updated_at", time.Now())

	h.streamResponse(c, &thread, strings.TrimSpace(req.Content), &newMsg.ID)
}

type branchRequest struct {
	ParentMessageID int64  `json:"parent_message_id"`
	Content         string `json:"content"`
}

// Branch creates a new branch from any message in the thread.
func (h *ChatHandler) Branch(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req branchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		respondError(c, http.StatusBadRequest, "content is required")
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, threadID).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	var parentMsg models.Message
	if err := h.db.First(&parentMsg, req.ParentMessageID).Error; err != nil || parentMsg.ThreadID != threadID {
		respondError(c, http.StatusBadRequest, "parent message not found in this thread")
		return
	}

	parentID := parentMsg.ID
	msg := models.Message{
		ThreadID: threadID,
		Role:     "user",
		Content:  content,
		ParentID: &parentID,
	}
	if err := h.db.Create(&msg).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to save message")
		return
	}

	// Set branch selection to new branch
	h.db.Where(models.BranchSelection{ThreadID: threadID, ForkMessageID: parentID}).
		Assign(models.BranchSelection{SelectedChildID: msg.ID}).
		FirstOrCreate(&models.BranchSelection{})
	h.db.Model(&models.Thread{}).Where("id = ?", threadID).Update("updated_at", time.Now())

	h.streamResponse(c, &thread, content, &msg.ID)
}

type switchBranchRequest struct {
	ForkMessageID   int64 `json:"fork_message_id"`
	SelectedChildID int64 `json:"selected_child_id"`
}

// SwitchBranch updates which child is active at a fork point.
func (h *ChatHandler) SwitchBranch(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req switchBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	h.db.Where(models.BranchSelection{ThreadID: threadID, ForkMessageID: req.ForkMessageID}).
		Assign(models.BranchSelection{SelectedChildID: req.SelectedChildID}).
		FirstOrCreate(&models.BranchSelection{})

	respondOK(c, gin.H{"status": "ok"})
}

// streamResponse is the core function that sends a message to Claude and streams SSE events.
// It uses persistent sessions via SessionManager: a single Claude Code process stays alive
// across all messages in a thread using the --input-format stream-json protocol.
func (h *ChatHandler) streamResponse(c *gin.Context, thread *models.Thread, lastUserContent string, lastUserMsgID *int64) {
	threadID := thread.ID

	// Determine working directory from project
	workDir := h.defaultDir
	var projectClaudeMD string
	var projectName, projectPath string
	if thread.ProjectID != nil {
		var project models.Project
		if err := h.db.First(&project, "id = ?", *thread.ProjectID).Error; err == nil {
			if project.Path != "" {
				if err := os.MkdirAll(project.Path, 0755); err != nil {
					log.Printf("failed to create directory %s: %v", project.Path, err)
				} else {
					workDir = project.Path
				}
			}
			projectClaudeMD = project.ClaudeMD
			projectName = project.Name
			projectPath = project.Path
		}
	}

	// Determine if this is a new session or resume
	isNewSession := thread.ClaudeSessionID == nil || *thread.ClaudeSessionID == ""
	sessionID := ""
	if !isNewSession {
		sessionID = *thread.ClaudeSessionID
		// Check if the session file actually exists for this working directory
		if !claude.SessionExists(sessionID, workDir) {
			log.Printf("[chat] thread %d: session %s not found for dir %s, starting fresh", threadID, sessionID[:8], workDir)
			isNewSession = true
			sessionID = uuid.New().String()
			h.db.Model(&models.Thread{}).Where("id = ?", threadID).Update("claude_session_id", sessionID)
		}
	} else {
		sessionID = uuid.New().String()
		h.db.Model(&models.Thread{}).Where("id = ?", threadID).Update("claude_session_id", sessionID)
	}

	// Assemble context file for new sessions
	var contextFile string
	if isNewSession {
		var existingMessages []models.Message
		if msgs, _, err := getActivePath(h.db, threadID); err == nil {
			existingMessages = msgs
		}

		// Build memory function
		getMemories := func(ctx context.Context) (string, error) {
			var memories []models.Memory
			if err := h.db.Find(&memories).Error; err != nil {
				return "", err
			}
			var parts []string
			for _, m := range memories {
				parts = append(parts, m.Content)
			}
			return strings.Join(parts, "\n\n"), nil
		}

		var err error
		contextFile, err = claude.AssembleContext(c.Request.Context(), h.contextCfg, threadID, getMemories, thread.SystemPrompt, projectClaudeMD, projectName, projectPath, existingMessages)
		if err != nil {
			log.Printf("failed to assemble context: %v", err)
		}
	}

	aiModel := ""
	if thread.Model != nil {
		aiModel = *thread.Model
	}
	if aiModel == "" {
		aiModel = h.model
	}

	cfg := h.claudeCfg
	cfg.SessionID = sessionID
	cfg.Resume = !isNewSession
	cfg.Model = aiModel
	cfg.SystemPromptFile = contextFile
	cfg.WorkDir = workDir
	cfg.Name = thread.Title

	// Get or create a persistent session for this thread.
	// The session stays alive across messages — no need for pre-warming or
	// per-message process spawns. Registry is managed by SessionManager.
	session, isNew := claude.Sessions.GetOrCreate(cfg, threadID, thread.Title)
	if session == nil {
		// Failed to start process — fall back to one-shot Run
		log.Printf("[chat] failed to create session for thread %d, falling back to Run", threadID)
		streamCtx, streamCancel := context.WithCancel(context.Background())
		defer streamCancel()
		claude.Registry.Register(threadID, thread.Title, streamCancel)
		stream := claude.Run(streamCtx, cfg, lastUserContent)
		h.streamEventsToClient(c, thread, stream, lastUserContent, lastUserMsgID, cfg, true)
		return
	}

	_ = isNew // session manager handles registration

	// Send the message to the persistent session
	stream := claude.Sessions.SendMessage(session, lastUserContent)

	// Start a stream buffer so late-joining clients can reconnect.
	claude.Streams.Start(threadID)

	h.streamEventsToClient(c, thread, stream, lastUserContent, lastUserMsgID, cfg, false)
}

// streamEventsToClient reads events from a stream channel and sends them to the
// HTTP client as SSE. It also saves the response to the database and handles
// stale session cleanup. If isFallback is true, the registry entry is cleaned up
// on completion (one-shot Run mode); otherwise the persistent session stays registered.
func (h *ChatHandler) streamEventsToClient(c *gin.Context, thread *models.Thread, stream <-chan claude.StreamEvent, lastUserContent string, lastUserMsgID *int64, cfg claude.RunConfig, isFallback bool) {
	threadID := thread.ID

	// Start a stream buffer so late-joining clients can reconnect.
	claude.Streams.Start(threadID)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		respondError(c, http.StatusInternalServerError, "streaming not supported")
		return
	}

	clientGone := c.Request.Context().Done()
	clientDisconnected := false

	var fullResponse strings.Builder
	var errored bool
	var lastErrorMsg string
	var lastCostUSD float64
	var lastInputTokens, lastOutputTokens int

	for event := range stream {
		// Check if client disconnected — stop writing but keep reading
		// so we can save the full response to the database.
		if !clientDisconnected {
			select {
			case <-clientGone:
				clientDisconnected = true
				log.Printf("[chat] client disconnected for thread %d, continuing to capture response", threadID)
			default:
			}
		}

		switch event.Kind {
		case claude.KindContentDelta:
			fullResponse.WriteString(event.Text)
			chunk, _ := json.Marshal(map[string]string{"content": event.Text})
			sseData := fmt.Sprintf("data: %s\n\n", chunk)
			claude.Streams.Publish(threadID, sseData)
			if !clientDisconnected {
				fmt.Fprint(c.Writer, sseData)
				flusher.Flush()
			}

		case claude.KindThinkingDelta:
			chunk, _ := json.Marshal(map[string]string{"content": event.Thinking})
			sseData := fmt.Sprintf("event: thinking\ndata: %s\n\n", chunk)
			claude.Streams.Publish(threadID, sseData)
			if !clientDisconnected {
				fmt.Fprint(c.Writer, sseData)
				flusher.Flush()
			}

		case claude.KindToolUse:
			toolJSON, _ := json.Marshal(map[string]interface{}{
				"id":    event.ToolID,
				"name":  event.ToolName,
				"input": json.RawMessage(event.ToolInput),
			})
			sseData := fmt.Sprintf("event: tool_use\ndata: %s\n\n", toolJSON)
			claude.Streams.Publish(threadID, sseData)
			if !clientDisconnected {
				fmt.Fprint(c.Writer, sseData)
				flusher.Flush()
			}

		case claude.KindToolResult:
			resultJSON, _ := json.Marshal(map[string]interface{}{
				"tool_use_id": event.ToolUseID,
				"content":     event.ToolContent,
				"is_error":    event.ToolIsError,
			})
			sseData := fmt.Sprintf("event: tool_result\ndata: %s\n\n", resultJSON)
			claude.Streams.Publish(threadID, sseData)
			if !clientDisconnected {
				fmt.Fprint(c.Writer, sseData)
				flusher.Flush()
			}

		case claude.KindResult:
			if event.IsError {
				log.Printf("[chat] thread %d result error: %s", threadID, event.ErrorMsg)
				errJSON, _ := json.Marshal(map[string]string{"error": event.ErrorMsg})
				sseData := fmt.Sprintf("event: error\ndata: %s\n\n", errJSON)
				claude.Streams.Publish(threadID, sseData)
				if !clientDisconnected {
					fmt.Fprint(c.Writer, sseData)
					flusher.Flush()
				}
				errored = true
				lastErrorMsg = event.ErrorMsg
			} else {
				if fullResponse.Len() == 0 && event.ResultText != "" {
					fullResponse.WriteString(event.ResultText)
				}
				lastCostUSD = event.CostUSD
				lastInputTokens = event.InputTokens
				lastOutputTokens = event.OutputTokens
				usageJSON, _ := json.Marshal(map[string]interface{}{
					"cost_usd":      event.CostUSD,
					"duration_ms":   event.DurationMs,
					"num_turns":     event.NumTurns,
					"input_tokens":  event.InputTokens,
					"output_tokens": event.OutputTokens,
				})
				sseData := fmt.Sprintf("event: usage\ndata: %s\n\n", usageJSON)
				claude.Streams.Publish(threadID, sseData)
				if !clientDisconnected {
					fmt.Fprint(c.Writer, sseData)
					flusher.Flush()
				}
			}

		case claude.KindError:
			log.Printf("[chat] thread %d process error: %s", threadID, event.ErrorMsg)
			errJSON, _ := json.Marshal(map[string]string{"error": event.ErrorMsg})
			sseData := fmt.Sprintf("event: error\ndata: %s\n\n", errJSON)
			claude.Streams.Publish(threadID, sseData)
			if !clientDisconnected {
				fmt.Fprint(c.Writer, sseData)
				flusher.Flush()
			}
			errored = true
			lastErrorMsg = event.ErrorMsg
		}
	}

	// Always save the response to the database, even if client disconnected
	if !errored {
		assistantContent := fullResponse.String()
		if assistantContent != "" {
			assistantMsg := models.Message{
				ThreadID:         threadID,
				Role:             "assistant",
				Content:          assistantContent,
				ParentID:         lastUserMsgID,
				PromptTokens:     &lastInputTokens,
				CompletionTokens: &lastOutputTokens,
				CostUSD:          &lastCostUSD,
			}
			if err := h.db.Create(&assistantMsg).Error; err != nil {
				log.Printf("failed to save assistant message: %v", err)
			}
		}

		// Update session token tracking
		claude.Sessions.UpdateTokens(threadID, lastInputTokens, lastOutputTokens)

		// Auto-title from first message
		var msgCount int64
		h.db.Model(&models.Message{}).Where("thread_id = ?", threadID).Count(&msgCount)
		needsAutoTitle := thread.Title == "New Chat" || (thread.PersonaName != "" && thread.Title == thread.PersonaName)
		if msgCount == 2 && needsAutoTitle {
			generatedTitle := claude.TitleFromContent(lastUserContent)
			h.db.Model(&models.Thread{}).Where("id = ?", threadID).Update("title", generatedTitle)
			titleJSON, _ := json.Marshal(map[string]string{"title": generatedTitle})
			sseData := fmt.Sprintf("event: title\ndata: %s\n\n", titleJSON)
			claude.Streams.Publish(threadID, sseData)
			if !clientDisconnected {
				fmt.Fprint(c.Writer, sseData)
				flusher.Flush()
			}
		}
	}

	doneData := "data: [DONE]\n\n"
	claude.Streams.Publish(threadID, doneData)
	claude.Streams.Finish(threadID)
	if !clientDisconnected {
		fmt.Fprint(c.Writer, doneData)
		flusher.Flush()
	}

	// If the session was not found, clear it so the next message starts a fresh session
	if errored && strings.Contains(lastErrorMsg, "No conversation found") {
		log.Printf("[chat] thread %d: stale session %s, clearing for next attempt", threadID, cfg.SessionID)
		h.db.Model(&models.Thread{}).Where("id = ?", threadID).Update("claude_session_id", nil)
		claude.Sessions.Evict(threadID)
	}

	// For fallback one-shot Run mode, clean up registry. For persistent sessions,
	// the session stays registered — it's alive and waiting for the next message.
	if isFallback && errored {
		claude.Registry.Unregister(threadID)
	}
}

// ClearSession clears the Claude Code session ID for a thread.
func (h *ChatHandler) ClearSession(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	claude.Sessions.Evict(threadID)
	h.db.Model(&models.Thread{}).Where("id = ?", threadID).Update("claude_session_id", nil)
	respondOK(c, gin.H{"status": "ok"})
}

// NewSession generates a new Claude Code session ID for a thread.
func (h *ChatHandler) NewSession(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	claude.Sessions.Evict(threadID)
	newSessionID := uuid.New().String()
	h.db.Model(&models.Thread{}).Where("id = ?", threadID).Update("claude_session_id", newSessionID)

	respondOK(c, gin.H{"status": "ok", "session_id": newSessionID})
}

// SessionHealth returns health information for the active Claude session
// on a thread, including token usage and context window proximity.
func (h *ChatHandler) SessionHealth(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, threadID).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	model := h.model
	if thread.Model != nil && *thread.Model != "" {
		model = *thread.Model
	}

	health := claude.Sessions.GetHealth(threadID, model)
	respondOK(c, health)
}

// SubscribeStream allows a client to reconnect to an in-progress SSE stream
// for a thread. It replays buffered events and then streams new ones until the
// stream finishes or the client disconnects.
func (h *ChatHandler) SubscribeStream(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	buffered, ch, ok := claude.Streams.Subscribe(threadID)
	if !ok {
		respondError(c, http.StatusNotFound, "no active stream")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	flusher, flusherOK := c.Writer.(http.Flusher)
	if !flusherOK {
		claude.Streams.Unsubscribe(threadID, ch)
		respondError(c, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Replay buffered events.
	for _, ev := range buffered {
		fmt.Fprint(c.Writer, ev)
	}
	flusher.Flush()

	// Stream new events until the stream finishes or client disconnects.
	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			claude.Streams.Unsubscribe(threadID, ch)
			return
		case ev, open := <-ch:
			if !open {
				// Stream finished — all events (including [DONE]) already replayed.
				return
			}
			fmt.Fprint(c.Writer, ev)
			flusher.Flush()
		}
	}
}

// buildPromptWithAttachments appends file references to the prompt so Claude can read them.
func (h *ChatHandler) buildPromptWithAttachments(content string, attachments []*models.Attachment) string {
	absDir, err := filepath.Abs(h.uploadDir)
	if err != nil {
		absDir = h.uploadDir
	}

	var b strings.Builder
	if content == "(attached files)" {
		// Files-only message — don't include the placeholder
	} else {
		b.WriteString(content)
	}

	b.WriteString("\n\nAttached files:\n")
	hasImage := false
	for _, att := range attachments {
		absPath := filepath.Join(absDir, att.StoredName)
		fmt.Fprintf(&b, "- %s (%s, original: %s)\n", absPath, att.MimeType, att.OriginalName)
		if strings.HasPrefix(att.MimeType, "image/") {
			hasImage = true
		}
	}
	if hasImage {
		b.WriteString("\nUse the Read tool to view image files.")
	}

	return b.String()
}

// saveUploadedFile writes a multipart file to disk and creates a DB record.
func (h *ChatHandler) saveUploadedFile(messageID int64, fh *multipart.FileHeader) (*models.Attachment, error) {
	if fh.Size > maxUploadSize {
		return nil, fmt.Errorf("file too large: %d bytes", fh.Size)
	}

	mimeType := fh.Header.Get("Content-Type")
	if !allowedMimeTypes[mimeType] {
		return nil, fmt.Errorf("unsupported file type: %s", mimeType)
	}

	src, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("open uploaded file: %w", err)
	}
	defer src.Close()

	ext := filepath.Ext(fh.Filename)
	storedName := uuid.New().String() + ext

	if err := os.MkdirAll(h.uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}

	dstPath := filepath.Join(h.uploadDir, storedName)
	dst, err := os.Create(dstPath)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(dstPath)
		return nil, fmt.Errorf("write file: %w", err)
	}

	attachment := models.Attachment{
		MessageID:    messageID,
		StoredName:   storedName,
		OriginalName: fh.Filename,
		MimeType:     mimeType,
		Size:         fh.Size,
	}
	if err := h.db.Create(&attachment).Error; err != nil {
		os.Remove(dstPath)
		return nil, fmt.Errorf("save attachment record: %w", err)
	}

	return &attachment, nil
}
