package runner

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

func TestBufferIOWriterInterface(t *testing.T) {
	var _ io.Writer = (*Buffer)(nil)
}

func TestNewBuffer(t *testing.T) {
	b := NewBuffer(64)
	if b == nil {
		t.Fatal("NewBuffer returned nil")
	}
	if b.Len() != 0 {
		t.Fatalf("new buffer Len = %d, want 0", b.Len())
	}
}

func TestReadAllEmpty(t *testing.T) {
	b := NewBuffer(16)
	got := b.ReadAll()
	if got != nil {
		t.Fatalf("ReadAll on empty buffer = %v, want nil", got)
	}
}

func TestWriteReadAllBasic(t *testing.T) {
	b := NewBuffer(64)
	data := []byte("hello world")
	n, err := b.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write returned n=%d, want %d", n, len(data))
	}
	got := b.ReadAll()
	if !bytes.Equal(got, data) {
		t.Fatalf("ReadAll = %q, want %q", got, data)
	}
}

func TestLenTracking(t *testing.T) {
	b := NewBuffer(64)
	if b.Len() != 0 {
		t.Fatalf("initial Len = %d, want 0", b.Len())
	}
	b.Write([]byte("abc"))
	if b.Len() != 3 {
		t.Fatalf("after 3-byte write, Len = %d, want 3", b.Len())
	}
	b.Write([]byte("de"))
	if b.Len() != 5 {
		t.Fatalf("after 5 total bytes, Len = %d, want 5", b.Len())
	}
}

func TestCapacityExactFill(t *testing.T) {
	capacity := 8
	b := NewBuffer(capacity)
	data := []byte("12345678")
	b.Write(data)
	if b.Len() != capacity {
		t.Fatalf("Len = %d, want %d", b.Len(), capacity)
	}
	got := b.ReadAll()
	if !bytes.Equal(got, data) {
		t.Fatalf("ReadAll = %q, want %q", got, data)
	}
}

func TestCapacityNoOverflow(t *testing.T) {
	b := NewBuffer(16)
	data := []byte("short")
	b.Write(data)
	if b.Len() != len(data) {
		t.Fatalf("Len = %d, want %d", b.Len(), len(data))
	}
	got := b.ReadAll()
	if !bytes.Equal(got, data) {
		t.Fatalf("ReadAll = %q, want %q", got, data)
	}
}

func TestCapacityOverflow(t *testing.T) {
	capacity := 8
	b := NewBuffer(capacity)
	// Write more than capacity in a single write
	data := []byte("abcdefghijklmnop") // 16 bytes, cap=8
	n, err := b.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 16 {
		t.Fatalf("Write returned n=%d, want 16", n)
	}
	if b.Len() != capacity {
		t.Fatalf("Len = %d, want %d", b.Len(), capacity)
	}
	// Should keep the last 8 bytes
	got := b.ReadAll()
	want := []byte("ijklmnop")
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadAll = %q, want %q", got, want)
	}
}

