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

func TestKeepalivePing_SkipsWhenRateLimited(t *testing.T) {
	t.Parallel()

	mon := NewUsageMonitor("", 0.90, 0.95)
	mon.lastPollOK = true
	mon.info = UsageInfo{FiveHourPct: 0.95, SevenDayPct: 0.50}

	called := false
	r := &Runner{
		state:    models.StateRunning,
		config:   &config.Config{},
		usageMon: mon,
		pingFn:   func() error { called = true; return nil },
	}

	r.keepalivePing()

	if called {
		t.Error("expected ping to be skipped when usage monitor reports rate limited")
	}
}

func TestKeepalivePing_RunsWhenNotRateLimited(t *testing.T) {
	t.Parallel()

	mon := NewUsageMonitor("", 0.90, 0.95)
	mon.lastPollOK = true
	mon.info = UsageInfo{FiveHourPct: 0.50, SevenDayPct: 0.60}

	called := false
	r := &Runner{
		state:    models.StateRunning,
		config:   &config.Config{},
		usageMon: mon,
		pingFn:   func() error { called = true; return nil },
	}

	r.keepalivePing()

	if !called {
		t.Error("expected ping to run when usage monitor reports not rate limited")
	}
}

func TestKeepalivePing_RunsWhenUsageMonNil(t *testing.T) {
	t.Parallel()

	called := false
	r := &Runner{
		state:    models.StateRunning,
		config:   &config.Config{},
		usageMon: nil,
		pingFn:   func() error { called = true; return nil },
	}

	r.keepalivePing()

	if !called {
		t.Error("expected ping to run when usage monitor is nil")
	}
}

func TestKeepalivePing_StoppedTakesPrecedenceOverRateLimit(t *testing.T) {
	t.Parallel()

	mon := NewUsageMonitor("", 0.90, 0.95)
	mon.lastPollOK = true
	mon.info = UsageInfo{FiveHourPct: 0.95, SevenDayPct: 0.50}

	called := false
	r := &Runner{
		state:    models.StateStopped,
		config:   &config.Config{},
		usageMon: mon,
		pingFn:   func() error { called = true; return nil },
	}

	r.keepalivePing()

	if called {
		t.Error("expected ping to be skipped when stopped (regardless of rate limit)")
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

	target, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, time.Time{})

	wantTarget := resetsAt.Add(2 * time.Minute)
	if !target.Equal(wantTarget) {
		t.Errorf("expected target %v, got %v", wantTarget, target)
	}
	// Delay should be ~47min: 45min until the reset, plus the 2min delay after it.
	if delay < 46*time.Minute || delay > 48*time.Minute {
		t.Errorf("expected delay around 47m, got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_PastTargetPingsImmediately(t *testing.T) {
	t.Parallel()

	// The window reset 5 minutes ago and we have not pinged for it — the target
	// is in the past, so open a window now.
	resetsAt := time.Now().Add(-5 * time.Minute)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	_, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, time.Time{})

	if delay != 0 {
		t.Errorf("expected immediate ping (delay=0), got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_NearTargetWaitsInsteadOfFiringEarly(t *testing.T) {
	t.Parallel()

	// The reset is 30s away, so the target is 2m30s away. The old
	// keepaliveMinDelay clamp would round any sub-minute delay down to zero;
	// under the new timing that fires the ping BEFORE the reset, spending it on
	// the closing window and opening nothing. The delay must be waited out.
	resetsAt := time.Now().Add(30 * time.Second)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	_, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, time.Time{})

	if delay < 2*time.Minute {
		t.Errorf("expected the delay to be waited out (>=2m), got %v — a ping this early lands before the reset", delay)
	}
}

func TestComputeKeepaliveSchedule_ZeroResetsAtUsesFallback(t *testing.T) {
	t.Parallel()

	r := &Runner{
		resetsAtFn: func() time.Time { return time.Time{} },
	}

	target, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, time.Time{})

	if !target.IsZero() {
		t.Errorf("expected zero target in fallback mode, got %v", target)
	}
	if delay != 60*time.Minute {
		t.Errorf("expected fallback delay 60m, got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_ProjectsNextWindowWhileMonitorIsStale(t *testing.T) {
	t.Parallel()

	// We just pinged 2m after the reset, opening a new window. claude-usage is a
	// cron-refreshed cache, so the monitor keeps reporting the OLD resets_at for
	// several minutes. Recomputing must project to the window our own ping
	// opened (lastPing + 5h) instead of re-firing into it 2m later.
	resetsAt := time.Now().Add(-2 * time.Minute)
	lastPing := time.Now()
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	target, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, lastPing)

	wantTarget := lastPing.Add(5*time.Hour + 2*time.Minute)
	if !target.Equal(wantTarget) {
		t.Errorf("expected target projected to the next window %v, got %v", wantTarget, target)
	}
	if delay < 5*time.Hour {
		t.Errorf("expected a ~5h delay, got %v — this is the double ping the projection exists to prevent", delay)
	}
}

func TestComputeKeepaliveSchedule_FreshResetsAtAgreesWithProjection(t *testing.T) {
	t.Parallel()

	// Same moment as the stale-cache case, except the monitor has caught up and
	// reports the reset of the window our ping opened. Both branches of the max
	// must land on the same target.
	lastPing := time.Now()
	resetsAt := lastPing.Add(5 * time.Hour)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	target, _ := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, lastPing)

	wantTarget := resetsAt.Add(2 * time.Minute)
	if !target.Equal(wantTarget) {
		t.Errorf("expected target %v, got %v", wantTarget, target)
	}
}

func TestKeepaliveLoop_FallbackWhenResetsAtZero(t *testing.T) {
	t.Parallel()

	var count atomic.Int32
	cfg := &config.Config{
		KeepaliveInterval:   10 * time.Millisecond,
		KeepaliveResetDelay: 2 * time.Minute,
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
		KeepaliveInterval:   10 * time.Millisecond,
		KeepaliveResetDelay: 2 * time.Minute,
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
		KeepaliveInterval:   10 * time.Millisecond,
		KeepaliveResetDelay: 2 * time.Minute,
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
