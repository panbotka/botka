package box

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRunner returns a runCmd function that behaves according to the probe
// results list. Each call dequeues the next entry; when exhausted, the last
// entry is repeated.
type fakeRunner struct {
	probeCalls atomic.Int32
	wakeCalls  atomic.Int32
	wolCommand string
	// Per-probe behavior: nil = success, non-nil = error.
	probeResults []error
}

// run is the runCmd hook installed on the Waker.
func (f *fakeRunner) run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if name == "ssh" {
		idx := int(f.probeCalls.Add(1)) - 1
		if idx >= len(f.probeResults) {
			idx = len(f.probeResults) - 1
		}
		if idx < 0 {
			return nil, errors.New("no probe results configured")
		}
		return nil, f.probeResults[idx]
	}
	if name == f.wolCommand {
		f.wakeCalls.Add(1)
		return nil, nil
	}
	return nil, errors.New("unexpected command: " + name)
}

// newTestWaker builds a Waker wired up to a fakeRunner with tight timing.
func newTestWaker(fr *fakeRunner) *Waker {
	w := NewWaker("box", "box", "boxon")
	fr.wolCommand = "boxon"
	w.runCmd = fr.run
	w.probeTimeout = 50 * time.Millisecond
	w.wakeTimeout = 200 * time.Millisecond
	w.pollInterval = 20 * time.Millisecond
	w.cacheTTL = 100 * time.Millisecond
	return w
}

func TestSSHTarget(t *testing.T) {
	t.Parallel()
	w := NewWaker("boxhost", "boxuser", "")
	if got, want := w.SSHTarget(), "boxuser@boxhost"; got != want {
		t.Errorf("SSHTarget() = %q, want %q", got, want)
	}
}

func TestEnsureUp_AlreadyAlive(t *testing.T) {
	t.Parallel()
	fr := &fakeRunner{probeResults: []error{nil}}
	w := newTestWaker(fr)
	if err := w.EnsureUp(context.Background()); err != nil {
		t.Fatalf("EnsureUp() error = %v", err)
	}
	if got := fr.probeCalls.Load(); got != 1 {
		t.Errorf("probe calls = %d, want 1", got)
	}
	if got := fr.wakeCalls.Load(); got != 0 {
		t.Errorf("wake calls = %d, want 0", got)
	}
}

func TestEnsureUp_CacheSkipsProbe(t *testing.T) {
	t.Parallel()
	fr := &fakeRunner{probeResults: []error{nil}}
	w := newTestWaker(fr)
	if err := w.EnsureUp(context.Background()); err != nil {
		t.Fatalf("first EnsureUp() error = %v", err)
	}
	if err := w.EnsureUp(context.Background()); err != nil {
		t.Fatalf("cached EnsureUp() error = %v", err)
	}
	if got := fr.probeCalls.Load(); got != 1 {
		t.Errorf("probe calls = %d, want 1 (cache miss)", got)
	}
}

func TestEnsureUp_WakesWhenDown(t *testing.T) {
	t.Parallel()
	// First probe fails, all subsequent probes succeed.
	fr := &fakeRunner{probeResults: []error{errors.New("down"), nil}}
	w := newTestWaker(fr)
	if err := w.EnsureUp(context.Background()); err != nil {
		t.Fatalf("EnsureUp() error = %v", err)
	}
	if got := fr.wakeCalls.Load(); got != 1 {
		t.Errorf("wake calls = %d, want 1", got)
	}
}

func TestEnsureUp_TimeoutWhenUnreachable(t *testing.T) {
	t.Parallel()
	fr := &fakeRunner{probeResults: []error{errors.New("down")}}
	w := newTestWaker(fr)
	err := w.EnsureUp(context.Background())
	if !errors.Is(err, ErrBoxUnreachable) {
		t.Errorf("EnsureUp() error = %v, want ErrBoxUnreachable", err)
	}
}

func TestInvalidate_ForcesReprobe(t *testing.T) {
	t.Parallel()
	fr := &fakeRunner{probeResults: []error{nil, nil}}
	w := newTestWaker(fr)
	if err := w.EnsureUp(context.Background()); err != nil {
		t.Fatalf("first EnsureUp() error = %v", err)
	}
	w.Invalidate()
	if err := w.EnsureUp(context.Background()); err != nil {
		t.Fatalf("second EnsureUp() error = %v", err)
	}
	if got := fr.probeCalls.Load(); got != 2 {
		t.Errorf("probe calls after invalidate = %d, want 2", got)
	}
}

func TestEnsureUp_CacheExpires(t *testing.T) {
	t.Parallel()
	fr := &fakeRunner{probeResults: []error{nil, nil}}
	w := newTestWaker(fr)
	if err := w.EnsureUp(context.Background()); err != nil {
		t.Fatalf("first EnsureUp() error = %v", err)
	}
	// Wait just past cacheTTL (100ms) to force a re-probe.
	time.Sleep(150 * time.Millisecond)
	if err := w.EnsureUp(context.Background()); err != nil {
		t.Fatalf("second EnsureUp() error = %v", err)
	}
	if got := fr.probeCalls.Load(); got != 2 {
		t.Errorf("probe calls after TTL = %d, want 2", got)
	}
}
