package handlers

import (
	"context"
	"strings"
	"sync"
	"testing"

	"botka/internal/models"
	"botka/internal/push"
)

// fakePushNotifier captures every Send/Broadcast call so tests can assert
// the exact payloads triggered by chat events.
type fakePushNotifier struct {
	mu         sync.Mutex
	sends      []sendCall
	broadcasts []push.PushPayload
}

type sendCall struct {
	UserID  int64
	Payload push.PushPayload
}

func (f *fakePushNotifier) Send(_ context.Context, userID int64, payload push.PushPayload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, sendCall{UserID: userID, Payload: payload})
	return nil
}

func (f *fakePushNotifier) Broadcast(_ context.Context, payload push.PushPayload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broadcasts = append(f.broadcasts, payload)
	return nil
}

func (f *fakePushNotifier) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends) + len(f.broadcasts)
}

func threadFixture(id int64, title string) *models.Thread {
	return &models.Thread{ID: id, Title: title}
}

func TestNotifyAssistantReply_BroadcastsWithTitle(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	thread := threadFixture(42, "Hello world")

	notifyAssistantReply(fake, true, thread, "the body", false)

	if len(fake.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(fake.broadcasts))
	}
	got := fake.broadcasts[0]
	if got.Title != "Hello world" {
		t.Errorf("title = %q", got.Title)
	}
	if got.URL != "/chat/42" {
		t.Errorf("url = %q", got.URL)
	}
	if got.Tag != "thread-42" {
		t.Errorf("tag = %q", got.Tag)
	}
	if got.Body != "the body" {
		t.Errorf("body = %q", got.Body)
	}
}

func TestNotifyAssistantReply_FallbackTitleForUntitledThread(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	thread := threadFixture(7, "")

	notifyAssistantReply(fake, true, thread, "hi", false)

	if len(fake.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(fake.broadcasts))
	}
	if fake.broadcasts[0].Title != chatPushDefaultName {
		t.Errorf("title = %q, want %q", fake.broadcasts[0].Title, chatPushDefaultName)
	}
}

func TestNotifyAssistantReply_StripsMarkdown(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	thread := threadFixture(1, "t")
	body := "# Heading\n- bullet **bold** with `code` and a [link](https://example.com)"

	notifyAssistantReply(fake, true, thread, body, false)

	if len(fake.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(fake.broadcasts))
	}
	got := fake.broadcasts[0].Body
	for _, banned := range []string{"#", "**", "`", "](", "- "} {
		if strings.Contains(got, banned) {
			t.Errorf("body still contains %q: %q", banned, got)
		}
	}
	if !strings.Contains(got, "Heading") || !strings.Contains(got, "bullet") || !strings.Contains(got, "link") {
		t.Errorf("body lost meaningful text: %q", got)
	}
}

func TestNotifyAssistantReply_Truncates(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	thread := threadFixture(1, "t")
	long := strings.Repeat("a", 200)

	notifyAssistantReply(fake, true, thread, long, false)

	if len(fake.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(fake.broadcasts))
	}
	got := []rune(fake.broadcasts[0].Body)
	want := chatPushBodyMaxRune + len([]rune("…"))
	if len(got) != want {
		t.Errorf("body rune len = %d, want %d", len(got), want)
	}
	if !strings.HasSuffix(fake.broadcasts[0].Body, "…") {
		t.Errorf("expected ellipsis suffix, got %q", fake.broadcasts[0].Body)
	}
}

func TestNotifyAssistantReply_SkipsWhenClientConnected(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	thread := threadFixture(1, "t")

	notifyAssistantReply(fake, true, thread, "hi", true)

	if got := fake.total(); got != 0 {
		t.Errorf("expected no push (client still connected), got %d", got)
	}
}

func TestNotifyAssistantReply_SkipsWhenDisabled(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	thread := threadFixture(1, "t")

	notifyAssistantReply(fake, false, thread, "hi", false)

	if got := fake.total(); got != 0 {
		t.Errorf("expected no push when disabled, got %d", got)
	}
}

func TestNotifyAssistantReply_NilNotifierIsNoOp(t *testing.T) {
	t.Parallel()
	thread := threadFixture(1, "t")
	notifyAssistantReply(nil, true, thread, "hi", false)
	// Pass: no panic.
}

func TestStripMarkdown_CollapsesWhitespace(t *testing.T) {
	t.Parallel()
	in := "line1\n\n\nline2  \n  line3"
	got := stripMarkdown(in)
	if strings.Contains(got, "\n") {
		t.Errorf("expected no newlines, got %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Errorf("expected single spaces, got %q", got)
	}
}

func TestStripMarkdown_DropsCodeFenceLines(t *testing.T) {
	t.Parallel()
	in := "before\n```go\nfunc main() {}\n```\nafter"
	got := stripMarkdown(in)
	if strings.Contains(got, "```") {
		t.Errorf("code fence not stripped: %q", got)
	}
	if !strings.Contains(got, "func main()") {
		t.Errorf("body lost code text: %q", got)
	}
}
