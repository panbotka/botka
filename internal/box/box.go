// Package box provides Wake-on-LAN and SSH connectivity management for the
// remote Box build machine. It wakes the machine when necessary and caches
// its "alive" state so repeated calls within a short window skip the SSH probe.
//
// The Waker is safe for concurrent use and is intended to be shared between
// the chat runner and the task executor. Both code paths call EnsureUp before
// spawning Claude Code over SSH on the Box.
package box

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

const (
	// defaultAliveCacheTTL is how long a successful probe keeps us from
	// re-checking on every call. Five minutes matches the session pool TTL
	// so an active user session reuses one probe.
	defaultAliveCacheTTL = 5 * time.Minute

	// defaultProbeTimeout bounds each SSH connectivity probe.
	defaultProbeTimeout = 3 * time.Second

	// defaultWakeTimeout bounds the total wait for Box to come back up.
	defaultWakeTimeout = 60 * time.Second

	// defaultPollInterval is how often we re-probe while waiting for wake.
	defaultPollInterval = 5 * time.Second
)

// ErrBoxUnreachable is returned when the Box does not come up within the
// configured wake timeout.
var ErrBoxUnreachable = errors.New("box did not come up in time")

// Waker performs SSH connectivity checks and Wake-on-LAN for the Box.
// A single instance should be shared across the application.
type Waker struct {
	sshHost    string
	sshUser    string
	wolCommand string

	probeTimeout time.Duration
	wakeTimeout  time.Duration
	pollInterval time.Duration
	cacheTTL     time.Duration

	// runCmd runs a command and returns its combined output. Overridable for tests.
	runCmd func(ctx context.Context, name string, args ...string) ([]byte, error)

	mu           sync.Mutex
	lastAliveAt  time.Time
	lastCheckErr error
}

// NewWaker returns a Waker configured with the given SSH target and WoL command.
// sshHost is the SSH host alias or address (e.g. "box"); sshUser is the login
// user (e.g. "box"); wolCommand is the path to the Wake-on-LAN script.
func NewWaker(sshHost, sshUser, wolCommand string) *Waker {
	return &Waker{
		sshHost:      sshHost,
		sshUser:      sshUser,
		wolCommand:   wolCommand,
		probeTimeout: defaultProbeTimeout,
		wakeTimeout:  defaultWakeTimeout,
		pollInterval: defaultPollInterval,
		cacheTTL:     defaultAliveCacheTTL,
		runCmd: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // args are controlled
		},
	}
}

// SSHTarget returns the "user@host" string used as the SSH destination.
func (w *Waker) SSHTarget() string {
	return fmt.Sprintf("%s@%s", w.sshUser, w.sshHost)
}

// EnsureUp makes sure the Box is reachable via SSH. If a recent successful
// probe is cached, it returns immediately. Otherwise it runs a quick SSH probe,
// triggers Wake-on-LAN on failure, and polls until Box comes up or the wake
// timeout elapses. Returns ErrBoxUnreachable if the machine never responds.
func (w *Waker) EnsureUp(ctx context.Context) error {
	if w.cachedAlive() {
		return nil
	}

	// Quick probe: is Box already up?
	if err := w.probe(ctx); err == nil {
		w.markAlive()
		return nil
	}

	slog.Info("box unreachable, sending wake-on-lan", "host", w.sshHost, "command", w.wolCommand)
	if err := w.triggerWake(ctx); err != nil {
		slog.Warn("box wake command failed", "error", err)
		// fall through — we still poll in case Box wakes up anyway
	}

	if err := w.pollUntilAlive(ctx); err != nil {
		return err
	}
	w.markAlive()
	return nil
}

// Invalidate clears any cached "alive" state, forcing the next EnsureUp
// call to re-probe. Call this when you observe a dropped connection.
func (w *Waker) Invalidate() {
	w.mu.Lock()
	w.lastAliveAt = time.Time{}
	w.mu.Unlock()
}

// cachedAlive reports whether a recent successful probe still covers this call.
func (w *Waker) cachedAlive() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lastAliveAt.IsZero() {
		return false
	}
	return time.Since(w.lastAliveAt) < w.cacheTTL
}

// markAlive records the current time as the last successful probe.
func (w *Waker) markAlive() {
	w.mu.Lock()
	w.lastAliveAt = time.Now()
	w.lastCheckErr = nil
	w.mu.Unlock()
}

// probe runs a single SSH connectivity check bounded by probeTimeout.
// It returns nil if Box answers the probe, or an error otherwise.
func (w *Waker) probe(ctx context.Context) error {
	probeCtx, cancel := context.WithTimeout(ctx, w.probeTimeout)
	defer cancel()
	out, err := w.runCmd(probeCtx, "ssh",
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(w.probeTimeout.Seconds())),
		"-o", "StrictHostKeyChecking=no",
		w.SSHTarget(), "true",
	)
	if err != nil {
		return fmt.Errorf("ssh probe failed: %w: %s", err, string(out))
	}
	return nil
}

// triggerWake executes the configured Wake-on-LAN command.
func (w *Waker) triggerWake(ctx context.Context) error {
	if w.wolCommand == "" {
		return errors.New("wake-on-lan command not configured")
	}
	wakeCtx, cancel := context.WithTimeout(ctx, 10*time.Second) //nolint:mnd // internal wake-cmd timeout
	defer cancel()
	out, err := w.runCmd(wakeCtx, w.wolCommand)
	if err != nil {
		return fmt.Errorf("%s: %w: %s", w.wolCommand, err, string(out))
	}
	return nil
}

// pollUntilAlive repeatedly probes SSH until Box responds or wakeTimeout elapses.
func (w *Waker) pollUntilAlive(ctx context.Context) error {
	deadline := time.Now().Add(w.wakeTimeout)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	// Try an immediate probe first so we don't always wait a full interval.
	if err := w.probe(ctx); err == nil {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for box: %w", ctx.Err())
		case <-ticker.C:
			if err := w.probe(ctx); err == nil {
				return nil
			}
			if time.Now().After(deadline) {
				return ErrBoxUnreachable
			}
		}
	}
}
