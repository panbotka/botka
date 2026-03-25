package claude

import "sync"

// StreamBuffer manages per-thread SSE event buffers so that late-joining
// clients (e.g. after navigating away and back) can replay missed events
// and subscribe to new ones.
type StreamBuffer struct {
	mu      sync.Mutex
	buffers map[int64]*threadBuffer
}

type threadBuffer struct {
	mu     sync.Mutex
	events []string
	subs   []chan string
	done   bool
}

// Streams is the package-level singleton for stream buffering.
var Streams = &StreamBuffer{buffers: make(map[int64]*threadBuffer)}

// Start creates a new stream buffer for the given thread, replacing any
// existing one.
func (s *StreamBuffer) Start(threadID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Close subscribers of any previous buffer for this thread.
	if old, ok := s.buffers[threadID]; ok {
		old.mu.Lock()
		for _, ch := range old.subs {
			close(ch)
		}
		old.subs = nil
		old.mu.Unlock()
	}
	s.buffers[threadID] = &threadBuffer{}
}

// Publish appends an SSE-formatted event to the buffer and sends it to all
// active subscribers.
func (s *StreamBuffer) Publish(threadID int64, sseData string) {
	s.mu.Lock()
	tb, ok := s.buffers[threadID]
	s.mu.Unlock()
	if !ok {
		return
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.done {
		return
	}
	tb.events = append(tb.events, sseData)
	for _, ch := range tb.subs {
		select {
		case ch <- sseData:
		default:
			// Subscriber too slow — skip to avoid blocking the stream.
		}
	}
}

// Finish marks the stream as done and closes all subscriber channels.
func (s *StreamBuffer) Finish(threadID int64) {
	s.mu.Lock()
	tb, ok := s.buffers[threadID]
	s.mu.Unlock()
	if !ok {
		return
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.done = true
	for _, ch := range tb.subs {
		close(ch)
	}
	tb.subs = nil
}

// Subscribe returns the buffered events so far and a channel for new events.
// The channel is closed when the stream finishes. Returns ok=false if no
// active stream exists for the thread.
func (s *StreamBuffer) Subscribe(threadID int64) (buffered []string, ch chan string, ok bool) {
	s.mu.Lock()
	tb, exists := s.buffers[threadID]
	s.mu.Unlock()
	if !exists {
		return nil, nil, false
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Copy buffered events.
	buffered = make([]string, len(tb.events))
	copy(buffered, tb.events)

	if tb.done {
		// Stream already finished — return buffer but no channel.
		ch = make(chan string)
		close(ch)
		return buffered, ch, true
	}

	ch = make(chan string, 256)
	tb.subs = append(tb.subs, ch)
	return buffered, ch, true
}

// Unsubscribe removes a subscriber channel.
func (s *StreamBuffer) Unsubscribe(threadID int64, ch chan string) {
	s.mu.Lock()
	tb, ok := s.buffers[threadID]
	s.mu.Unlock()
	if !ok {
		return
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()
	for i, sub := range tb.subs {
		if sub == ch {
			tb.subs = append(tb.subs[:i], tb.subs[i+1:]...)
			return
		}
	}
}

// Remove deletes the buffer for a thread entirely.
func (s *StreamBuffer) Remove(threadID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tb, ok := s.buffers[threadID]; ok {
		tb.mu.Lock()
		for _, ch := range tb.subs {
			close(ch)
		}
		tb.subs = nil
		tb.mu.Unlock()
		delete(s.buffers, threadID)
	}
}
