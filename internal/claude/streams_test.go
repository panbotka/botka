package claude

import (
	"sync"
	"testing"
)

func TestStreamBuffer_PublishAndSubscribe(t *testing.T) {
	sb := &StreamBuffer{buffers: make(map[int64]*threadBuffer)}
	sb.Start(1)

	// Publish some events before subscribing.
	sb.Publish(1, "data: {\"content\": \"hello\"}\n\n")
	sb.Publish(1, "data: {\"content\": \" world\"}\n\n")

	buffered, ch, ok := sb.Subscribe(1)
	if !ok {
		t.Fatal("expected subscribe to succeed")
	}
	if len(buffered) != 2 {
		t.Fatalf("expected 2 buffered events, got %d", len(buffered))
	}
	if buffered[0] != "data: {\"content\": \"hello\"}\n\n" {
		t.Errorf("unexpected first event: %q", buffered[0])
	}

	// Publish after subscribe — should arrive on channel.
	sb.Publish(1, "data: {\"content\": \"!\"}\n\n")
	ev := <-ch
	if ev != "data: {\"content\": \"!\"}\n\n" {
		t.Errorf("unexpected event from channel: %q", ev)
	}

	// Finish — channel should close.
	sb.Finish(1)
	_, open := <-ch
	if open {
		t.Error("expected channel to be closed after Finish")
	}
}

func TestStreamBuffer_SubscribeAfterFinish(t *testing.T) {
	sb := &StreamBuffer{buffers: make(map[int64]*threadBuffer)}
	sb.Start(1)
	sb.Publish(1, "data: {\"content\": \"done\"}\n\n")
	sb.Finish(1)

	buffered, ch, ok := sb.Subscribe(1)
	if !ok {
		t.Fatal("expected subscribe to succeed even after finish")
	}
	if len(buffered) != 1 {
		t.Fatalf("expected 1 buffered event, got %d", len(buffered))
	}

	// Channel should be immediately closed.
	_, open := <-ch
	if open {
		t.Error("expected channel to be closed for finished stream")
	}
}

func TestStreamBuffer_NoStream(t *testing.T) {
	sb := &StreamBuffer{buffers: make(map[int64]*threadBuffer)}
	_, _, ok := sb.Subscribe(99)
	if ok {
		t.Error("expected subscribe to fail for nonexistent stream")
	}
}

func TestStreamBuffer_Unsubscribe(t *testing.T) {
	sb := &StreamBuffer{buffers: make(map[int64]*threadBuffer)}
	sb.Start(1)
	_, ch, _ := sb.Subscribe(1)
	sb.Unsubscribe(1, ch)

	// Publish should not block or panic after unsubscribe.
	sb.Publish(1, "data: test\n\n")
	sb.Finish(1)
}

func TestStreamBuffer_MultipleSubscribers(t *testing.T) {
	sb := &StreamBuffer{buffers: make(map[int64]*threadBuffer)}
	sb.Start(1)

	_, ch1, _ := sb.Subscribe(1)
	_, ch2, _ := sb.Subscribe(1)

	sb.Publish(1, "data: broadcast\n\n")

	ev1 := <-ch1
	ev2 := <-ch2
	if ev1 != "data: broadcast\n\n" || ev2 != "data: broadcast\n\n" {
		t.Error("expected both subscribers to receive the event")
	}

	sb.Finish(1)
}

func TestStreamBuffer_StartReplacesExisting(t *testing.T) {
	sb := &StreamBuffer{buffers: make(map[int64]*threadBuffer)}
	sb.Start(1)
	_, ch, _ := sb.Subscribe(1)

	// Starting a new stream for the same thread should close the old subscriber.
	sb.Start(1)
	_, open := <-ch
	if open {
		t.Error("expected old subscriber channel to be closed on restart")
	}
}

func TestStreamBuffer_Remove(t *testing.T) {
	sb := &StreamBuffer{buffers: make(map[int64]*threadBuffer)}
	sb.Start(1)
	_, ch, _ := sb.Subscribe(1)

	sb.Remove(1)
	_, open := <-ch
	if open {
		t.Error("expected channel to be closed on Remove")
	}

	_, _, ok := sb.Subscribe(1)
	if ok {
		t.Error("expected subscribe to fail after Remove")
	}
}

func TestStreamBuffer_ConcurrentPublish(t *testing.T) {
	sb := &StreamBuffer{buffers: make(map[int64]*threadBuffer)}
	sb.Start(1)
	_, ch, _ := sb.Subscribe(1)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sb.Publish(1, "data: concurrent\n\n")
		}()
	}
	wg.Wait()

	// Drain channel.
	received := 0
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			received++
		default:
			goto done
		}
	}
done:
	if received != 100 {
		t.Errorf("expected 100 events, got %d", received)
	}

	sb.Finish(1)
}
