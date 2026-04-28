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
		config: &config.Config{},
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
		config: &config.Config{},
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
		config: &config.Config{},
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
		config: &config.Config{},
		pingFn: func() error { return errors.New("connection refused") },
	}

	// Should not panic; errors are logged and swallowed.
	r.keepalivePing()
}

func TestKeepalivePing_SkipsOnRecentTaskActivity(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		state:  models.StateRunning,
		config: &config.Config{KeepaliveActivityThreshold: 50 * time.Minute},
		pingFn: func() error { called = true; return nil },
		activityFn: func() (time.Time, error) {
			return time.Now().Add(-12 * time.Minute), nil
		},
	}

	r.keepalivePing()

	if called {
		t.Error("expected ping to be skipped after recent task activity")
	}
}

func TestKeepalivePing_SkipsOnRecentMessageActivity(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		state:  models.StateRunning,
		config: &config.Config{KeepaliveActivityThreshold: 50 * time.Minute},
		pingFn: func() error { called = true; return nil },
		activityFn: func() (time.Time, error) {
			// Simulate a recent chat message inside the threshold window.
			return time.Now().Add(-5 * time.Minute), nil
		},
	}

	r.keepalivePing()

	if called {
		t.Error("expected ping to be skipped after recent message activity")
	}
}

func TestKeepalivePing_RunsWhenActivityIsOld(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		state:  models.StateRunning,
		config: &config.Config{KeepaliveActivityThreshold: 50 * time.Minute},
		pingFn: func() error { called = true; return nil },
		activityFn: func() (time.Time, error) {
			return time.Now().Add(-2 * time.Hour), nil
		},
	}

	r.keepalivePing()

	if !called {
		t.Error("expected ping to run when most recent activity is older than threshold")
	}
}

func TestKeepalivePing_RunsWhenActivityCheckErrors(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		state:  models.StateRunning,
		config: &config.Config{KeepaliveActivityThreshold: 50 * time.Minute},
		pingFn: func() error { called = true; return nil },
		activityFn: func() (time.Time, error) {
			return time.Time{}, errors.New("db unavailable")
		},
	}

	r.keepalivePing()

	if !called {
		t.Error("expected ping to fall through and run when activity check fails")
	}
}

func TestKeepalivePing_RunsWhenTablesEmpty(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		state:  models.StateRunning,
		config: &config.Config{KeepaliveActivityThreshold: 50 * time.Minute},
		pingFn: func() error { called = true; return nil },
		activityFn: func() (time.Time, error) {
			// Both tables empty: zero time, no error.
			return time.Time{}, nil
		},
	}

	r.keepalivePing()

	if !called {
		t.Error("expected ping to run when both tables are empty")
	}
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

