package signal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/box"
	"botka/internal/claude"
	"botka/internal/models"
)

// BotNumber is the E.164 phone number of Pan Botka's Signal account. Messages
// from this number are skipped by the bridge to prevent reply loops.
const BotNumber = "+420723750652"

// defaultReconnectInterval is the delay before the bridge reconnects to the
// signal-cli SSE stream after a disconnect or error.
const defaultReconnectInterval = 5 * time.Second

// BridgeConfig holds the dependencies the Bridge needs to process Signal
// messages end-to-end — database access, a Signal client, and the Claude
// runner configuration used to generate replies.
type BridgeConfig struct {
	// DB is the shared GORM handle used to look up threads, persist messages,
	// and read signal_bridges.
	DB *gorm.DB
	// Client is the Signal client used for both receiving (Subscribe) and
	// sending (SendGroupMessage) messages.
	Client *Client
	// ClaudeCfg is the base RunConfig used when invoking Claude Code to
	// generate replies. SessionID, Resume, Model, SystemPromptFile, WorkDir,
	// and Name are overridden per-thread.
	ClaudeCfg claude.RunConfig
	// ContextCfg is the context assembly configuration used when starting a
	// fresh Claude session for a bridged thread.
	ContextCfg claude.ContextConfig
	// DefaultModel is the fallback model when a thread has none set.
	DefaultModel string
	// DefaultWorkDir is the fallback working directory when a thread has no
	// project assigned.
	DefaultWorkDir string
	// BoxWaker, when non-nil, is used to wake and reach the remote Box host
	// for threads whose project uses a "box:" prefixed path.
	BoxWaker *box.Waker
	// BoxSSHTarget is the "user@host" SSH destination for remote threads.
	// When empty, remote-path threads fail fast.
	BoxSSHTarget string
	// ReconnectInterval is the delay between SSE reconnect attempts. Zero
	// means use defaultReconnectInterval.
	ReconnectInterval time.Duration
}

// Bridge is the background service that relays messages bidirectionally
// between Signal group chats and Botka threads.
//
// Incoming path: a goroutine keeps an SSE subscription open to the signal-cli
// daemon. Each group message is looked up in the signal_bridges table; if a
// matching thread exists and is active, the message is persisted, Claude is
// invoked to generate a reply, and the reply is sent back to the Signal group.
//
// Outgoing path: the chat handler calls ForwardToSignal after persisting a
// user or assistant message so that Signal participants see both sides of a
// conversation that originates in the Botka UI.
type Bridge struct {
	cfg BridgeConfig

	mu             sync.Mutex
	lastTimestamps map[string]int64      // group ID -> last processed ms timestamp
	threadLocks    map[int64]*sync.Mutex // thread ID -> per-thread processing lock
}

// NewBridge constructs a Bridge with the given config. The returned Bridge is
// idle until Start is called.
func NewBridge(cfg BridgeConfig) *Bridge {
	if cfg.ReconnectInterval == 0 {
		cfg.ReconnectInterval = defaultReconnectInterval
	}
	return &Bridge{
		cfg:            cfg,
		lastTimestamps: make(map[string]int64),
		threadLocks:    make(map[int64]*sync.Mutex),
	}
}

// Start launches the incoming-message loop in a background goroutine. The
// goroutine keeps a Subscribe stream open to the signal-cli daemon and
// reconnects after transient errors until ctx is cancelled.
func (b *Bridge) Start(ctx context.Context) {
	if b == nil || b.cfg.Client == nil {
		log.Printf("[signalbridge] not starting: no client configured")
		return
	}
	go b.run(ctx)
}

