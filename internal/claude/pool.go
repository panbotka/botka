package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// SessionManager manages persistent Claude Code processes that stay alive across
// multiple messages in a thread. Instead of spawning a new process per message,
// a single process uses --input-format stream-json to accept NDJSON user messages
// on stdin and streams responses on stdout.
//
// Process lifecycle:
//  1. First message to a thread spawns a new process with stream-json I/O
//  2. Each subsequent message writes NDJSON to the existing process's stdin
//  3. The process responds with NDJSON events, ending with a "result" event
//  4. After 5 minutes idle (no messages), the process is killed
//  5. Model/project changes or session clears kill the process; next message restarts
type SessionManager struct {
	mu       sync.Mutex
	sessions map[int64]*Session
	ttl      time.Duration
}

// ModelContextLimit returns the context window size for a given model name.
func ModelContextLimit(model string) int {
	switch model {
	case "opus":
		return 1_000_000
	default:
		return 200_000
	}
}

// SessionHealth contains health information for an active session.
type SessionHealth struct {
	Active                 bool    `json:"active"`
	TotalInputTokens       int     `json:"total_input_tokens,omitempty"`
	TotalOutputTokens      int     `json:"total_output_tokens,omitempty"`
	EstimatedContextTokens int     `json:"estimated_context_tokens,omitempty"`
	ContextLimit           int     `json:"context_limit,omitempty"`
	ContextUsagePct        float64 `json:"context_usage_pct,omitempty"`
	Model                  string  `json:"model,omitempty"`
	StartedAt              string  `json:"started_at,omitempty"`
	MessageCount           int     `json:"message_count,omitempty"`
}

// Session represents a persistent Claude Code process for a single thread.
type Session struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	stderrBuf *stderrBuffer
	scanner   *bufio.Scanner
	cancel    context.CancelFunc
	timer     *time.Timer
	cfg       RunConfig

	mcpHash     string // hash of resolved MCP servers at spawn time
	mcpCfgPath  string // path to generated MCP config file for cleanup
	threadID    int64
	threadTitle string

	// busy is true while a message is being processed (between writing the
	// user message and receiving the result event). Only one message at a
	// time per session.
	busy bool

	// msgMu serializes message sends to this session. A second message
	// arriving while one is in-flight blocks until the first completes.
	msgMu sync.Mutex

	// stdinMu serializes writes to stdin. This is needed because
	// SendToolResult writes to stdin from a different goroutine than
	// SendMessage's reading loop.
	stdinMu sync.Mutex

	// dead is set when the process exits or is killed. Once dead, the
	// session must be removed and a new one created.
	dead bool

	// exitMsg is a human-readable description of how the process exited
	// (e.g. "exit code 42" or "killed by signal killed (possible OOM kill)").
	// Populated by the monitor goroutine under m.mu before exited is closed.
	exitMsg string

	// exited is closed by the monitor goroutine after cmd.Wait() returns
	// and dead/exitMsg are populated. SendMessage waits on this briefly
	// after the scanner EOFs so it can report accurate exit details.
	exited chan struct{}

	sessionPrefix string

	// Token tracking for session health monitoring.
	totalInputTokens  int
	totalOutputTokens int
	lastInputTokens   int
	messageCount      int
	startedAt         time.Time
}

// Sessions is the package-level session manager singleton.
var Sessions = NewSessionManager(5 * time.Minute)

// NewSessionManager creates a session manager with the given idle timeout.
func NewSessionManager(ttl time.Duration) *SessionManager {
	return &SessionManager{
		sessions: make(map[int64]*Session),
		ttl:      ttl,
	}
}

