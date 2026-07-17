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
	// length. Used to project the next ping time off the window our own ping
	// just opened, while the usage monitor's cache still reports the old one.
	keepaliveWindowLength = 5 * time.Hour
)

// keepaliveLoop periodically runs a minimal Claude Code session to keep the
// Anthropic API 5h rate limit window active. Runs in a dedicated goroutine
// alongside the scheduler loop and does not consume worker slots. Pings are
// scheduled to fire KEEPALIVE_RESET_DELAY after the current window resets, so
// the next window opens back-to-back with the one that just closed instead of
// waiting for organic traffic. When no usage data is available yet, falls back
// to the fixed-interval behavior driven by KEEPALIVE_INTERVAL.
func (r *Runner) keepaliveLoop(stopCh <-chan struct{}) {
	defer r.wg.Done()

	resetDelay := r.config.KeepaliveResetDelay
	fallback := r.config.KeepaliveInterval

	slog.Info("keepalive loop started", "reset_delay", resetDelay, "fallback_interval", fallback)

	var lastPing time.Time
	target, delay := r.computeKeepaliveSchedule(resetDelay, fallback, lastPing)
	logKeepaliveSchedule(target, delay)
	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-stopCh:
			slog.Info("keepalive loop stopped")
			return
		case <-timer.C:
			// Stamp before the ping, not after: the window opens when the request
			// reaches the API, near the start of keepalivePing, which then blocks
			// on the subprocess for several seconds. Stamping afterwards would
			// fold that duration into every subsequent target.
			lastPing = time.Now()
			r.keepalivePing()
			target, delay = r.computeKeepaliveSchedule(resetDelay, fallback, lastPing)
			logKeepaliveSchedule(target, delay)
			timer.Reset(delay)
		}
	}
}

// computeKeepaliveSchedule returns the target time of the next ping and the
// delay until then. The ping fires resetDelay after the 5h window resets, so
// the new window opens immediately rather than waiting for organic traffic.
//
// Behavior:
//   - If the reset time is unknown (zero), fall back to the fixed interval and
//     return a zero target — this is the cold-start path when the usage monitor
//     hasn't polled yet.
//   - Otherwise target the later of resetsAt and lastPing+5h, plus resetDelay.
//     Our own ping at lastPing opened a window that resets 5h later, which is a
//     more reliable figure than a resetsAt we know may be stale by up to the
//     claude-usage cache interval. Taking the later of the two keeps the loop
//     from firing a second time into the window it just opened, and re-syncs
//     onto the authoritative resetsAt once the monitor catches up. On cold start
//     lastPing is the zero time, so lastPing+5h lands in year 1 and resetsAt
//     wins — no special case needed.
//   - Only a target in the past collapses to a zero delay. A target in the near
//     future must be waited out: firing before resetsAt spends the ping on the
//     closing window and opens nothing, costing a full 5h window, whereas
//     firing late costs only the wait.
//
// Always computes the deadline freshly from time.Now() and the target to avoid
// timer drift across iterations.
func (r *Runner) computeKeepaliveSchedule(resetDelay, fallback time.Duration, lastPing time.Time) (time.Time, time.Duration) {
	resetsAt := r.currentResetsAt()
	if resetsAt.IsZero() {
		return time.Time{}, fallback
	}

	target := maxTime(resetsAt, lastPing.Add(keepaliveWindowLength)).Add(resetDelay)

	delay := time.Until(target)
	if delay < 0 {
		delay = 0
	}
	return target, delay
}

// maxTime returns the later of a and b.
func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
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