// run is the main reconnect loop. It calls Subscribe and, if the stream
// closes or fails, waits for ReconnectInterval before trying again. The loop
// exits when ctx is cancelled.
func (b *Bridge) run(ctx context.Context) {
	log.Printf("[signalbridge] starting")
	defer log.Printf("[signalbridge] stopped")

	for {
		if err := ctx.Err(); err != nil {
			return
		}
		err := b.cfg.Client.Subscribe(ctx, b.handleMessage)
		switch {
		case err == nil, errors.Is(err, context.Canceled):
			// Stream closed cleanly or context cancelled.
		case errors.Is(err, ErrDaemonUnreachable):
			log.Printf("[signalbridge] daemon unreachable, retrying in %s", b.cfg.ReconnectInterval)
		default:
			log.Printf("[signalbridge] subscribe ended: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(b.cfg.ReconnectInterval):
		}
	}
}

// handleMessage is the SSE callback invoked for every incoming Signal
// envelope. It filters uninteresting messages (non-group, self-sent, empty,
// duplicates) and dispatches the remainder to processMessage in a new
// goroutine so the SSE loop is never blocked by Claude execution.
func (b *Bridge) handleMessage(_ context.Context, msg SignalMessage) error {
	if msg.GroupID == "" {
		return nil
	}
	if msg.SourceNumber == BotNumber {
		return nil
	}
	if strings.TrimSpace(msg.Text) == "" {
		return nil
	}

	b.mu.Lock()
	if last, ok := b.lastTimestamps[msg.GroupID]; ok && msg.Timestamp <= last {
		b.mu.Unlock()
		return nil
	}
	b.lastTimestamps[msg.GroupID] = msg.Timestamp
	b.mu.Unlock()

	go func(m SignalMessage) {
		// Use a background context with a generous deadline: Claude runs can
		// take a while, and we don't want to cut them off if the SSE stream
		// reconnects mid-reply.
		procCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := b.processMessage(procCtx, m); err != nil {
			log.Printf("[signalbridge] process error for group %s: %v", m.GroupID, err)
		}
	}(msg)

	return nil
}

// processMessage handles one incoming group message: look up the bridge,
// persist the user message, run Claude to generate a reply, persist the
// assistant message, and send the reply back to the Signal group.
func (b *Bridge) processMessage(ctx context.Context, msg SignalMessage) error {
	var bridge models.SignalBridge
	err := b.cfg.DB.Where("group_id = ?", msg.GroupID).First(&bridge).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load signal bridge: %w", err)
	}
	if !bridge.Active {
		return nil
	}

	lock := b.threadLock(bridge.ThreadID)
	lock.Lock()
	defer lock.Unlock()

	var thread models.Thread
	if err := b.cfg.DB.First(&thread, bridge.ThreadID).Error; err != nil {
		return fmt.Errorf("load thread %d: %w", bridge.ThreadID, err)
	}

	displayName := strings.TrimSpace(msg.SourceName)
	if displayName == "" {
		displayName = msg.SourceNumber
	}
	if displayName == "" {
		displayName = "Signal"
	}
	content := fmt.Sprintf("**%s:** %s", displayName, strings.TrimSpace(msg.Text))

	parentID := lastMessageID(b.cfg.DB, bridge.ThreadID)
	userMsg := models.Message{
		ThreadID: bridge.ThreadID,
		Role:     "user",
		Content:  content,
		ParentID: parentID,
	}
	if err := b.cfg.DB.Create(&userMsg).Error; err != nil {
		return fmt.Errorf("save user message: %w", err)
	}
	b.cfg.DB.Model(&models.Thread{}).Where("id = ?", bridge.ThreadID).Update("updated_at", time.Now())

	responseText, inputTok, outputTok, costUSD, runErr := b.runClaude(ctx, &thread, userMsg.Content)
	if runErr != nil {
		log.Printf("[signalbridge] claude failed for thread %d: %v", bridge.ThreadID, runErr)
		notice := "Sorry — I couldn't generate a response right now."
		if _, sendErr := b.cfg.Client.SendGroupMessage(ctx, bridge.GroupID, notice); sendErr != nil {
			log.Printf("[signalbridge] failed to send error notice: %v", sendErr)
		}
		return runErr
	}

	responseText = strings.TrimSpace(responseText)
	if responseText == "" {
		log.Printf("[signalbridge] empty Claude response for thread %d, skipping send", bridge.ThreadID)
		return nil
	}

	userMsgID := userMsg.ID
	assistantMsg := models.Message{
		ThreadID:         bridge.ThreadID,
		Role:             "assistant",
		Content:          responseText,
		ParentID:         &userMsgID,
		PromptTokens:     &inputTok,
		CompletionTokens: &outputTok,
		CostUSD:          &costUSD,
	}
	if err := b.cfg.DB.Create(&assistantMsg).Error; err != nil {
		return fmt.Errorf("save assistant message: %w", err)
	}
	claude.Sessions.UpdateTokens(bridge.ThreadID, inputTok, outputTok)

	if _, err := b.cfg.Client.SendGroupMessage(ctx, bridge.GroupID, responseText); err != nil {
		return fmt.Errorf("send group message: %w", err)
	}
	return nil
}