// GetOrCreate returns the existing persistent session for a thread, or creates
// a new one. If the existing session's config doesn't match (model, workdir,
// session ID, or MCP server hash changed), it kills the old session and creates
// a fresh one. The mcpHash parameter is the hash of the resolved MCP servers;
// pass an empty string when no MCP servers are configured.
// Returns the session and true if it was newly created.
func (m *SessionManager) GetOrCreate(cfg RunConfig, threadID int64, threadTitle, mcpHash string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[threadID]; ok && !s.dead {
		if s.cfg.SessionID == cfg.SessionID &&
			s.cfg.Model == cfg.Model &&
			s.cfg.WorkDir == cfg.WorkDir &&
			s.mcpHash == mcpHash {
			s.timer.Stop()
			s.timer = time.AfterFunc(m.ttl, func() { m.idleTimeout(threadID) })
			return s, false
		}
		log.Printf("[session] config mismatch for thread %d, killing old session", threadID)
		s.timer.Stop()
		s.cancel()
		s.dead = true
		RemoveMCPConfig(s.mcpCfgPath)
		delete(m.sessions, threadID)
	}

	s := m.startSession(cfg, threadID, threadTitle, mcpHash)
	if s == nil {
		return nil, true
	}
	m.sessions[threadID] = s
	return s, true
}

// startSession spawns a new Claude Code process with stream-json I/O.
// When cfg.Remote is set, the process is spawned on the remote host via SSH.
func (m *SessionManager) startSession(cfg RunConfig, threadID int64, threadTitle, mcpHash string) *Session {
	ctx, cancel := context.WithCancel(context.Background())

	args := buildStreamArgs(cfg)
	cmd, err := buildSessionCmd(ctx, cfg, args)
	if err != nil {
		cancel()
		log.Printf("[session] build session command failed for thread %d: %v", threadID, err)
		return nil
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		log.Printf("[session] stdin pipe error for thread %d: %v", threadID, err)
		return nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		log.Printf("[session] stdout pipe error for thread %d: %v", threadID, err)
		return nil
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		log.Printf("[session] stderr pipe error for thread %d: %v", threadID, err)
		return nil
	}

	sessionPrefix := cfg.SessionID
	if len(sessionPrefix) > 8 {
		sessionPrefix = sessionPrefix[:8]
	}

	log.Printf("[session] spawning: %s %v (dir=%s)", cfg.ClaudePath, args, cmd.Dir)

	if err := cmd.Start(); err != nil {
		cancel()
		log.Printf("[session] failed to start for thread %d: %v", threadID, err)
		return nil
	}

	log.Printf("[session] started session %s for thread %d (pid %d)", sessionPrefix, threadID, cmd.Process.Pid)

	s := &Session{
		cmd:           cmd,
		stdin:         stdin,
		stdout:        stdout,
		stderr:        stderr,
		stderrBuf:     &stderrBuffer{},
		cancel:        cancel,
		cfg:           cfg,
		mcpHash:       mcpHash,
		mcpCfgPath:    cfg.MCPConfigPath,
		threadID:      threadID,
		threadTitle:   threadTitle,
		sessionPrefix: sessionPrefix,
		startedAt:     time.Now(),
		exited:        make(chan struct{}),
	}

	// Set up the stdout scanner for NDJSON parsing
	s.scanner = bufio.NewScanner(stdout)
	s.scanner.Buffer(make([]byte, 0), 1<<20) // 1MB buffer

	// Drain stderr in background
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("[session:%s] stderr: %s", sessionPrefix, line)
			s.stderrBuf.Add(line)
		}
	}()

	// Monitor process exit in background to mark session dead, capture
	// exit info, and proactively close stdout so any in-flight SendMessage
	// scanner returns instead of hanging.
	go func() {
		waitErr := cmd.Wait()
		desc := describeExit(waitErr, cmd.ProcessState)

		m.mu.Lock()
		alreadyDead := s.dead
		if !alreadyDead {
			s.dead = true
			s.exitMsg = desc
		}
		m.mu.Unlock()

		if !alreadyDead {
			log.Printf("[session] process exited unexpectedly for thread %d (session %s): %s",
				threadID, sessionPrefix, desc)
			if stderrContent := s.stderrBuf.String(); stderrContent != "" {
				log.Printf("[session:%s] full stderr:\n%s", sessionPrefix, stderrContent)
			}
		}

		// Close stdout to unblock any SendMessage scanner that is still
		// waiting on a line. Close order matters for the race where the
		// OS hasn't already EOF'd the pipe.
		_ = stdout.Close()
		close(s.exited)
	}()

	// Set idle timer
	s.timer = time.AfterFunc(m.ttl, func() { m.idleTimeout(threadID) })

	// Register in the process registry
	Registry.Register(threadID, threadTitle, cancel)

	return s
}

