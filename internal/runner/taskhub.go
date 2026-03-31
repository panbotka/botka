package runner

import (
	"sync"

	"github.com/google/uuid"

	"botka/internal/models"
)

// TaskEvent is emitted whenever a task's status changes.
type TaskEvent struct {
	TaskID    uuid.UUID         `json:"task_id"`
	Status    models.TaskStatus `json:"status"`
	ProjectID uuid.UUID         `json:"project_id"`
}

// TaskEventHub is a simple pub/sub for task status change events.
// All methods are safe for concurrent use.
type TaskEventHub struct {
	mu          sync.RWMutex
	subscribers map[chan TaskEvent]struct{}
}

// NewTaskEventHub creates a new TaskEventHub.
func NewTaskEventHub() *TaskEventHub {
	return &TaskEventHub{
		subscribers: make(map[chan TaskEvent]struct{}),
	}
}

// Subscribe returns a channel that receives task events and an unsubscribe function.
// The caller must call unsubscribe when done to avoid leaking the channel.
func (h *TaskEventHub) Subscribe() (<-chan TaskEvent, func()) {
	ch := make(chan TaskEvent, 16)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.subscribers, ch)
		h.mu.Unlock()
	}
}

// Publish sends an event to all current subscribers.
// Non-blocking: if a subscriber's buffer is full, the event is dropped for that subscriber.
func (h *TaskEventHub) Publish(event TaskEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}
