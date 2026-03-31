package runner

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"botka/internal/config"
	"botka/internal/models"
)

func TestKeepalivePing_SkipsWhenStopped(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		state:  models.StateStopped,
		pingFn: func() error { called = true; return nil },
	}

	r.keepalivePing()

	if called {
		t.Error("expected ping to be skipped when runner is stopped")
	}
}

func TestKeepalivePing_RunsWhenRunning(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		state:  models.StateRunning,
		pingFn: func() error { called = true; return nil },
	}

	r.keepalivePing()

	if !called {
		t.Error("expected ping to run when runner is running")
	}
}

func TestKeepalivePing_RunsWhenPaused(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		state:  models.StatePaused,
		pingFn: func() error { called = true; return nil },
	}

	r.keepalivePing()

	if !called {
		t.Error("expected ping to run when runner is paused")
	}
}

func TestKeepalivePing_HandlesError(t *testing.T) {
	t.Parallel()

	r := &Runner{
		state:  models.StateRunning,
		pingFn: func() error { return errors.New("connection refused") },
	}

	// Should not panic; errors are logged and swallowed.
	r.keepalivePing()
}

func TestKeepaliveLoop_StopsOnClose(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{KeepaliveInterval: 10 * time.Millisecond}
	r := &Runner{
		state:  models.StateRunning,
		config: cfg,
		pingFn: func() error { return nil },
	}

	stopCh := make(chan struct{})
	r.wg.Add(1)
	go r.keepaliveLoop(stopCh)

	// Let a few ticks fire.
	time.Sleep(50 * time.Millisecond)
	close(stopCh)
	r.wg.Wait() // must return promptly
}

func TestKeepaliveLoop_FiresPeriodically(t *testing.T) {
	t.Parallel()

	var count atomic.Int32
	cfg := &config.Config{KeepaliveInterval: 10 * time.Millisecond}
	r := &Runner{
		state:  models.StateRunning,
		config: cfg,
		pingFn: func() error { count.Add(1); return nil },
	}

	stopCh := make(chan struct{})
	r.wg.Add(1)
	go r.keepaliveLoop(stopCh)

	time.Sleep(55 * time.Millisecond)
	close(stopCh)
	r.wg.Wait()

	got := count.Load()
	if got < 3 {
		t.Errorf("expected at least 3 pings in 55ms with 10ms interval, got %d", got)
	}
}

func TestDoPing_UsesPingFnWhenSet(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		pingFn: func() error { called = true; return nil },
	}

	err := r.doPing()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected pingFn to be called")
	}
}

func TestDoPing_FallsBackToDefault(t *testing.T) {
	t.Parallel()

	// Use /bin/true as a stand-in for claude — it ignores args and exits 0.
	r := &Runner{
		config: &config.Config{ClaudePath: "/bin/true"},
	}

	err := r.doPing()
	if err != nil {
		t.Fatalf("expected /bin/true to succeed, got: %v", err)
	}
}

func TestStartLocked_StartsKeepaliveWhenEnabled(t *testing.T) {
	t.Parallel()

	var pingCount atomic.Int32
	cfg := &config.Config{
		KeepaliveEnabled:  true,
		KeepaliveInterval: 10 * time.Millisecond,
	}
	r := &Runner{
		state:  models.StatePaused,
		config: cfg,
		pingFn: func() error { pingCount.Add(1); return nil },
	}

	r.mu.Lock()
	r.startLocked()
	r.mu.Unlock()

	time.Sleep(35 * time.Millisecond)

	// Shutdown to stop both loops.
	r.mu.Lock()
	if r.stopCh != nil {
		close(r.stopCh)
		r.stopCh = nil
	}
	r.mu.Unlock()
	r.wg.Wait()

	if pingCount.Load() < 1 {
		t.Error("expected at least 1 keepalive ping")
	}
}

func TestStartLocked_NoKeepaliveWhenDisabled(t *testing.T) {
	t.Parallel()

	var pingCount atomic.Int32
	cfg := &config.Config{
		KeepaliveEnabled:  false,
		KeepaliveInterval: 10 * time.Millisecond,
	}
	r := &Runner{
		state:  models.StatePaused,
		config: cfg,
		pingFn: func() error { pingCount.Add(1); return nil },
	}

	r.mu.Lock()
	r.startLocked()
	r.mu.Unlock()

	time.Sleep(35 * time.Millisecond)

	r.mu.Lock()
	if r.stopCh != nil {
		close(r.stopCh)
		r.stopCh = nil
	}
	r.mu.Unlock()
	r.wg.Wait()

	if pingCount.Load() != 0 {
		t.Errorf("expected 0 pings when disabled, got %d", pingCount.Load())
	}
}