// threadLock returns the per-thread mutex used to serialize bridge message
// processing for a single thread. Different threads run concurrently.
func (b *Bridge) threadLock(threadID int64) *sync.Mutex {
	b.mu.Lock()
	defer b.mu.Unlock()
	if m, ok := b.threadLocks[threadID]; ok {
		return m
	}
	m := &sync.Mutex{}
	b.threadLocks[threadID] = m
	return m
}

// runClaude assembles context, gets-or-creates a persistent Claude session
// for the thread, sends the prompt, and collects the full response along
// with token usage. The persistent session is shared with the chat UI, which
// means msgMu inside the session naturally queues concurrent requests.
func (b *Bridge) runClaude(ctx context.Context, thread *models.Thread, prompt string) (string, int, int, float64, error) {
	workDir := b.cfg.DefaultWorkDir
	var projectClaudeMD, projectName, projectPath string
	if thread.ProjectID != nil {
		var project models.Project
		if err := b.cfg.DB.First(&project, "id = ?", *thread.ProjectID).Error; err == nil {
			if project.Path != "" {
				switch {
				case claude.IsRemotePath(project.Path):
					workDir = project.Path
				default:
					if err := os.MkdirAll(project.Path, 0755); err == nil {
						workDir = project.Path
					}
				}
			}
			projectClaudeMD = project.ClaudeMD
			projectName = project.Name
			projectPath = project.Path
		}
	}

	isNewSession := thread.ClaudeSessionID == nil || *thread.ClaudeSessionID == ""
	sessionID := ""
	if !isNewSession {
		sessionID = *thread.ClaudeSessionID
		if !claude.SessionExists(sessionID, workDir) {
			isNewSession = true
			sessionID = uuid.New().String()
			b.cfg.DB.Model(&models.Thread{}).Where("id = ?", thread.ID).Update("claude_session_id", sessionID)
			thread.ClaudeSessionID = &sessionID
		}
	} else {
		sessionID = uuid.New().String()
		b.cfg.DB.Model(&models.Thread{}).Where("id = ?", thread.ID).Update("claude_session_id", sessionID)
		thread.ClaudeSessionID = &sessionID
	}

	var contextFile string
	if isNewSession {
		getMemories := func(_ context.Context) (string, error) {
			var memories []models.Memory
			if err := b.cfg.DB.Find(&memories).Error; err != nil {
				return "", err
			}
			var parts []string
			for _, m := range memories {
				parts = append(parts, m.Content)
			}
			return strings.Join(parts, "\n\n"), nil
		}

		var existing []models.Message
		b.cfg.DB.Where("thread_id = ?", thread.ID).Order("created_at ASC, id ASC").Find(&existing)

		var dbSources []models.ThreadSource
		b.cfg.DB.Where("thread_id = ?", thread.ID).Order("position ASC").Find(&dbSources)
		var sourceInputs []claude.SourceInput
		for _, s := range dbSources {
			sourceInputs = append(sourceInputs, claude.SourceInput{URL: s.URL, Label: s.Label})
		}

		var err error
		contextFile, err = claude.AssembleContext(ctx, b.cfg.ContextCfg, thread.ID, getMemories, thread.SystemPrompt, thread.CustomContext, projectClaudeMD, projectName, projectPath, sourceInputs, existing)
		if err != nil {
			log.Printf("[signalbridge] context assembly error: %v", err)
		}
	}

	model := b.cfg.DefaultModel
	if thread.Model != nil && *thread.Model != "" {
		model = *thread.Model
	}

	cfg := b.cfg.ClaudeCfg
	cfg.SessionID = sessionID
	cfg.Resume = !isNewSession
	cfg.Model = model
	cfg.SystemPromptFile = contextFile
	cfg.WorkDir = workDir
	cfg.Name = thread.Title
	if claude.IsRemotePath(workDir) && b.cfg.BoxSSHTarget != "" {
		cfg.Remote = &claude.RemoteSpec{
			SSHTarget: b.cfg.BoxSSHTarget,
			Waker:     b.cfg.BoxWaker,
		}
	}

	// Resolve MCP servers for this thread/project.
	var mcpHash string
	mcpServers, mcpErr := models.ResolveMCPServers(b.cfg.DB, &thread.ID, thread.ProjectID)
	if mcpErr != nil {
		log.Printf("[signalbridge] failed to resolve MCP servers for thread %d: %v", thread.ID, mcpErr)
	}
	if len(mcpServers) > 0 {
		mcpHash = claude.MCPServerHash(mcpServers)
		mcpPath, genErr := claude.GenerateMCPConfig(mcpServers, b.cfg.ContextCfg.ContextDir, fmt.Sprintf("thread-%d", thread.ID))
		if genErr != nil {
			log.Printf("[signalbridge] failed to generate MCP config for thread %d: %v", thread.ID, genErr)
		} else {
			cfg.MCPConfigPath = mcpPath
		}
	}

	session, _ := claude.Sessions.GetOrCreate(cfg, thread.ID, thread.Title, mcpHash)
	var stream <-chan claude.StreamEvent
	if session == nil {
		log.Printf("[signalbridge] session pool unavailable for thread %d, using one-shot Run", thread.ID)
		stream = claude.Run(ctx, cfg, prompt)
	} else {
		stream = claude.Sessions.SendMessage(session, prompt)
	}

	var fullResponse strings.Builder
	var lastCostUSD float64
	var lastInputTokens, lastOutputTokens int
	var errored bool
	var errMsg string

	for event := range stream {
		switch event.Kind {
		case claude.KindContentDelta:
			fullResponse.WriteString(event.Text)
		case claude.KindResult:
			if event.IsError {
				errored = true
				errMsg = event.ErrorMsg
			} else {
				if fullResponse.Len() == 0 && event.ResultText != "" {
					fullResponse.WriteString(event.ResultText)
				}
				lastCostUSD = event.CostUSD
				lastInputTokens = event.InputTokens
				lastOutputTokens = event.OutputTokens
			}
		case claude.KindError:
			errored = true
			errMsg = event.ErrorMsg
		}
	}

	if errored && fullResponse.Len() == 0 {
		if errMsg == "" {
			errMsg = "claude run failed"
		}
		return "", 0, 0, 0, errors.New(errMsg)
	}
	return fullResponse.String(), lastInputTokens, lastOutputTokens, lastCostUSD, nil
}

