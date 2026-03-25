package claude

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SessionPool manages pre-warmed Claude Code subprocesses that stay alive
// between messages. After a response completes, a new idle process is spawned
// with --resume but no prompt argument. It reads from stdin when the next
// message arrives. After the configured TTL of inactivity, it is killed.
type SessionPool struct {
	mu       sync.Mutex
	sessions map[int64]*poolEntry
	ttl      time.Duration
}

type poolEntry struct {
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      io.ReadCloser
	stderr      io.ReadCloser
	stderrBuf   *stderrBuffer
	cancel      context.CancelFunc
	timer       *time.Timer
	cfg         RunConfig
	threadID    int64
	threadTitle string
}

// Pool is the package-level session pool singleton.
var Pool = NewSessionPool(5 * time.Minute)

// NewSessionPool creates a session pool with the given idle timeout.
func NewSessionPool(ttl time.Duration) *SessionPool {
	return &SessionPool{
		sessions: make(map[int64]*poolEntry),
		ttl:      ttl,
	}
}

// PreWarm spawns a new idle Claude process for the given thread.
// The process loads the session and waits for a prompt on stdin.
// The process is also registered in the ProcessRegistry so it shows
// in the UI as an active session.
func (p *SessionPool) PreWarm(cfg RunConfig, threadID int64, threadTitle string) {
	ctx, cancel := context.WithCancel(context.Background())

	args := buildIdleArgs(cfg)
	cmd := exec.CommandContext(ctx, cfg.ClaudePath, args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		log.Printf("[pool] stdin pipe error for thread %d: %v", threadID, err)
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		log.Printf("[pool] stdout pipe error for thread %d: %v", threadID, err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		log.Printf("[pool] stderr pipe error for thread %d: %v", threadID, err)
		return
	}

	if err := cmd.Start(); err != nil {
		cancel()
		log.Printf("[pool] failed to pre-warm for thread %d: %v", threadID, err)
		return
	}

	// Write a space immediately to prevent Claude's 3-second stdin timeout.
	// Claude reads all of stdin until EOF as the prompt, so the leading space
	// is harmless — it just keeps the process alive waiting for more input.
	if _, err := stdin.Write([]byte(" ")); err != nil {
		cancel()
		log.Printf("[pool] failed to write keepalive for thread %d: %v", threadID, err)
		return
	}

	sessionPrefix := cfg.SessionID
	if len(sessionPrefix) > 8 {
		sessionPrefix = sessionPrefix[:8]
	}
	log.Printf("[pool] pre-warmed session %s for thread %d (pid %d)", sessionPrefix, threadID, cmd.Process.Pid)

	// Register in the process registry so the UI shows the session as active.
	Registry.Register(threadID, threadTitle, cancel)

	entry := &poolEntry{
		cmd:         cmd,
		stdin:       stdin,
		stdout:      stdout,
		stderr:      stderr,
		stderrBuf:   &stderrBuffer{},
		cancel:      cancel,
		cfg:         cfg,
		threadID:    threadID,
		threadTitle: threadTitle,
	}

	// Drain stderr in background, keeping last lines for error reporting
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("[pool:%s] stderr: %s", sessionPrefix, line)
			entry.stderrBuf.Add(line)
		}
	}()

	entry.timer = time.AfterFunc(p.ttl, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if current, ok := p.sessions[threadID]; ok && current == entry {
			log.Printf("[pool] idle timeout for thread %d, killing pre-warmed session", threadID)
			delete(p.sessions, threadID)
			Registry.Unregister(threadID)
			cancel()
		}
	})

	p.mu.Lock()
	// Kill any existing pre-warmed session for this thread
	if old, ok := p.sessions[threadID]; ok {
		old.timer.Stop()
		old.cancel()
		// Registry entry is replaced by the new Register call above
	}
	p.sessions[threadID] = entry
	p.mu.Unlock()
}

// Acquire removes and returns a pre-warmed session for the given thread.
// Returns nil if no session exists or if the config doesn't match.
func (p *SessionPool) Acquire(threadID int64, cfg RunConfig) *poolEntry {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.sessions[threadID]
	if !ok {
		return nil
	}

	delete(p.sessions, threadID)
	entry.timer.Stop()
	// Don't unregister from the registry here. The caller (streamResponse)
	// already called Registry.Register before Acquire, replacing the idle
	// entry with the active session's cancel func. Unregistering would
	// create a brief gap where the session disappears from /api/v1/processes.

	// Check config compatibility
	if entry.cfg.SessionID != cfg.SessionID ||
		entry.cfg.Model != cfg.Model ||
		entry.cfg.WorkDir != cfg.WorkDir {
		log.Printf("[pool] config mismatch for thread %d, killing stale session", threadID)
		entry.cancel()
		return nil
	}

	return entry
}

// Evict kills and removes a pre-warmed session for the given thread.
func (p *SessionPool) Evict(threadID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok := p.sessions[threadID]; ok {
		entry.timer.Stop()
		entry.cancel()
		Registry.Unregister(threadID)
		delete(p.sessions, threadID)
		log.Printf("[pool] evicted pre-warmed session for thread %d", threadID)
	}
}

// Shutdown kills all pre-warmed sessions.
func (p *SessionPool) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, entry := range p.sessions {
		entry.timer.Stop()
		entry.cancel()
		Registry.Unregister(id)
		delete(p.sessions, id)
	}
	log.Printf("[pool] shutdown: all pre-warmed sessions killed")
}

// RunFromPool pipes a prompt to a pre-warmed session's stdin and streams
// events from its stdout. The event channel has identical semantics to Run.
func RunFromPool(entry *poolEntry, prompt string) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)

		sessionPrefix := entry.cfg.SessionID
		if len(sessionPrefix) > 8 {
			sessionPrefix = sessionPrefix[:8]
		}

		log.Printf("[pool] sending prompt to pre-warmed session %s", sessionPrefix)

		// Write prompt to stdin and close to signal EOF
		if _, err := fmt.Fprintln(entry.stdin, prompt); err != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("write stdin: %v", err)}
			entry.cancel()
			return
		}
		if err := entry.stdin.Close(); err != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("close stdin: %v", err)}
			entry.cancel()
			return
		}

		// Read stdout NDJSON events (same as Run)
		scanner := bufio.NewScanner(entry.stdout)
		scanner.Buffer(make([]byte, 0), 1<<20)

		var gotResultError bool
		for scanner.Scan() {
			line := scanner.Bytes()
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
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("read stdout: %v", err)}
		}

		if err := entry.cmd.Wait(); err != nil {
			if !gotResultError {
				errMsg := fmt.Sprintf("claude exited: %v", err)
				if entry.stderrBuf != nil {
					if detail := entry.stderrBuf.String(); detail != "" {
						errMsg += "\n" + detail
					}
				}
				ch <- StreamEvent{Kind: KindError, ErrorMsg: errMsg}
			}
		}
	}()

	return ch
}

// buildIdleArgs builds the command-line arguments for a pre-warmed session
// (same as Run but without the prompt argument).
func buildIdleArgs(cfg RunConfig) []string {
	args := []string{
		"-p",
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--dangerously-skip-permissions",
	}

	if cfg.SessionID != "" {
		args = append(args, "--resume", cfg.SessionID)
	}

	if cfg.Model != "" && !strings.Contains(cfg.Model, "/") {
		args = append(args, "--model", cfg.Model)
	}

	// No --append-system-prompt-file for resumed sessions
	// No --name for resumed sessions
	// No prompt argument — process reads from stdin

	return args
}
