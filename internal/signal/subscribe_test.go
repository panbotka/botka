package signal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- parseSSEStream ---

func TestParseSSEStream_SingleEnvelopeNotification(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`data: {"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+1111","sourceNumber":"+1111","sourceName":"Alice","timestamp":1712650000000,"dataMessage":{"timestamp":1712650000000,"message":"hi","groupInfo":{"groupId":"g1","type":"DELIVER"}}},"account":"+9999"}}

`)
	var got []SignalMessage
	err := parseSSEStream(context.Background(), stream, func(_ context.Context, msg SignalMessage) error {
		got = append(got, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("handler called %d times, want 1", len(got))
	}
	if got[0].Text != "hi" || got[0].GroupID != "g1" || got[0].SourceName != "Alice" {
		t.Errorf("unexpected message: %+v", got[0])
	}
}

func TestParseSSEStream_BareEnvelopeParams(t *testing.T) {
	t.Parallel()
	// Older signal-cli shape: params IS the envelope, not {"envelope": ...}.
	stream := strings.NewReader(`data: {"jsonrpc":"2.0","method":"receive","params":{"source":"+2222","sourceNumber":"+2222","timestamp":42,"dataMessage":{"message":"yo"}}}

`)
	var got []SignalMessage
	err := parseSSEStream(context.Background(), stream, func(_ context.Context, msg SignalMessage) error {
		got = append(got, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	if len(got) != 1 || got[0].Text != "yo" || got[0].SourceNumber != "+2222" {
		t.Errorf("unexpected messages: %+v", got)
	}
}

func TestParseSSEStream_MultipleEvents(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`data: {"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+1","timestamp":1,"dataMessage":{"message":"first"}}}}

data: {"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+2","timestamp":2,"dataMessage":{"message":"second"}}}}

`)
	var got []SignalMessage
	err := parseSSEStream(context.Background(), stream, func(_ context.Context, msg SignalMessage) error {
		got = append(got, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("handler called %d times, want 2", len(got))
	}
	if got[0].Text != "first" || got[1].Text != "second" {
		t.Errorf("unexpected message ordering: %+v", got)
	}
}

func TestParseSSEStream_IgnoresNonReceiveMethods(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`data: {"jsonrpc":"2.0","method":"syncMessage","params":{"foo":"bar"}}

data: {"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+1","timestamp":1,"dataMessage":{"message":"ok"}}}}

`)
	var got []SignalMessage
	err := parseSSEStream(context.Background(), stream, func(_ context.Context, msg SignalMessage) error {
		got = append(got, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	if len(got) != 1 || got[0].Text != "ok" {
		t.Errorf("unexpected messages: %+v", got)
	}
}

func TestParseSSEStream_IgnoresCommentAndEventLines(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`: keepalive comment
event: heartbeat
data: ignored-heartbeat

data: {"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+1","timestamp":1,"dataMessage":{"message":"real"}}}}

`)
	var got []SignalMessage
	err := parseSSEStream(context.Background(), stream, func(_ context.Context, msg SignalMessage) error {
		got = append(got, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	if len(got) != 1 || got[0].Text != "real" {
		t.Errorf("unexpected messages: %+v", got)
	}
}

func TestParseSSEStream_MalformedDataIgnored(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`data: not-json

data: {"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+1","timestamp":1,"dataMessage":{"message":"good"}}}}

`)
	var got []SignalMessage
	err := parseSSEStream(context.Background(), stream, func(_ context.Context, msg SignalMessage) error {
		got = append(got, msg)
		return nil
	})
	if err != nil {
		t.Fatalf("parseSSEStream() error = %v", err)
	}
	if len(got) != 1 || got[0].Text != "good" {
		t.Errorf("unexpected messages: %+v", got)
	}
}

func TestParseSSEStream_HandlerErrorAborts(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`data: {"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+1","timestamp":1,"dataMessage":{"message":"a"}}}}

data: {"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+2","timestamp":2,"dataMessage":{"message":"b"}}}}

`)
	want := errors.New("boom")
	var count int
	err := parseSSEStream(context.Background(), stream, func(_ context.Context, _ SignalMessage) error {
		count++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected handler error, got %v", err)
	}
	if count != 1 {
		t.Errorf("handler called %d times, want 1", count)
	}
}

func TestParseSSEStream_NilHandlerErrors(t *testing.T) {
	t.Parallel()
	c := NewClient("http://unused")
	err := c.Subscribe(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

// --- Subscribe end-to-end ---

func TestSubscribe_SSEServer(t *testing.T) {
	t.Parallel()

	// SSE server that pushes two envelopes then holds the connection.
	events := []string{
		`{"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+1","sourceNumber":"+1","sourceName":"Alice","timestamp":100,"dataMessage":{"timestamp":100,"message":"one","groupInfo":{"groupId":"g","type":"DELIVER"}}}}}`,
		`{"jsonrpc":"2.0","method":"receive","params":{"envelope":{"source":"+2","sourceNumber":"+2","sourceName":"Bob","timestamp":200,"dataMessage":{"timestamp":200,"message":"two"}}}}`,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != eventsPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("ResponseWriter is not a Flusher")
			return
		}
		for _, ev := range events {
			if _, err := fmt.Fprintf(w, "data: %s\n\n", ev); err != nil {
				return
			}
			flusher.Flush()
		}
		// Hold until the client disconnects via context cancellation.
		<-r.Context().Done()
	}))
	defer ts.Close()

	c := NewClient(ts.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu   sync.Mutex
		msgs []SignalMessage
		done = make(chan error, 1)
	)
	go func() {
		done <- c.Subscribe(ctx, func(_ context.Context, msg SignalMessage) error {
			mu.Lock()
			msgs = append(msgs, msg)
			gotBoth := len(msgs) == 2
			mu.Unlock()
			if gotBoth {
				cancel()
			}
			return nil
		})
	}()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Subscribe() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("Subscribe() did not return within 5s")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Text != "one" || msgs[0].SourceName != "Alice" || msgs[0].GroupID != "g" {
		t.Errorf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Text != "two" || msgs[1].SourceName != "Bob" {
		t.Errorf("unexpected second message: %+v", msgs[1])
	}
}

func TestSubscribe_ConnectionRefused(t *testing.T) {
	t.Parallel()
	c := NewClient("http://127.0.0.1:1")
	err := c.Subscribe(context.Background(), func(_ context.Context, _ SignalMessage) error { return nil })
	if !errors.Is(err, ErrDaemonUnreachable) {
		t.Fatalf("expected ErrDaemonUnreachable, got %v", err)
	}
}

func TestSubscribe_Non200Status(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	err := c.Subscribe(context.Background(), func(_ context.Context, _ SignalMessage) error { return nil })
	if err == nil {
		t.Fatal("expected error for bad status")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error should mention status code: %v", err)
	}
}

// --- MessageFromEnvelope ---

func TestMessageFromEnvelope_FallbackSourceNumber(t *testing.T) {
	t.Parallel()
	env := Envelope{
		Source:    "+999",
		Timestamp: 100,
		DataMessage: &DataMessage{
			Message: "hey",
		},
	}
	msg := MessageFromEnvelope(env)
	if msg.SourceNumber != "+999" {
		t.Errorf("SourceNumber = %q, want %q", msg.SourceNumber, "+999")
	}
	if msg.Text != "hey" {
		t.Errorf("Text = %q, want %q", msg.Text, "hey")
	}
}

func TestMessageFromEnvelope_NoDataMessage(t *testing.T) {
	t.Parallel()
	env := Envelope{
		SourceNumber: "+111",
		Timestamp:    50,
	}
	msg := MessageFromEnvelope(env)
	if msg.SourceNumber != "+111" || msg.Text != "" || msg.GroupID != "" {
		t.Errorf("unexpected msg: %+v", msg)
	}
}