// SendMessage writes a user message to the persistent session and streams
// events until the result event. The returned channel has the same semantics
// as Run(). Only one message can be in-flight per session at a time.
func (m *SessionManager) SendMessage(s *Session, prompt string) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)

		// Serialize messages — only one in-flight per session
		s.msgMu.Lock()
		defer s.msgMu.Unlock()

		if s.dead {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: "session process has exited"}
			return
		}

		s.busy = true
		defer func() { s.busy = false }()

		// Reset idle timer while processing
		m.mu.Lock()
		s.timer.Stop()
		m.mu.Unlock()

		// Build NDJSON user message
		userMsg := map[string]interface{}{
			"type": "user",
			"message": map[string]interface{}{
				"role":    "user",
				"content": prompt,
			},
		}
		msgBytes, err := json.Marshal(userMsg)
		if err != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("marshal message: %v", err)}
			return
		}

		log.Printf("[session] sending message to session %s for thread %d", s.sessionPrefix, s.threadID)

		// Write NDJSON line to stdin (under stdinMu to serialize with SendToolResult)
		s.stdinMu.Lock()
		_, writeErr := s.stdin.Write(append(msgBytes, '\n'))
		s.stdinMu.Unlock()
		if writeErr != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("write stdin: %v", writeErr)}
			m.markDead(s)
			return
		}

		// Read events from stdout until result event
		var gotResultError bool
		for s.scanner.Scan() {
			line := s.scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			event := parseEvent(line)
			if event.Kind == KindIgnored {
				continue
			}
			if event.Kind == KindResult && event.IsError {
				gotResultError = true
			}
			ch <- event
			if event.Kind == KindResult {
				// Turn complete — process is idle, ready for next message
				break
			}
		}

		if err := s.scanner.Err(); err != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("read stdout: %v", err)}
			m.markDead(s)
			return
		}

		// If scanner ended without a result event, process likely died.
		// Wait briefly for the monitor goroutine to populate exit info so
		// the error message can include the exit code or signal name.
		if !gotResultError {
			select {
			case <-s.exited:
			case <-time.After(500 * time.Millisecond):
			}
			m.mu.Lock()
			isDead := s.dead
			exitMsg := s.exitMsg
			m.mu.Unlock()
			if isDead {
				errMsg := "claude process exited unexpectedly"
				if exitMsg != "" {
					errMsg += " (" + exitMsg + ")"
				}
				if detail := s.stderrBuf.String(); detail != "" {
					errMsg += "\n" + detail
				}
				ch <- StreamEvent{Kind: KindError, ErrorMsg: errMsg}
				return
			}
		}

		// Reset idle timer after message completes
		m.mu.Lock()
		if !s.dead {
			s.timer = time.AfterFunc(m.ttl, func() { m.idleTimeout(s.threadID) })
		}
		m.mu.Unlock()
	}()

	return ch
}

// markDead marks a session as dead and cleans up its manager entry.
func (m *SessionManager) markDead(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !s.dead {
		s.dead = true
		s.cancel()
		RemoveMCPConfig(s.mcpCfgPath)
		if current, ok := m.sessions[s.threadID]; ok && current == s {
			delete(m.sessions, s.threadID)
		}
	}
}

// idleTimeout is called when a session has been idle too long.
func (m *SessionManager) idleTimeout(threadID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[threadID]
	if !ok || s.dead {
		return
	}

	log.Printf("[session] idle timeout for thread %d, killing session %s", threadID, s.sessionPrefix)
	s.dead = true
	s.cancel()
	RemoveMCPConfig(s.mcpCfgPath)
	delete(m.sessions, threadID)
	Registry.Unregister(threadID)
}

// Evict kills and removes a persistent session for the given thread.
func (m *SessionManager) Evict(threadID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[threadID]; ok {
		s.timer.Stop()
		s.dead = true
		s.cancel()
		RemoveMCPConfig(s.mcpCfgPath)
		Registry.Unregister(threadID)
		delete(m.sessions, threadID)
		log.Printf("[session] evicted session for thread %d", threadID)
	}
}

