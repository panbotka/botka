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

const (
	keepaliveTimeout = 2 * time.Minute
	// keepaliveWindowLength is the assumed Anthropic 5-hour rate-limit window
	// length. Used to project the next ping time after one has just fired.
	keepaliveWindowLength = 5 * time.Hour
	// keepaliveMinDelay is the threshold below which a computed delay is
	// treated as "ping immediately". One minute is enough slack for clock
	// skew and short delays in the usage monitor's poll cycle.
	keepaliveMinDelay = time.Minute
)

// keepaliveLoop periodically runs a minimal Claude Code session to keep the
// Anthropic API 5h rate limit window active. Runs in a dedicated goroutine
// alongside the scheduler loop and does not consume worker slots. Pings are
// scheduled to fire KEEPALIVE_LEAD_TIME before the current window resets, so
// the surviving window is refreshed just before it would expire. When no
// usage data is available yet, falls back to the legacy fixed-interval
// behavior driven by KEEPALIVE_INTERVAL.
func (r *Runner) keepaliveLoop(stopCh <-chan struct{}) {
	defer r.wg.Done()

	leadTime := r.config.KeepaliveLeadTime
	fallback := r.config.KeepaliveInterval

	slog.Info("keepalive loop started", "lead_time", leadTime, "fallback_interval", fallback)

	var lastTarget time.Time
	target, delay := r.computeKeepaliveSchedule(leadTime, fallback, lastTarget)
	logKeepaliveSchedule(target, delay)
	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-stopCh:
			slog.Info("keepalive loop stopped")
			return
		case <-timer.C:
			r.keepalivePing()
			lastTarget = target
			target, delay = r.computeKeepaliveSchedule(leadTime, fallback, lastTarget)
			logKeepaliveSchedule(target, delay)
			timer.Reset(delay)
		}
	}
}

// computeKeepaliveSchedule returns the target time of the next ping and the
// delay until then. It reads the current 5h reset time from the usage monitor
// and schedules the ping leadTime before reset.
//
// Behavior:
//   - If the reset time is unknown (zero), fall back to the fixed interval and
//     return a zero target — this is the cold-start path when the usage monitor
//     hasn't polled yet.
//   - If `resetsAt - leadTime` is in the past or within keepaliveMinDelay,
//     return a zero delay so the loop pings immediately.
//   - If we already pinged at or after the computed target (lastTarget is at
//     or past target), advance to the next window so we don't tight-loop on a
//     stale resetsAt.
//
// Always computes the deadline freshly from time.Now() and resetsAt to avoid
// timer drift across iterations.
func (r *Runner) computeKeepaliveSchedule(leadTime, fallback time.Duration, lastTarget time.Time) (time.Time, time.Duration) {
	resetsAt := r.currentResetsAt()
	if resetsAt.IsZero() {
		return time.Time{}, fallback
	}

	target := resetsAt.Add(-leadTime)

	// We've already pinged at or past this target — usage monitor hasn't
	// reflected the new window yet. Project to the next window so the loop
	// doesn't fire repeatedly off the same resetsAt.
	if !lastTarget.IsZero() && !target.After(lastTarget) {
		target = lastTarget.Add(keepaliveWindowLength)
	}

	delay := time.Until(target)
	if delay < keepaliveMinDelay {
		delay = 0
	}
	return target, delay
}

// currentResetsAt returns the current 5h reset time, using resetsAtFn for
// tests when set, otherwise reading from the usage monitor. Returns zero
// when no usage monitor is wired up (e.g. unit tests that don't need it).
func (r *Runner) currentResetsAt() time.Time {
	if r.resetsAtFn != nil {
		return r.resetsAtFn()
	}
	if r.usageMon == nil {
		return time.Time{}
	}
	return r.usageMon.ResetsAt()
}

// logKeepaliveSchedule emits an info log describing when the next ping will
// fire. The target is zero when we're in the fixed-interval fallback path.
func logKeepaliveSchedule(target time.Time, delay time.Duration) {
	if target.IsZero() {
		slog.Info("keepalive: next ping scheduled (fallback)",
			"at", time.Now().Add(delay).Format(time.RFC3339),
			"delay", delay)
		return
	}
	slog.Info("keepalive: next ping scheduled",
		"at", target.Format(time.RFC3339),
		"delay", delay)
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

	if r.usageMon != nil {
		if limited, reason := r.usageMon.IsRateLimited(); limited {
			slog.Info("keepalive skipped: rate limited", "reason", reason)
			return
		}
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
