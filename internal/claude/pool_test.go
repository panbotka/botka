package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeClaude writes an executable shell script at <dir>/fake-claude
// with the given body and returns its absolute path. The script runs under
// /bin/sh so it can rely on POSIX builtins like read, printf, and kill.
func writeFakeClaude(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-claude")
	content := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	return path
}

// drain collects events from the stream until the channel closes or the
// deadline elapses.
func drain(t *testing.T, ch <-chan StreamEvent, timeout time.Duration) []StreamEvent {
	t.Helper()
	var events []StreamEvent
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-deadline:
			t.Fatalf("timed out waiting for stream to close after %v", timeout)
			return events
		}
	}
}

func TestSession_ProcessCrash_ReportsExitCode(t *testing.T) {
	dir := t.TempDir()
	// Emit init, read one line from stdin (the user message), then exit 42.
	script := writeFakeClaude(t, dir, `
printf '{"type":"system","subtype":"init","session_id":"crash-test"}\n'
read line
exit 42
`)

	mgr := NewSessionManager(5 * time.Minute)
	defer mgr.Shutdown()

	cfg := RunConfig{
		ClaudePath: script,
		WorkDir:    dir,
		SessionID:  "crash-test",
	}
	s, _ := mgr.GetOrCreate(cfg, 1, "crash-test-thread", "")
	if s == nil {
		t.Fatal("failed to start session")
	}

	events := drain(t, mgr.SendMessage(s, "hello"), 5*time.Second)
	if len(events) == 0 {
		t.Fatal("no events received")
	}

	last := events[len(events)-1]
	if last.Kind != KindError {
		t.Fatalf("expected last event KindError, got %d (msg=%q)", last.Kind, last.ErrorMsg)
	}
	if !strings.Contains(last.ErrorMsg, "exit code 42") {
		t.Errorf("error message missing exit code: %q", last.ErrorMsg)
	}
	if !strings.Contains(last.ErrorMsg, "exited unexpectedly") {
		t.Errorf("error message missing crash phrase: %q", last.ErrorMsg)
	}
}

func TestSession_ProcessKilled_HintsOOM(t *testing.T) {
	dir := t.TempDir()
	// Emit init, read one line, self-kill with SIGKILL.
	script := writeFakeClaude(t, dir, `
printf '{"type":"system","subtype":"init","session_id":"kill-test"}\n'
read line
kill -KILL $$
`)

	mgr := NewSessionManager(5 * time.Minute)
	defer mgr.Shutdown()

	cfg := RunConfig{
		ClaudePath: script,
		WorkDir:    dir,
		SessionID:  "kill-test",
	}
	s, _ := mgr.GetOrCreate(cfg, 2, "kill-test-thread", "")
	if s == nil {
		t.Fatal("failed to start session")
	}

	events := drain(t, mgr.SendMessage(s, "hello"), 5*time.Second)
	if len(events) == 0 {
		t.Fatal("no events received")
	}

	last := events[len(events)-1]
	if last.Kind != KindError {
		t.Fatalf("expected last event KindError, got %d (msg=%q)", last.Kind, last.ErrorMsg)
	}
	low := strings.ToLower(last.ErrorMsg)
	if !strings.Contains(low, "killed") && !strings.Contains(low, "signal") {
		t.Errorf("error message should mention signal/killed: %q", last.ErrorMsg)
	}
	if !strings.Contains(low, "oom") {
		t.Errorf("SIGKILL error should hint at possible OOM: %q", last.ErrorMsg)
	}
}

func TestSession_ScannerUnblockedOnExit(t *testing.T) {
	// Regression guard: when the process exits without printing a result
	// event, the SendMessage scanner must not hang waiting for more output.
	// The monitor goroutine closes stdout so bufio.Scanner sees EOF.
	dir := t.TempDir()
	script := writeFakeClaude(t, dir, `
printf '{"type":"system","subtype":"init","session_id":"unblock-test"}\n'
read line
# Exit silently without printing a result event. If stdout is not closed,
# the SendMessage scanner will hang indefinitely.
exit 0
`)

	mgr := NewSessionManager(5 * time.Minute)
	defer mgr.Shutdown()

	cfg := RunConfig{
		ClaudePath: script,
		WorkDir:    dir,
		SessionID:  "unblock-test",
	}
	s, _ := mgr.GetOrCreate(cfg, 3, "unblock-test-thread", "")
	if s == nil {
		t.Fatal("failed to start session")
	}

	done := make(chan struct{})
	go func() {
		for range mgr.SendMessage(s, "hello") {
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("SendMessage hung after process exit — scanner was not unblocked")
	}
}