// Shutdown kills all persistent sessions.
func (m *SessionManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, s := range m.sessions {
		s.timer.Stop()
		s.dead = true
		s.cancel()
		RemoveMCPConfig(s.mcpCfgPath)
		Registry.Unregister(id)
		delete(m.sessions, id)
	}
	log.Printf("[session] shutdown: all sessions killed")
}

// IsBusy returns true if the session for the given thread is currently
// processing a message.
func (m *SessionManager) IsBusy(threadID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[threadID]; ok {
		return s.busy
	}
	return false
}

// ErrNoSession is returned when no session exists for the given thread.
var ErrNoSession = errors.New("no active session")

// ErrNotBusy is returned when the session is not currently processing a message.
var ErrNotBusy = errors.New("session is not streaming")

// ErrSessionDead is returned when the session process has exited.
var ErrSessionDead = errors.New("session process has exited")

// SendToolResult writes a tool_result NDJSON message to the session's stdin.
// This is used when Claude calls an interactive tool like AskUserQuestion and
// waits for user input. The existing SendMessage reading loop is still running
// and will pick up Claude's continuation events after the tool result.
func (m *SessionManager) SendToolResult(threadID int64, toolUseID, content string, isError bool) error {
	m.mu.Lock()
	s, ok := m.sessions[threadID]
	m.mu.Unlock()

	if !ok {
		return ErrNoSession
	}
	if s.dead {
		return ErrSessionDead
	}
	if !s.busy {
		return ErrNotBusy
	}

	msg := map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     content,
		"is_error":    isError,
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal tool result: %w", err)
	}

	log.Printf("[session] sending tool result for %s to session %s (thread %d)", toolUseID, s.sessionPrefix, s.threadID)

	s.stdinMu.Lock()
	_, err = s.stdin.Write(append(msgBytes, '\n'))
	s.stdinMu.Unlock()
	if err != nil {
		m.markDead(s)
		return fmt.Errorf("write stdin: %w", err)
	}

	return nil
}

// Interrupt sends SIGINT to the Claude process for the given thread, which
// stops the current response without killing the session. The process emits
// a result event after receiving SIGINT, so the SendMessage scanner loop
// ends naturally and the session remains alive for the next message.
func (m *SessionManager) Interrupt(threadID int64) error {
	m.mu.Lock()
	s, ok := m.sessions[threadID]
	m.mu.Unlock()

	if !ok || s.dead {
		return ErrNoSession
	}
	if !s.busy {
		return ErrNotBusy
	}

	log.Printf("[session] sending SIGINT to session %s for thread %d (pid %d)",
		s.sessionPrefix, threadID, s.cmd.Process.Pid)

	if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
		log.Printf("[session] failed to send SIGINT to thread %d: %v", threadID, err)
		return fmt.Errorf("signal interrupt: %w", err)
	}

	return nil
}

// UpdateTokens records token usage from a completed message on the session
// for the given thread. Safe to call even if no session exists.
func (m *SessionManager) UpdateTokens(threadID int64, inputTokens, outputTokens int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[threadID]; ok && !s.dead {
		s.totalInputTokens += inputTokens
		s.totalOutputTokens += outputTokens
		s.lastInputTokens = inputTokens
		s.messageCount++
	}
}

// GetHealth returns session health information for the given thread.
// If no active session exists, returns SessionHealth with Active=false.
func (m *SessionManager) GetHealth(threadID int64, model string) SessionHealth {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[threadID]
	if !ok || s.dead {
		return SessionHealth{Active: false}
	}
	limit := ModelContextLimit(model)
	var pct float64
	if limit > 0 && s.lastInputTokens > 0 {
		pct = float64(s.lastInputTokens) / float64(limit) * 100
		if pct > 100 {
			pct = 100
		}
	}
	return SessionHealth{
		Active:                 true,
		TotalInputTokens:       s.totalInputTokens,
		TotalOutputTokens:      s.totalOutputTokens,
		EstimatedContextTokens: s.lastInputTokens,
		ContextLimit:           limit,
		ContextUsagePct:        math.Round(pct*10) / 10,
		Model:                  model,
		StartedAt:              s.startedAt.Format(time.RFC3339),
		MessageCount:           s.messageCount,
	}
}

