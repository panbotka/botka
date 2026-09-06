package boxoff

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeProber struct {
	mu       sync.Mutex
	readings BoxReadings
	err      error
	calls    int
}

func (f *fakeProber) Probe(context.Context) (BoxReadings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.readings, f.err
}

func (f *fakeProber) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeKukatko struct {
	mu       sync.Mutex
	readings KukatkoReadings
	err      error
	calls    int
}

func (f *fakeKukatko) Fetch(context.Context) (KukatkoReadings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.readings, f.err
}

func (f *fakeKukatko) set(r KukatkoReadings, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readings, f.err = r, err
}

func (f *fakeKukatko) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeActivity struct {
	mu    sync.Mutex
	tasks []string
	chats []string
	err   error
}

func (f *fakeActivity) RunningTasks() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tasks
}

func (f *fakeActivity) BoxChatThreads(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.chats, f.err
}

func (f *fakeActivity) setTasks(tasks []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks = tasks
}

type fakeShutdowner struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeShutdowner) Shutdown(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

func (f *fakeShutdowner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// newTestMonitor builds a monitor over fakes with a clean, idle world.
// The nil DB is deliberate: event persistence is best-effort and must not
// panic or change the decision when there is nowhere to write.
func newTestMonitor(t *testing.T) (*Monitor, *fakeProber, *fakeKukatko, *fakeActivity, *fakeShutdowner) {
	t.Helper()

	prober := &fakeProber{readings: BoxReadings{UptimeSeconds: 33390, Load1: 0.01, Threads: 24}}
	kuk := &fakeKukatko{readings: KukatkoReadings{StatusCode: 200}}
	act := &fakeActivity{}
	sd := &fakeShutdowner{}

	m := NewMonitor(Config{
		Prober:     prober,
		Kukatko:    kuk,
		Activity:   act,
		Shutdowner: sd,
		Thresholds: testThresholds(),
	})
	m.setEnabled(true)

	return m, prober, kuk, act, sd
}

func TestMonitor_ShutsDownAfterThreeCleanChecks(t *testing.T) {
	m, _, _, _, sd := newTestMonitor(t)
	ctx := context.Background()

	for i := 1; i <= 2; i++ {
		if ev := m.RunOnce(ctx); !ev.Idle {
			t.Fatalf("check %d: not idle, blockers %v", i, blockerNames(ev))
		}
		if sd.count() != 0 {
			t.Fatalf("shut down after %d clean checks, want 3", i)
		}
	}

	m.RunOnce(ctx)

	if sd.count() != 1 {
		t.Fatalf("shutdown calls = %d, want exactly 1", sd.count())
	}
	if got := m.Snapshot().Streak; got != 0 {
		t.Errorf("streak = %d after shutdown, want 0", got)
	}
}

func TestMonitor_BlockerResetsTheStreak(t *testing.T) {
	m, _, kuk, _, sd := newTestMonitor(t)
	ctx := context.Background()

	m.RunOnce(ctx)
	m.RunOnce(ctx)

	kuk.set(KukatkoReadings{StatusCode: 200, Queued: 4}, nil)
	m.RunOnce(ctx)

	if got := m.Snapshot().Streak; got != 0 {
		t.Fatalf("streak = %d after a blocker, want 0", got)
	}

	kuk.set(KukatkoReadings{StatusCode: 200}, nil)
	m.RunOnce(ctx)
	m.RunOnce(ctx)

	if sd.count() != 0 {
		t.Fatal("shut down after 2 clean checks following a reset, want 3")
	}
}

func TestMonitor_DisabledDoesNotProbe(t *testing.T) {
	m, prober, kuk, _, sd := newTestMonitor(t)
	m.setEnabled(false)

	ev := m.RunOnce(context.Background())

	if prober.count() != 0 || kuk.count() != 0 {
		t.Errorf("probed %d / scraped %d while disabled, want 0/0", prober.count(), kuk.count())
	}
	if sd.count() != 0 {
		t.Error("shut down while disabled")
	}
	if len(ev.Blockers) != 1 || ev.Blockers[0].Name != BlockerDisabled {
		t.Errorf("blockers = %v, want [disabled]", blockerNames(ev))
	}
}

func TestMonitor_ProbeFailureIsOfflineNotIdle(t *testing.T) {
	m, prober, kuk, _, sd := newTestMonitor(t)
	prober.err = errors.New("no route to host")

	for i := 0; i < 5; i++ {
		m.RunOnce(context.Background())
	}

	if sd.count() != 0 {
		t.Fatal("shut down while Box was unreachable")
	}
	if kuk.count() != 0 {
		t.Error("scraped Kukátko even though the probe failed; offline is terminal")
	}
	ev := m.Snapshot().LastEvaluation
	if ev == nil || len(ev.Blockers) != 1 || ev.Blockers[0].Name != BlockerBoxOffline {
		t.Errorf("blockers = %v, want [box_offline]", ev)
	}
}

func TestMonitor_KukatkoFailureBlocksForever(t *testing.T) {
	m, _, kuk, _, sd := newTestMonitor(t)
	kuk.set(KukatkoReadings{StatusCode: 502, BodyExcerpt: "upstream is down"}, errors.New("bad gateway"))

	for i := 0; i < 5; i++ {
		m.RunOnce(context.Background())
	}

	if sd.count() != 0 {
		t.Fatal("shut down without a trustworthy Kukátko reading")
	}
	ev := m.Snapshot().LastEvaluation
	if ev == nil || ev.Kukatko == nil || ev.Kukatko.BodyExcerpt == "" {
		t.Error("the failing response detail must survive into the snapshot for the UI")
	}
}

func TestMonitor_LateTaskAbortsTheShutdown(t *testing.T) {
	m, _, _, act, sd := newTestMonitor(t)
	ctx := context.Background()

	m.RunOnce(ctx)
	m.RunOnce(ctx)

	// A task is claimed between the readings and the shutdown: the monitor
	// re-checks its own cheap, local conditions right before firing.
	m.beforeShutdown = func() { act.setTasks([]string{"a task that just started"}) }

	m.RunOnce(ctx)

	if sd.count() != 0 {
		t.Fatal("shut down even though a task started during the evaluation")
	}
	if got := m.Snapshot().Streak; got != 0 {
		t.Errorf("streak = %d after an abort, want 0", got)
	}
}

func TestMonitor_ShutdownErrorIsRecordedNotSwallowed(t *testing.T) {
	m, _, _, _, sd := newTestMonitor(t)
	sd.err = errors.New("permission denied")
	ctx := context.Background()

	m.RunOnce(ctx)
	m.RunOnce(ctx)
	m.RunOnce(ctx)

	if sd.count() != 1 {
		t.Fatalf("shutdown calls = %d, want 1", sd.count())
	}
	if m.Snapshot().LastError == "" {
		t.Error("a failed shutdown must be visible; LastError is empty")
	}
}

func TestMonitor_SnapshotKeepsRecentEvaluations(t *testing.T) {
	m, _, _, _, _ := newTestMonitor(t)

	for i := 0; i < recentEvaluations+5; i++ {
		m.RunOnce(context.Background())
	}

	if got := len(m.Snapshot().Recent); got != recentEvaluations {
		t.Fatalf("Recent = %d entries, want the ring to cap at %d", got, recentEvaluations)
	}
}

func TestMonitor_SnapshotCarriesTheCountdown(t *testing.T) {
	m, _, _, _, _ := newTestMonitor(t)
	m.setNextCheckAt(time.Now().Add(10 * time.Minute))

	m.RunOnce(context.Background())

	snap := m.Snapshot()
	if snap.EarliestShutdownAt == nil {
		t.Fatal("EarliestShutdownAt is nil after a clean check")
	}
	if !snap.EarliestShutdownAt.After(time.Now()) {
		t.Error("EarliestShutdownAt is in the past")
	}
}

func TestMonitor_StartStop(t *testing.T) {
	m, _, _, _, _ := newTestMonitor(t)

	m.Start()
	m.Stop()

	// Stop must be safe to reach without the first tick ever firing; the
	// firstTickDelay is far longer than this test runs.
	if m.Snapshot().RequiredChecks != testThresholds().IdleChecks {
		t.Error("snapshot unavailable after Stop")
	}
}
