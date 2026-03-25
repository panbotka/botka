package claude

import (
	"context"
	"testing"
)

func TestProcessRegistry_RegisterAndList(t *testing.T) {
	r := &ProcessRegistry{entries: make(map[int64]*processEntry)}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Register(1, "Thread One", cancel)
	r.Register(2, "Thread Two", cancel)

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}

	found := map[int64]bool{}
	for _, p := range list {
		found[p.ThreadID] = true
		if p.StartedAt == "" {
			t.Error("expected non-empty StartedAt")
		}
	}
	if !found[1] || !found[2] {
		t.Error("expected both thread IDs in list")
	}
}

func TestProcessRegistry_Unregister(t *testing.T) {
	r := &ProcessRegistry{entries: make(map[int64]*processEntry)}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Register(1, "Thread One", cancel)
	r.Unregister(1)

	list := r.List()
	if len(list) != 0 {
		t.Fatalf("expected 0 entries after unregister, got %d", len(list))
	}
}

func TestProcessRegistry_Kill(t *testing.T) {
	r := &ProcessRegistry{entries: make(map[int64]*processEntry)}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Register(1, "Thread One", cancel)

	ok := r.Kill(1)
	if !ok {
		t.Error("expected Kill to return true for existing entry")
	}

	// Context should be cancelled
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled after Kill")
	}

	// Should be removed from registry
	list := r.List()
	if len(list) != 0 {
		t.Fatalf("expected 0 entries after kill, got %d", len(list))
	}
}

func TestProcessRegistry_KillNonexistent(t *testing.T) {
	r := &ProcessRegistry{entries: make(map[int64]*processEntry)}

	ok := r.Kill(999)
	if ok {
		t.Error("expected Kill to return false for nonexistent entry")
	}
}

func TestProcessRegistry_UnregisterNonexistent(t *testing.T) {
	r := &ProcessRegistry{entries: make(map[int64]*processEntry)}

	// Should not panic
	r.Unregister(999)
}

func TestProcessRegistry_KillAll(t *testing.T) {
	r := &ProcessRegistry{entries: make(map[int64]*processEntry)}

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	r.Register(1, "Thread One", cancel1)
	r.Register(2, "Thread Two", cancel2)

	count := r.KillAll()
	if count != 2 {
		t.Errorf("expected KillAll to return 2, got %d", count)
	}
	if ctx1.Err() == nil {
		t.Error("expected ctx1 to be cancelled")
	}
	if ctx2.Err() == nil {
		t.Error("expected ctx2 to be cancelled")
	}
	if len(r.List()) != 0 {
		t.Error("expected empty registry after KillAll")
	}
}

func TestProcessRegistry_KillAllEmpty(t *testing.T) {
	r := &ProcessRegistry{entries: make(map[int64]*processEntry)}

	count := r.KillAll()
	if count != 0 {
		t.Errorf("expected KillAll to return 0 on empty registry, got %d", count)
	}
}