func TestLargeWriteExceedingCapacityKeepsTail(t *testing.T) {
	tests := []struct {
		name  string
		cap   int
		write string
		want  string
	}{
		{"double capacity", 4, "abcdefgh", "efgh"},
		{"triple capacity", 3, "123456789", "789"},
		{"exact capacity", 5, "hello", "hello"},
		{"one over", 4, "abcde", "bcde"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuffer(tt.cap)
			b.Write([]byte(tt.write))
			got := string(b.ReadAll())
			if got != tt.want {
				t.Fatalf("ReadAll = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWrapAroundMultipleWrites(t *testing.T) {
	b := NewBuffer(8)
	// Fill up with 6 bytes
	b.Write([]byte("abcdef"))
	// Write 4 more, causing wrap: total 10 bytes, cap 8 -> keeps last 8
	b.Write([]byte("ghij"))
	if b.Len() != 8 {
		t.Fatalf("Len = %d, want 8", b.Len())
	}
	got := string(b.ReadAll())
	want := "cdefghij"
	if got != want {
		t.Fatalf("ReadAll = %q, want %q", got, want)
	}
}

func TestMultipleSmallWrites(t *testing.T) {
	b := NewBuffer(16)
	writes := []string{"He", "ll", "o,", " W", "or", "ld", "!"}
	for _, w := range writes {
		b.Write([]byte(w))
	}
	got := string(b.ReadAll())
	want := "Hello, World!"
	if got != want {
		t.Fatalf("ReadAll = %q, want %q", got, want)
	}
	if b.Len() != len(want) {
		t.Fatalf("Len = %d, want %d", b.Len(), len(want))
	}
}

func TestMultipleSmallWritesWithOverflow(t *testing.T) {
	b := NewBuffer(5)
	for _, ch := range "abcdefghij" {
		b.Write([]byte(string(ch)))
	}
	got := string(b.ReadAll())
	want := "fghij"
	if got != want {
		t.Fatalf("ReadAll = %q, want %q", got, want)
	}
}

func TestWriteToClosedBuffer(t *testing.T) {
	b := NewBuffer(16)
	b.Write([]byte("before"))
	b.Close()
	n, err := b.Write([]byte("after"))
	if err == nil {
		t.Fatal("expected error writing to closed buffer, got nil")
	}
	if n != 0 {
		t.Fatalf("Write to closed buffer returned n=%d, want 0", n)
	}
	// Data from before close should still be readable
	got := string(b.ReadAll())
	if got != "before" {
		t.Fatalf("ReadAll after close = %q, want %q", got, "before")
	}
}

func TestCloseIdempotent(t *testing.T) {
	b := NewBuffer(16)
	b.Write([]byte("data"))
	ch, unsub := b.Subscribe()
	_ = ch

	// Close multiple times -- must not panic
	b.Close()
	b.Close()
	b.Close()

	// Unsubscribe after close -- must not panic
	unsub()
}

func TestSubscribeReceivesNewData(t *testing.T) {
	b := NewBuffer(64)
	ch, unsub := b.Subscribe()
	defer unsub()

	b.Write([]byte("hello"))
	got := <-ch
	if !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("subscriber got %q, want %q", got, "hello")
	}

	b.Write([]byte(" world"))
	got = <-ch
	if !bytes.Equal(got, []byte(" world")) {
		t.Fatalf("subscriber got %q, want %q", got, " world")
	}
}

func TestSubscribeMissesOldData(t *testing.T) {
	b := NewBuffer(64)
	b.Write([]byte("old data"))

	ch, unsub := b.Subscribe()
	defer unsub()

	// Write new data
	b.Write([]byte("new"))

	got := <-ch
	if !bytes.Equal(got, []byte("new")) {
		t.Fatalf("subscriber got %q, want %q (should miss old data)", got, "new")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	b := NewBuffer(64)
	ch1, unsub1 := b.Subscribe()
	defer unsub1()
	ch2, unsub2 := b.Subscribe()
	defer unsub2()

	data := []byte("broadcast")
	b.Write(data)

	got1 := <-ch1
	got2 := <-ch2
	if !bytes.Equal(got1, data) {
		t.Fatalf("subscriber 1 got %q, want %q", got1, data)
	}
	if !bytes.Equal(got2, data) {
		t.Fatalf("subscriber 2 got %q, want %q", got2, data)
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	b := NewBuffer(64)
	ch, unsub := b.Subscribe()
	unsub()

	// Channel should be closed; receive should yield zero value and ok=false
	_, ok := <-ch
	if ok {
		t.Fatal("channel should be closed after unsubscribe")
	}
}

func TestUnsubscribeIdempotent(t *testing.T) {
	b := NewBuffer(64)
	_, unsub := b.Subscribe()

	// Call unsubscribe multiple times -- must not panic
	unsub()
	unsub()
	unsub()
	_ = b
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := NewBuffer(64)
	ch, unsub := b.Subscribe()
	unsub()

	// Write after unsubscribe -- should not panic or block
	b.Write([]byte("data"))

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("expected closed channel after unsubscribe")
	}
}

func TestSubscribeOnClosedBuffer(t *testing.T) {
	b := NewBuffer(64)
	b.Write([]byte("data"))
	b.Close()

	ch, unsub := b.Subscribe()
	defer unsub()

	// Channel should already be closed
	_, ok := <-ch
	if ok {
		t.Fatal("channel from Subscribe on closed buffer should be closed")
	}
}

func TestCloseClosesAllSubscriberChannels(t *testing.T) {
	b := NewBuffer(64)
	ch1, _ := b.Subscribe()
	ch2, _ := b.Subscribe()
	ch3, _ := b.Subscribe()

	b.Close()

	for i, ch := range []<-chan []byte{ch1, ch2, ch3} {
		_, ok := <-ch
		if ok {
			t.Fatalf("subscriber %d channel should be closed after Close", i+1)
		}
	}
}

func TestSubscriberReceivesCopyNotOriginal(t *testing.T) {
	b := NewBuffer(64)
	ch, unsub := b.Subscribe()
	defer unsub()

	original := []byte("hello")
	b.Write(original)

	got := <-ch
	// Mutate what subscriber received -- should not affect buffer
	got[0] = 'X'
	buffered := b.ReadAll()
	if buffered[0] != 'h' {
		t.Fatal("subscriber mutation affected buffer data")
	}
}

func TestReadAllReturnsCopy(t *testing.T) {
	b := NewBuffer(64)
	b.Write([]byte("hello"))

	first := b.ReadAll()
	first[0] = 'X'

	second := b.ReadAll()
	if second[0] != 'h' {
		t.Fatal("ReadAll did not return an independent copy")
	}
}

func TestConcurrentWriteRead(t *testing.T) {
	b := NewBuffer(256)
	var wg sync.WaitGroup
	const writers = 5
	const writesPerWriter = 100

	// Concurrent writers
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				b.Write([]byte("data"))
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				_ = b.ReadAll()
				_ = b.Len()
			}
		}()
	}

	wg.Wait()

	// Buffer should still be consistent
	l := b.Len()
	if l > 256 {
		t.Fatalf("Len = %d exceeds capacity 256", l)
	}
	data := b.ReadAll()
	if len(data) != l {
		t.Fatalf("ReadAll length = %d, Len = %d, mismatch", len(data), l)
	}
}

func TestConcurrentSubscribeWrite(t *testing.T) {
	b := NewBuffer(128)
	var wg sync.WaitGroup

	// Start subscribers
	const numSubs = 5
	channels := make([]<-chan []byte, numSubs)
	unsubs := make([]func(), numSubs)
	for i := 0; i < numSubs; i++ {
		channels[i], unsubs[i] = b.Subscribe()
	}

	// Writer goroutine
	const numWrites = 50
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numWrites; i++ {
			b.Write([]byte("x"))
		}
		b.Close()
	}()

	// Reader goroutines drain their channels
	for i := 0; i < numSubs; i++ {
		wg.Add(1)
		go func(ch <-chan []byte) {
			defer wg.Done()
			count := 0
			for range ch {
				count++
			}
			// Should have received some data (channel buffer=64, so may drop some)
			if count == 0 {
				t.Error("subscriber received zero messages")
			}
		}(channels[i])
	}

	wg.Wait()

	// Clean up unsubs -- should not panic after close
	for _, fn := range unsubs {
		fn()
	}
}

