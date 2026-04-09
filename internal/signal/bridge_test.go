package signal

import (
	"context"
	"testing"
)

// TestBridge_HandleMessage_FilteringRules exercises the early filters inside
// handleMessage — direct messages, self-sent messages, and empty text are
// dropped before any dispatch. The bridge's lastTimestamps map is used as a
// signal for whether the message passed filtering (it is only updated for
// messages that would be dispatched).
func TestBridge_HandleMessage_FilteringRules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		msg      SignalMessage
		expected bool // true if the message should be accepted (dispatched)
	}{
		{
			name:     "direct message (no group) is skipped",
			msg:      SignalMessage{GroupID: "", Text: "hi", SourceNumber: "+1", Timestamp: 1},
			expected: false,
		},
		{
			name:     "bot's own number is skipped",
			msg:      SignalMessage{GroupID: "g1", SourceNumber: BotNumber, Text: "loop", Timestamp: 2},
			expected: false,
		},
		{
			name:     "empty text is skipped",
			msg:      SignalMessage{GroupID: "g1", SourceNumber: "+123", Text: "   ", Timestamp: 3},
			expected: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := NewBridge(BridgeConfig{})
			if err := b.handleMessage(context.Background(), tc.msg); err != nil {
				t.Fatalf("handleMessage returned error: %v", err)
			}
			b.mu.Lock()
			_, tracked := b.lastTimestamps[tc.msg.GroupID]
			b.mu.Unlock()
			if tracked != tc.expected {
				t.Errorf("lastTimestamps tracking = %v, want %v", tracked, tc.expected)
			}
		})
	}
}

// TestBridge_HandleMessage_Dedup ensures that a second message with a
// timestamp <= the previously-seen timestamp for the same group is not
// re-dispatched.
func TestBridge_HandleMessage_Dedup(t *testing.T) {
	t.Parallel()

	b := NewBridge(BridgeConfig{})
	// Pre-seed the tracker as if the first message was already processed.
	// (Calling handleMessage with a real message would spawn a goroutine that
	// hits a nil DB; seeding directly avoids that.)
	b.mu.Lock()
	b.lastTimestamps["g1"] = 100
	b.mu.Unlock()

	older := SignalMessage{
		GroupID:      "g1",
		SourceNumber: "+1",
		Text:         "older duplicate",
		Timestamp:    99,
	}
	if err := b.handleMessage(context.Background(), older); err != nil {
		t.Fatalf("handleMessage returned error: %v", err)
	}

	equal := SignalMessage{
		GroupID:      "g1",
		SourceNumber: "+1",
		Text:         "same timestamp",
		Timestamp:    100,
	}
	if err := b.handleMessage(context.Background(), equal); err != nil {
		t.Fatalf("handleMessage returned error: %v", err)
	}

	// Timestamp should still be the seeded value — neither older nor equal
	// should advance it.
	b.mu.Lock()
	got := b.lastTimestamps["g1"]
	b.mu.Unlock()
	if got != 100 {
		t.Errorf("lastTimestamps[g1] = %d, want 100 (duplicate should not advance)", got)
	}
}

// TestBridge_ThreadLock returns the same mutex for the same thread ID and a
// different mutex for a different thread ID. This guarantees per-thread
// serialization without blocking unrelated threads.
func TestBridge_ThreadLock(t *testing.T) {
	t.Parallel()

	b := NewBridge(BridgeConfig{})
	a1 := b.threadLock(1)
	a2 := b.threadLock(1)
	if a1 != a2 {
		t.Error("threadLock should return the same mutex for the same thread ID")
	}
	other := b.threadLock(2)
	if a1 == other {
		t.Error("threadLock should return distinct mutexes for different thread IDs")
	}
}

// TestBridge_ForwardToSignal_NilReceiver verifies that ForwardToSignal is a
// no-op on a nil *Bridge, which lets the chat handler call it unconditionally.
func TestBridge_ForwardToSignal_NilReceiver(t *testing.T) {
	t.Parallel()
	var b *Bridge
	// Must not panic.
	b.ForwardToSignal(context.Background(), 1, "user", "hello")
}
