package runner

import (
	"errors"
	"sync"
)

// Buffer is a thread-safe rolling ring buffer that implements io.Writer.
// It stores the most recent bytes up to a fixed capacity, discarding oldest
// data when full. Subscribers receive new data as it is written.
type Buffer struct {
	mu   sync.RWMutex
	data []byte
	cap  int
	head int // write position (next byte goes here)
	size int // current number of valid bytes

	closed      bool
	subscribers []subscriber
}

type subscriber struct {
	ch     chan []byte
	closed bool
}

// NewBuffer creates a new rolling buffer with the given byte capacity.
func NewBuffer(capacity int) *Buffer {
	return &Buffer{
		data: make([]byte, capacity),
		cap:  capacity,
	}
}

// Write appends p to the buffer, discarding oldest data if capacity is exceeded.
// It is safe for concurrent use. Returns an error if the buffer is closed.
func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return 0, errors.New("write to closed buffer")
	}

	n := len(p)
	src := p

	// If data is larger than capacity, only keep the tail
	if len(src) >= b.cap {
		src = src[len(src)-b.cap:]
		b.head = 0
		b.size = b.cap
		copy(b.data, src)
	} else {
		for len(src) > 0 {
			// How much space from head to end of slice
			space := b.cap - b.head
			toWrite := len(src)
			if toWrite > space {
				toWrite = space
			}
			copy(b.data[b.head:b.head+toWrite], src[:toWrite])
			b.head = (b.head + toWrite) % b.cap
			b.size += toWrite
			if b.size > b.cap {
				b.size = b.cap
			}
			src = src[toWrite:]
		}
	}

	// Notify subscribers (non-blocking)
	for i := range b.subscribers {
		if b.subscribers[i].closed {
			continue
		}
		select {
		case b.subscribers[i].ch <- copyBytes(p):
		default:
			// Skip slow consumers
		}
	}

	return n, nil
}

// ReadAll returns all buffered data in chronological order.
// The returned slice is a copy and safe to mutate.
func (b *Buffer) ReadAll() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.size == 0 {
		return nil
	}

	result := make([]byte, b.size)

	if b.size < b.cap {
		// No wrap — data starts at 0
		copy(result, b.data[:b.size])
	} else {
		// Buffer is full and has wrapped. head points to the oldest byte.
		firstLen := b.cap - b.head
		copy(result, b.data[b.head:b.head+firstLen])
		copy(result[firstLen:], b.data[:b.head])
	}

	return result
}

// Subscribe returns a channel that receives new data as it is written.
// Only data written after the call is delivered. Call the returned unsubscribe
// function to close the channel and stop receiving.
// If the buffer is already closed, the returned channel is closed immediately.
func (b *Buffer) Subscribe() (ch <-chan []byte, unsubscribe func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	c := make(chan []byte, 64)
	idx := len(b.subscribers)
	b.subscribers = append(b.subscribers, subscriber{ch: c, closed: b.closed})

	if b.closed {
		close(c)
	}

	return c, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if !b.subscribers[idx].closed {
			b.subscribers[idx].closed = true
			close(c)
		}
	}
}

// Len returns the number of bytes currently stored.
func (b *Buffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.size
}

// Close marks the buffer as closed and closes all subscriber channels.
// Subsequent writes will return an error.
func (b *Buffer) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for i := range b.subscribers {
		if !b.subscribers[i].closed {
			b.subscribers[i].closed = true
			close(b.subscribers[i].ch)
		}
	}
}

func copyBytes(p []byte) []byte {
	c := make([]byte, len(p))
	copy(c, p)
	return c
}