func TestWriteEmptySlice(t *testing.T) {
	b := NewBuffer(16)
	n, err := b.Write([]byte{})
	if err != nil {
		t.Fatalf("Write empty slice error: %v", err)
	}
	if n != 0 {
		t.Fatalf("Write empty slice returned n=%d, want 0", n)
	}
	if b.Len() != 0 {
		t.Fatalf("Len after empty write = %d, want 0", b.Len())
	}
	got := b.ReadAll()
	if got != nil {
		t.Fatalf("ReadAll after empty write = %v, want nil", got)
	}
}

func TestWrapAroundReadAllOrder(t *testing.T) {
	// Verify that ReadAll returns data in chronological order after wrap
	b := NewBuffer(6)
	b.Write([]byte("abc"))   // buffer: [a b c _ _ _], head=3, size=3
	b.Write([]byte("defgh")) // overflow: total 8 bytes, cap 6 -> keeps last 6
	// After these writes, buffer should contain "cdefgh"
	got := string(b.ReadAll())
	want := "cdefgh"
	if got != want {
		t.Fatalf("ReadAll = %q, want %q", got, want)
	}
}

func TestWriteExactlyDoubleCapacity(t *testing.T) {
	b := NewBuffer(4)
	// Write exactly 2x capacity
	b.Write([]byte("abcdefgh"))
	got := string(b.ReadAll())
	want := "efgh"
	if got != want {
		t.Fatalf("ReadAll = %q, want %q", got, want)
	}
}