// ForwardToSignal sends the given message content to the Signal group linked
// to threadID, if an active bridge exists. It is called by the chat handler
// after persisting a user or assistant message so Signal participants see
// both sides of a conversation that originates in the Botka UI. Errors are
// logged but not returned — forwarding is best-effort.
//
// role should be "user" or "assistant"; the content is prefixed accordingly
// so the Signal group can distinguish the two sides of the conversation.
// Calling ForwardToSignal on a nil *Bridge is a no-op, which lets callers
// wire the bridge conditionally without nil checks at every call site.
func (b *Bridge) ForwardToSignal(ctx context.Context, threadID int64, role, content string) {
	if b == nil {
		return
	}
	text := strings.TrimSpace(content)
	if text == "" {
		return
	}

	var bridge models.SignalBridge
	err := b.cfg.DB.Where("thread_id = ?", threadID).First(&bridge).Error
	if err != nil {
		return
	}
	if !bridge.Active {
		return
	}

	var body string
	switch role {
	case "user":
		body = "**🧑 User:** " + text
	case "assistant":
		body = text
	default:
		body = text
	}

	if _, err := b.cfg.Client.SendGroupMessage(ctx, bridge.GroupID, body); err != nil {
		log.Printf("[signalbridge] forward (%s) to group %s failed: %v", role, bridge.GroupID, err)
	}
}

// lastMessageID returns a pointer to the highest message ID for threadID, or
// nil if the thread has no messages yet. This is a simple approximation of
// "latest message in the active branch path" that is sufficient for bridge
// traffic where branching is rare.
func lastMessageID(db *gorm.DB, threadID int64) *int64 {
	var last models.Message
	if err := db.Select("id").Where("thread_id = ?", threadID).Order("id DESC").First(&last).Error; err != nil {
		return nil
	}
	id := last.ID
	return &id
}