func TestComputeKeepaliveSchedule_FutureResetsAt(t *testing.T) {
	t.Parallel()

	resetsAt := time.Now().Add(45 * time.Minute)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	target, delay := r.computeKeepaliveSchedule(15*time.Minute, 60*time.Minute, time.Time{})

	wantTarget := resetsAt.Add(-15 * time.Minute)
	if !target.Equal(wantTarget) {
		t.Errorf("expected target %v, got %v", wantTarget, target)
	}
	// Delay should be ~30min (45min until reset minus 15min lead time).
	if delay < 29*time.Minute || delay > 31*time.Minute {
		t.Errorf("expected delay around 30m, got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_PastTargetPingsImmediately(t *testing.T) {
	t.Parallel()

	// resetsAt is 5 minutes from now, leadTime is 15 minutes — target is in past.
	resetsAt := time.Now().Add(5 * time.Minute)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	_, delay := r.computeKeepaliveSchedule(15*time.Minute, 60*time.Minute, time.Time{})

	if delay != 0 {
		t.Errorf("expected immediate ping (delay=0), got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_NearTargetPingsImmediately(t *testing.T) {
	t.Parallel()

	// Target is 30 seconds from now — within the ~1 minute "imminent" window.
	resetsAt := time.Now().Add(15*time.Minute + 30*time.Second)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	_, delay := r.computeKeepaliveSchedule(15*time.Minute, 60*time.Minute, time.Time{})

	if delay != 0 {
		t.Errorf("expected immediate ping (delay=0) when target is within minDelay, got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_ZeroResetsAtUsesFallback(t *testing.T) {
	t.Parallel()

	r := &Runner{
		resetsAtFn: func() time.Time { return time.Time{} },
	}

	target, delay := r.computeKeepaliveSchedule(15*time.Minute, 60*time.Minute, time.Time{})

	if !target.IsZero() {
		t.Errorf("expected zero target in fallback mode, got %v", target)
	}
	if delay != 60*time.Minute {
		t.Errorf("expected fallback delay 60m, got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_AdvancesToNextWindowAfterPing(t *testing.T) {
	t.Parallel()

	// Simulate: we just pinged at the current target, but resetsAt hasn't
	// updated yet. The next computation should project to the next window
	// (5h later) instead of looping on the same target.
	resetsAt := time.Now().Add(45 * time.Minute)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}
	lastTarget := resetsAt.Add(-15 * time.Minute)

	target, delay := r.computeKeepaliveSchedule(15*time.Minute, 60*time.Minute, lastTarget)

	wantTarget := lastTarget.Add(5 * time.Hour)
	if !target.Equal(wantTarget) {
		t.Errorf("expected target advanced by one window to %v, got %v", wantTarget, target)
	}
	// Delay should be roughly 5h30m (5h + 30min until original target).
	wantDelay := time.Until(wantTarget)
	if delay < wantDelay-time.Second || delay > wantDelay+time.Second {
		t.Errorf("expected delay near %v, got %v", wantDelay, delay)
	}
}

func TestComputeKeepaliveSchedule_UsesFreshResetsAtAfterWindowAdvanced(t *testing.T) {
	t.Parallel()

	// After we ping for the current window, the usage monitor refreshes and
	// reports the next window's resetsAt. The new target is well after
	// lastTarget, so we use it directly without projecting.
	prevResetsAt := time.Now().Add(45 * time.Minute)
	newResetsAt := prevResetsAt.Add(5*time.Hour + 10*time.Minute)
	r := &Runner{
		resetsAtFn: func() time.Time { return newResetsAt },
	}
	lastTarget := prevResetsAt.Add(-15 * time.Minute)

	target, _ := r.computeKeepaliveSchedule(15*time.Minute, 60*time.Minute, lastTarget)

	wantTarget := newResetsAt.Add(-15 * time.Minute)
	if !target.Equal(wantTarget) {
		t.Errorf("expected target derived from fresh resetsAt %v, got %v", wantTarget, target)
	}
}

func TestKeepaliveLoop_FallbackWhenResetsAtZero(t *testing.T) {
	t.Parallel()

	var count atomic.Int32
	cfg := &config.Config{
		KeepaliveInterval: 10 * time.Millisecond,
		KeepaliveLeadTime: 15 * time.Minute,
	}
	r := &Runner{
		state:      models.StateRunning,
		config:     cfg,
		pingFn:     func() error { count.Add(1); return nil },
		resetsAtFn: func() time.Time { return time.Time{} },
	}

	stopCh := make(chan struct{})
	r.wg.Add(1)
	go r.keepaliveLoop(stopCh)

	time.Sleep(55 * time.Millisecond)
	close(stopCh)
	r.wg.Wait()

	if count.Load() < 3 {
		t.Errorf("expected at least 3 fallback pings, got %d", count.Load())
	}
}

func TestKeepaliveLoop_PingsImmediatelyWhenTargetInPast(t *testing.T) {
	t.Parallel()

	var count atomic.Int32
	// Fixed past resetsAt — target is always in the past, so first ping is
	// immediate. After the first ping the loop should advance to next window
	// (5h later) so we don't see a flood.
	resetsAt := time.Now().Add(-1 * time.Hour)
	cfg := &config.Config{
		KeepaliveInterval: 10 * time.Millisecond,
		KeepaliveLeadTime: 15 * time.Minute,
	}
	r := &Runner{
		state:      models.StateRunning,
		config:     cfg,
		pingFn:     func() error { count.Add(1); return nil },
		resetsAtFn: func() time.Time { return resetsAt },
	}

	stopCh := make(chan struct{})
	r.wg.Add(1)
	go r.keepaliveLoop(stopCh)

	// Allow the immediate ping to fire, then stop. A second ping should not
	// fire because the next target is 5h away.
	time.Sleep(50 * time.Millisecond)
	close(stopCh)
	r.wg.Wait()

	if count.Load() != 1 {
		t.Errorf("expected exactly 1 immediate ping then advance to next window, got %d", count.Load())
	}
}

func TestKeepaliveLoop_StopsCleanlyWhileWaitingOnLongDelay(t *testing.T) {
	t.Parallel()

	// resetsAt is far in the future — the timer would normally wait an hour,
	// but stopCh must interrupt cleanly without leaking the timer.
	resetsAt := time.Now().Add(2 * time.Hour)
	cfg := &config.Config{
		KeepaliveInterval: 10 * time.Millisecond,
		KeepaliveLeadTime: 15 * time.Minute,
	}
	pinged := false
	r := &Runner{
		state:      models.StateRunning,
		config:     cfg,
		pingFn:     func() error { pinged = true; return nil },
		resetsAtFn: func() time.Time { return resetsAt },
	}

	stopCh := make(chan struct{})
	r.wg.Add(1)
	go r.keepaliveLoop(stopCh)

	// The next ping is ~1h45m away — give the goroutine time to settle on the
	// timer, then stop. Must return promptly without firing a ping.
	time.Sleep(20 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		close(stopCh)
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("keepaliveLoop did not stop within 1s after closing stopCh")
	}

	if pinged {
		t.Error("did not expect any ping when target is hours away")
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
