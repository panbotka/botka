package claude

import (
	"context"
	"sync"
	"time"
)

// ProcessInfo is the JSON-safe view of a running process.
type ProcessInfo struct {
	ThreadID    int64  `json:"thread_id"`
	ThreadTitle string `json:"thread_title"`
	StartedAt   string `json:"started_at"`
	DurationSec int    `json:"duration_sec"`
}

type processEntry struct {
	threadID    int64
	threadTitle string
	startedAt   time.Time
	cancel      context.CancelFunc
}

// ProcessRegistry tracks active Claude Code subprocesses.
type ProcessRegistry struct {
	mu      sync.RWMutex
	entries map[int64]*processEntry
}

// Registry is the package-level singleton.
var Registry = &ProcessRegistry{entries: make(map[int64]*processEntry)}

// Register adds a running process to the registry.
func (r *ProcessRegistry) Register(threadID int64, title string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[threadID] = &processEntry{
		threadID:    threadID,
		threadTitle: title,
		startedAt:   time.Now(),
		cancel:      cancel,
	}
}

// Unregister removes a process from the registry.
func (r *ProcessRegistry) Unregister(threadID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, threadID)
}

// List returns a snapshot of all active processes.
func (r *ProcessRegistry) List() []ProcessInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	out := make([]ProcessInfo, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, ProcessInfo{
			ThreadID:    e.threadID,
			ThreadTitle: e.threadTitle,
			StartedAt:   e.startedAt.Format(time.RFC3339),
			DurationSec: int(now.Sub(e.startedAt).Seconds()),
		})
	}
	return out
}

// Kill cancels a running process and removes it from the registry.
// Returns true if the process was found.
func (r *ProcessRegistry) Kill(threadID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[threadID]
	if !ok {
		return false
	}
	e.cancel()
	delete(r.entries, threadID)
	return true
}

// KillAll cancels all running processes and clears the registry.
// Returns the number of processes killed.
func (r *ProcessRegistry) KillAll() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := len(r.entries)
	for id, e := range r.entries {
		e.cancel()
		delete(r.entries, id)
	}
	return count
}
