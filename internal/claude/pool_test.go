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

func TestSession_LargeNDJSONLine(t *testing.T) {
	// Regression guard: a single NDJSON event larger than 1MB (e.g. a big
	// tool_result payload) must be read without error. The old
	// bufio.Scanner buffer rejected such lines with "token too long".
	dir := t.TempDir()

	// Build the >1MB tool_result event and the final result event.
	largeText := strings.Repeat("y", 2<<20) // 2 MiB
	largeEvent := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tool_big","content":"` + largeText + `"}]}}`
	resultEvent := `{"type":"result","subtype":"success","result":"ok","is_error":false,"duration_ms":1,"num_turns":1,"total_cost_usd":0.0,"usage":{"input_tokens":1,"output_tokens":1}}`

	eventsFile := filepath.Join(dir, "events.ndjson")
	if err := os.WriteFile(eventsFile, []byte(largeEvent+"\n"+resultEvent+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Emit init, consume one input line, print the prepared events, then
	// stay alive — a real Claude session remains running after delivering
	// a result so the session can accept the next message.
	body := `printf '{"type":"system","subtype":"init","session_id":"large-test"}\n'
read line
cat ` + eventsFile + `
sleep 30
`
	script := writeFakeClaude(t, dir, body)

	mgr := NewSessionManager(5 * time.Minute)
	defer mgr.Shutdown()

	cfg := RunConfig{
		ClaudePath: script,
		WorkDir:    dir,
		SessionID:  "large-test",
	}
	s, _ := mgr.GetOrCreate(cfg, 100, "large-test-thread", "")
	if s == nil {
		t.Fatal("failed to start session")
	}

	events := drain(t, mgr.SendMessage(s, "hello"), 10*time.Second)
	if len(events) == 0 {
		t.Fatal("no events received")
	}

	var gotToolResult, gotResult bool
	for _, evt := range events {
		if evt.Kind == KindError {
			t.Fatalf("unexpected error event: %q", evt.ErrorMsg)
		}
		if evt.Kind == KindToolResult {
			gotToolResult = true
			if len(evt.ToolContent) != len(largeText) {
				t.Errorf("tool content size mismatch: got %d, want %d", len(evt.ToolContent), len(largeText))
			}
		}
		if evt.Kind == KindResult {
			gotResult = true
		}
	}
	if !gotToolResult {
		t.Error("did not receive tool_result event parsed from >1MB line")
	}
	if !gotResult {
		t.Error("did not receive result event")
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
