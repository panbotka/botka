package runner

import (
	"context"
	"log/slog"
	"os/exec"
	"time"

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

// keepalivePing runs a minimal Claude Code session unless the runner is stopped
// or already rate limited. The ping is unconditional with respect to activity:
// it is scheduled to fire just after the 5h window resets, and a task or chat
// message from before that reset belongs to the closing window and cannot open
// the next one, so there is nothing for prior activity to make redundant.
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
