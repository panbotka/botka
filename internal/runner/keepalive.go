package runner

import (
	"context"
	"log/slog"
	"os/exec"
	"time"

	"botka/internal/models"
)

const keepaliveTimeout = 2 * time.Minute

// keepaliveLoop periodically runs a minimal Claude Code session to keep the
// Anthropic API 5h rate limit window active. Runs in a dedicated goroutine
// alongside the scheduler loop and does not consume worker slots.
func (r *Runner) keepaliveLoop(stopCh <-chan struct{}) {
	defer r.wg.Done()

	interval := r.config.KeepaliveInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("keepalive loop started", "interval", interval)

	for {
		select {
		case <-stopCh:
			slog.Info("keepalive loop stopped")
			return
		case <-ticker.C:
			r.keepalivePing()
		}
	}
}

// keepalivePing runs a minimal Claude Code session if the runner is not stopped.
// Skipped when the runner is stopped since no tasks will run and the rate limit
// window does not need to stay active. Errors are logged but do not affect the runner.
func (r *Runner) keepalivePing() {
	r.mu.RLock()
	state := r.state
	r.mu.RUnlock()

	if state == models.StateStopped {
		slog.Debug("keepalive skipped: runner is stopped")
		return
	}

	if err := r.doPing(); err != nil {
		slog.Warn("keepalive ping failed", "error", err)
		return
	}
	slog.Info("keepalive ping completed")
}

// doPing executes the ping. Uses pingFn if set (for testing), otherwise runs
// the default Claude Code ping command.
func (r *Runner) doPing() error {
	if r.pingFn != nil {
		return r.pingFn()
	}
	return r.defaultPing()
}

// defaultPing spawns a minimal Claude Code session with a simple prompt.
// The session counts as a real API interaction, keeping the 5h window active.
func (r *Runner) defaultPing() error {
	ctx, cancel := context.WithTimeout(context.Background(), keepaliveTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.config.ClaudePath,
		"-p", "reply with pong",
		"--output-format", "text",
	)
	return cmd.Run()
}