// buildSessionCmd constructs the exec.Cmd for a persistent stream-json
// session. When cfg.Remote is non-nil the command is wrapped in SSH so the
// claude process lives on the remote host. Persistent sessions work the same
// over SSH because "ssh user@host cmd" transparently forwards stdin and
// stdout of the remote process.
func buildSessionCmd(ctx context.Context, cfg RunConfig, claudeArgs []string) (*exec.Cmd, error) {
	if cfg.Remote != nil {
		remoteDir, _ := SplitRemotePath(cfg.WorkDir)
		if remoteDir == "" {
			return nil, fmt.Errorf("remote session: empty remote path in %q", cfg.WorkDir)
		}
		if err := ensureRemoteUp(ctx, cfg.Remote); err != nil {
			return nil, err
		}
		// Persistent sessions read NDJSON from stdin, so we don't append a
		// trailing prompt argument. Reuse BuildSSHArgs but pass an empty
		// prompt and then trim the quoted empty string off the end.
		sshArgs := buildRemoteSessionArgs(cfg.Remote.SSHTarget, remoteDir, cfg.ClaudePath, claudeArgs)
		cmd := exec.CommandContext(ctx, sshArgs[0], sshArgs[1:]...) //nolint:gosec // args are controlled
		cmd.Env = SanitizedEnv()
		return cmd, nil
	}

	cmd := exec.CommandContext(ctx, cfg.ClaudePath, claudeArgs...)
	cmd.Env = SanitizedEnv()
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	return cmd, nil
}

// buildRemoteSessionArgs assembles the SSH argv for a persistent stream-json
// session with no trailing prompt. The remote command cd's into the working
// directory and exec's the claude binary, letting SSH forward stdin/stdout.
func buildRemoteSessionArgs(sshTarget, remoteDir, claudePath string, args []string) []string {
	var sb strings.Builder
	sb.WriteString("cd ")
	sb.WriteString(shellQuote(remoteDir))
	sb.WriteString(" && exec ")
	sb.WriteString(shellQuote(claudePath))
	for _, a := range args {
		sb.WriteByte(' ')
		sb.WriteString(shellQuote(a))
	}
	return []string{
		"ssh",
		"-o", "BatchMode=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "StrictHostKeyChecking=no",
		sshTarget,
		sb.String(),
	}
}

// buildStreamArgs builds the command-line arguments for a persistent
// stream-json process. Unlike the old pool's -p mode, this process stays
// alive and reads NDJSON user messages from stdin.
func buildStreamArgs(cfg RunConfig) []string {
	args := []string{
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
	}

	if cfg.Resume && cfg.SessionID != "" {
		args = append(args, "--resume", cfg.SessionID)
	} else if cfg.SessionID != "" {
		args = append(args, "--session-id", cfg.SessionID)
	}

	// Only pass --model for Claude Code model names (sonnet, opus, haiku)
	if cfg.Model != "" && !strings.Contains(cfg.Model, "/") {
		args = append(args, "--model", cfg.Model)
	}

	if cfg.SystemPromptFile != "" && !cfg.Resume {
		args = append(args, "--append-system-prompt-file", cfg.SystemPromptFile)
	}

	if cfg.Name != "" {
		args = append(args, "--name", cfg.Name)
	}

	if cfg.MCPConfigPath != "" {
		args = append(args, "--mcp-config", cfg.MCPConfigPath)
	}

	return args
}

// describeExit renders the outcome of cmd.Wait() into a short human-readable
// string: either "exit code N" or "killed by signal NAME (possible OOM kill)"
// when the process was terminated by SIGKILL. Used for diagnostics when a
// persistent session dies unexpectedly.
func describeExit(waitErr error, state *os.ProcessState) string {
	if state == nil {
		if waitErr != nil {
			return "wait error: " + waitErr.Error()
		}
		return "process state unavailable"
	}
	if ws, ok := state.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			sig := ws.Signal()
			msg := "killed by signal " + sig.String()
			if sig == syscall.SIGKILL {
				msg += " (possible OOM kill)"
			}
			return msg
		}
		if ws.Exited() {
			return fmt.Sprintf("exit code %d", ws.ExitStatus())
		}
	}
	return fmt.Sprintf("exit code %d", state.ExitCode())
}
