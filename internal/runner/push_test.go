package runner

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"botka/internal/config"
	"botka/internal/models"
	"botka/internal/push"
)

// fakePushNotifier captures every Send/Broadcast call so tests can assert
// the exact payloads triggered by runner state transitions.
type fakePushNotifier struct {
	mu         sync.Mutex
	sends      []sendCall
	broadcasts []push.PushPayload
	sendErr    error
}

type sendCall struct {
	UserID  int64
	Payload push.PushPayload
}

func (f *fakePushNotifier) Send(_ context.Context, userID int64, payload push.PushPayload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, sendCall{UserID: userID, Payload: payload})
	return f.sendErr
}

func (f *fakePushNotifier) Broadcast(_ context.Context, payload push.PushPayload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broadcasts = append(f.broadcasts, payload)
	return f.sendErr
}

func (f *fakePushNotifier) totalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sends) + len(f.broadcasts)
}

func runnerWithFakePush(notifier PushNotifier, enabled bool) *Runner {
	return &Runner{
		config:       &config.Config{PushNotificationsEnabled: enabled},
		pushNotifier: notifier,
	}
}

func taskWithReason(title, reason string) *models.Task {
	return &models.Task{
		ID:            uuid.New(),
		Title:         title,
		FailureReason: ptrString(reason),
	}
}

func ptrString(s string) *string { return &s }

func TestNotifyTaskTransition_FailedSendsBroadcast(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, true)
	task := taskWithReason("clean up logs", "git push refused")

	r.notifyTaskTransition(task, models.TaskStatusFailed)

	if got := len(fake.broadcasts); got != 1 {
		t.Fatalf("expected 1 broadcast, got %d", got)
	}
	if len(fake.sends) != 0 {
		t.Errorf("expected no Send calls, got %d", len(fake.sends))
	}

	got := fake.broadcasts[0]
	if got.Title != "Task failed: clean up logs" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Body != "git push refused" {
		t.Errorf("body = %q", got.Body)
	}
	if got.URL != "/tasks/"+task.ID.String() {
		t.Errorf("url = %q", got.URL)
	}
	if got.Tag != "task-"+task.ID.String() {
		t.Errorf("tag = %q", got.Tag)
	}
}

func TestNotifyTaskTransition_FailedPrefersFailureSummary(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, true)
	summary := "Compilation failed because the import was unused."
	task := &models.Task{
		ID:             uuid.New(),
		Title:          "build",
		FailureReason:  ptrString("exit status 1"),
		FailureSummary: &summary,
	}

	r.notifyTaskTransition(task, models.TaskStatusFailed)

	if len(fake.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(fake.broadcasts))
	}
	if got := fake.broadcasts[0].Body; got != summary {
		t.Errorf("body = %q, want %q", got, summary)
	}
}

func TestNotifyTaskTransition_FailedTruncatesLongReason(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, true)
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	task := taskWithReason("noisy", long)

	r.notifyTaskTransition(task, models.TaskStatusFailed)

	if len(fake.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(fake.broadcasts))
	}
	body := fake.broadcasts[0].Body
	wantLen := pushBodyMaxRunes + len([]rune("…"))
	if got := len([]rune(body)); got != wantLen {
		t.Errorf("body rune len = %d, want %d", got, wantLen)
	}
}

func TestNotifyTaskTransition_FailedFallsBackWhenReasonEmpty(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, true)
	task := &models.Task{ID: uuid.New(), Title: "no info"}

	r.notifyTaskTransition(task, models.TaskStatusFailed)

	if len(fake.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(fake.broadcasts))
	}
	if got := fake.broadcasts[0].Body; got != "Task execution failed" {
		t.Errorf("body = %q", got)
	}
}

func TestNotifyTaskTransition_NeedsReview(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, true)
	task := &models.Task{ID: uuid.New(), Title: "ship feature"}

	r.notifyTaskTransition(task, models.TaskStatusNeedsReview)

	if len(fake.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(fake.broadcasts))
	}
	got := fake.broadcasts[0]
	if got.Title != "Task needs review: ship feature" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Body != "Verification command failed" {
		t.Errorf("body = %q", got.Body)
	}
	if got.Tag != "task-"+task.ID.String() {
		t.Errorf("tag = %q", got.Tag)
	}
}

func TestNotifyTaskTransition_DoneNoPush(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, true)
	task := &models.Task{ID: uuid.New(), Title: "done task"}

	r.notifyTaskTransition(task, models.TaskStatusDone)

	if got := fake.totalCalls(); got != 0 {
		t.Errorf("expected no push calls for done, got %d", got)
	}
}

func TestNotifyTaskTransition_CancelledNoPush(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, true)
	task := &models.Task{ID: uuid.New(), Title: "cancelled task"}

	r.notifyTaskTransition(task, models.TaskStatusCancelled)

	if got := fake.totalCalls(); got != 0 {
		t.Errorf("expected no push calls for cancelled, got %d", got)
	}
}

func TestNotifyTaskTransition_RunningNoPush(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, true)
	task := &models.Task{ID: uuid.New(), Title: "running task"}

	r.notifyTaskTransition(task, models.TaskStatusRunning)

	if got := fake.totalCalls(); got != 0 {
		t.Errorf("expected no push calls for running, got %d", got)
	}
}

func TestNotifyTaskTransition_DisabledByConfig(t *testing.T) {
	t.Parallel()
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, false)
	task := taskWithReason("x", "y")

	r.notifyTaskTransition(task, models.TaskStatusFailed)

	if got := fake.totalCalls(); got != 0 {
		t.Errorf("expected no push calls when disabled, got %d", got)
	}
}

func TestNotifyTaskTransition_NilNotifierIsNoOp(t *testing.T) {
	t.Parallel()
	r := &Runner{config: &config.Config{PushNotificationsEnabled: true}}
	task := taskWithReason("x", "y")

	// Should not panic.
	r.notifyTaskTransition(task, models.TaskStatusFailed)
}

func TestNotifyTaskTransition_TagStableAcrossRetries(t *testing.T) {
	t.Parallel()
	// Two consecutive failures for the same task share the same tag so the
	// OS notification center collapses them rather than stacking duplicates.
	fake := &fakePushNotifier{}
	r := runnerWithFakePush(fake, true)
	id := uuid.New()
	task := &models.Task{ID: id, Title: "flaky", FailureReason: ptrString("attempt 1")}
	r.notifyTaskTransition(task, models.TaskStatusFailed)
	task.FailureReason = ptrString("attempt 2")
	r.notifyTaskTransition(task, models.TaskStatusFailed)

	if len(fake.broadcasts) != 2 {
		t.Fatalf("expected 2 broadcasts, got %d", len(fake.broadcasts))
	}
	if fake.broadcasts[0].Tag != fake.broadcasts[1].Tag {
		t.Errorf("expected stable tag across retries, got %q vs %q",
			fake.broadcasts[0].Tag, fake.broadcasts[1].Tag)
	}
}

func TestTruncateRunes_HandlesMultibyte(t *testing.T) {
	t.Parallel()
	in := "ščřž ščřž ščřž ščřž"
	got := truncateRunes(in, 4)
	want := "ščřž…"
	if got != want {
		t.Errorf("truncateRunes = %q, want %q", got, want)
	}
}

func TestTruncateRunes_NoTruncationWhenShort(t *testing.T) {
	t.Parallel()
	in := "short"
	if got := truncateRunes(in, 100); got != in {
		t.Errorf("truncateRunes = %q, want %q", got, in)
	}
}
