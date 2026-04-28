package runner

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os/exec"
	"time"

	"gorm.io/gorm"

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

// keepalivePing runs a minimal Claude Code session if the runner is not stopped
// and there has been no recent activity. The activity check is a pre-flight
// optimization: if a task started or a chat message was sent within
// KeepaliveActivityThreshold, that real interaction has already kept the 5h
// window alive, so the redundant ping is skipped. Errors querying the DB are
// logged at warn level and fall through to pinging (fail open — better to ping
// unnecessarily than to let the window expire).
func (r *Runner) keepalivePing() {
	r.mu.RLock()
	state := r.state
	r.mu.RUnlock()

	if state == models.StateStopped {
		slog.Debug("keepalive skipped: runner is stopped")
		return
	}

	threshold := r.config.KeepaliveActivityThreshold
	if threshold > 0 {
		latest, err := r.recentActivity()
		switch {
		case err != nil:
			slog.Warn("keepalive activity check failed, pinging anyway", "error", err)
		case !latest.IsZero() && time.Since(latest) < threshold:
			slog.Info("keepalive skipped: recent activity",
				"age", time.Since(latest).Round(time.Second),
				"threshold", threshold)
			return
		}
	}

	if err := r.doPing(); err != nil {
		slog.Warn("keepalive ping failed", "error", err)
		return
	}
	slog.Info("keepalive ping completed")
}

// recentActivity returns the most recent activity timestamp from either the
// tasks or messages tables. Returns the zero time when both tables are empty.
// Uses activityFn if set (for testing), otherwise queries the database via
// mostRecentActivity().
func (r *Runner) recentActivity() (time.Time, error) {
	if r.activityFn != nil {
		return r.activityFn()
	}
	return r.mostRecentActivity()
}

// mostRecentActivity queries the database for the most recent task start or
// chat message and returns the maximum of the two timestamps. Returns the
// zero time when both tables are empty. Each query is bounded to LIMIT 1 and
// uses an indexed ordering column (started_at DESC NULLS LAST for tasks,
// id DESC for messages where id is the bigserial primary key and serves as a
// monotonic proxy for created_at).
func (r *Runner) mostRecentActivity() (time.Time, error) {
	if r.db == nil {
		return time.Time{}, errors.New("no database")
	}

	var taskStarted sql.NullTime
	if err := r.db.Raw(
		"SELECT started_at FROM tasks WHERE started_at IS NOT NULL ORDER BY started_at DESC LIMIT 1",
	).Scan(&taskStarted).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, err
	}

	var msgCreated sql.NullTime
	if err := r.db.Raw(
		"SELECT created_at FROM messages ORDER BY id DESC LIMIT 1",
	).Scan(&msgCreated).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, err
	}

	var latest time.Time
	if taskStarted.Valid {
		latest = taskStarted.Time
	}
	if msgCreated.Valid && msgCreated.Time.After(latest) {
		latest = msgCreated.Time
	}
	return latest, nil
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
