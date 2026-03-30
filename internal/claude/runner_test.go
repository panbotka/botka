package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseEvent_SystemInit(t *testing.T) {
	line := []byte(`{"type":"system","subtype":"init","session_id":"abc123"}`)
	evt := parseEvent(line)
	if evt.Kind != KindInit {
		t.Errorf("expected KindInit, got %d", evt.Kind)
	}
}

func TestParseEvent_SystemNonInit(t *testing.T) {
	line := []byte(`{"type":"system","subtype":"other"}`)
	evt := parseEvent(line)
	if evt.Kind != KindIgnored {
		t.Errorf("expected KindIgnored, got %d", evt.Kind)
	}
}

func TestParseEvent_ContentDelta(t *testing.T) {
	line := []byte(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}}`)
	evt := parseEvent(line)
	if evt.Kind != KindContentDelta {
		t.Errorf("expected KindContentDelta, got %d", evt.Kind)
	}
	if evt.Text != "hello" {
		t.Errorf("expected text 'hello', got %q", evt.Text)
	}
}

func TestParseEvent_ThinkingDelta(t *testing.T) {
	line := []byte(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"thinking_delta","text":"pondering"}}}`)
	evt := parseEvent(line)
	if evt.Kind != KindThinkingDelta {
		t.Errorf("expected KindThinkingDelta, got %d", evt.Kind)
	}
	if evt.Thinking != "pondering" {
		t.Errorf("expected thinking 'pondering', got %q", evt.Thinking)
	}
}

func TestParseEvent_ThinkingStart(t *testing.T) {
	line := []byte(`{"type":"stream_event","event":{"type":"content_block_start","content_block":{"type":"thinking","text":""}}}`)
	evt := parseEvent(line)
	if evt.Kind != KindThinkingDelta {
		t.Errorf("expected KindThinkingDelta for thinking start, got %d", evt.Kind)
	}
}

func TestParseEvent_ContentBlockStop(t *testing.T) {
	line := []byte(`{"type":"stream_event","event":{"type":"content_block_stop"}}`)
	evt := parseEvent(line)
	if evt.Kind != KindContentDone {
		t.Errorf("expected KindContentDone, got %d", evt.Kind)
	}
}

func TestParseEvent_ToolUse(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tool_1","name":"Read","input":{"path":"/tmp/x"}}]}}`)
	evt := parseEvent(line)
	if evt.Kind != KindToolUse {
		t.Errorf("expected KindToolUse, got %d", evt.Kind)
	}
	if evt.ToolName != "Read" {
		t.Errorf("expected tool name 'Read', got %q", evt.ToolName)
	}
	if evt.ToolID != "tool_1" {
		t.Errorf("expected tool ID 'tool_1', got %q", evt.ToolID)
	}
}

func TestParseEvent_Result(t *testing.T) {
	line := []byte(`{"type":"result","subtype":"success","result":"done","is_error":false,"duration_ms":5000,"num_turns":3,"total_cost_usd":0.05,"usage":{"input_tokens":100,"output_tokens":200}}`)
	evt := parseEvent(line)
	if evt.Kind != KindResult {
		t.Errorf("expected KindResult, got %d", evt.Kind)
	}
	if evt.ResultText != "done" {
		t.Errorf("expected result 'done', got %q", evt.ResultText)
	}
	if evt.CostUSD != 0.05 {
		t.Errorf("expected cost 0.05, got %f", evt.CostUSD)
	}
	if evt.DurationMs != 5000 {
		t.Errorf("expected duration 5000, got %d", evt.DurationMs)
	}
	if evt.NumTurns != 3 {
		t.Errorf("expected 3 turns, got %d", evt.NumTurns)
	}
	if evt.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", evt.InputTokens)
	}
	if evt.OutputTokens != 200 {
		t.Errorf("expected 200 output tokens, got %d", evt.OutputTokens)
	}
	if evt.IsError {
		t.Error("expected IsError=false")
	}
}

func TestParseEvent_ResultError(t *testing.T) {
	line := []byte(`{"type":"result","subtype":"error","result":"something failed","is_error":true,"duration_ms":100,"num_turns":1,"total_cost_usd":0.01,"usage":{"input_tokens":10,"output_tokens":20}}`)
	evt := parseEvent(line)
	if evt.Kind != KindResult {
		t.Errorf("expected KindResult, got %d", evt.Kind)
	}
	if !evt.IsError {
		t.Error("expected IsError=true")
	}
	if evt.ErrorMsg != "something failed" {
		t.Errorf("expected error msg 'something failed', got %q", evt.ErrorMsg)
	}
}

func TestParseEvent_Ignored(t *testing.T) {
	cases := []string{
		`{"type":"rate_limit_event"}`,
		`{"type":"user","message":"hi"}`,
		`{"type":"unknown_type"}`,
		`not json at all`,
	}
	for _, c := range cases {
		evt := parseEvent([]byte(c))
		if evt.Kind != KindIgnored {
			t.Errorf("expected KindIgnored for %q, got %d", c, evt.Kind)
		}
	}
}

func TestParseEvent_InputJSONDelta(t *testing.T) {
	line := []byte(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{}"}}}`)
	evt := parseEvent(line)
	if evt.Kind != KindIgnored {
		t.Errorf("expected KindIgnored for input_json_delta, got %d", evt.Kind)
	}
}

func TestParseEvent_MessageEvents(t *testing.T) {
	for _, mtype := range []string{"message_start", "message_delta", "message_stop"} {
		line := []byte(`{"type":"stream_event","event":{"type":"` + mtype + `"}}`)
		evt := parseEvent(line)
		if evt.Kind != KindIgnored {
			t.Errorf("expected KindIgnored for %s, got %d", mtype, evt.Kind)
		}
	}
}

func TestTitleFromContent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "short"},
		{"  spaces  ", "spaces"},
		{
			"This is a very long message that definitely exceeds sixty characters and needs truncation",
			"This is a very long message that definitely exceeds sixty...",
		},
		{
			"Thisisaverylongwordwithoutspacesthatshouldbetruncatedatexactlysixtycharactersmark",
			"Thisisaverylongwordwithoutspacesthatshouldbetruncatedatexact...",
		},
	}
	for _, tt := range tests {
		got := TitleFromContent(tt.input)
		if got != tt.want {
			t.Errorf("TitleFromContent(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRun_WithMockCommand(t *testing.T) {
	// Create a mock script that outputs NDJSON events
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-claude")
	events := []map[string]any{
		{"type": "system", "subtype": "init", "session_id": "test-123"},
		{"type": "stream_event", "event": map[string]any{
			"type":  "content_block_delta",
			"delta": map[string]any{"type": "text_delta", "text": "Hello world"},
		}},
		{"type": "result", "subtype": "success", "result": "Done", "is_error": false,
			"duration_ms": 1000, "num_turns": 1, "total_cost_usd": 0.01,
			"usage": map[string]any{"input_tokens": 50, "output_tokens": 100}},
	}

	var lines []string
	for _, e := range events {
		b, _ := json.Marshal(e)
		lines = append(lines, string(b))
	}

	scriptContent := "#!/bin/sh\n"
	for _, l := range lines {
		scriptContent += "echo '" + l + "'\n"
	}
	if err := os.WriteFile(script, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	// Verify mock script is executable
	if _, err := exec.LookPath(script); err != nil {
		// Use absolute path directly
		_ = err
	}

	cfg := RunConfig{
		ClaudePath: script,
		WorkDir:    dir,
		SessionID:  "test-session-id",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := Run(ctx, cfg, "test prompt")

	var collected []StreamEvent
	for evt := range ch {
		collected = append(collected, evt)
	}

	// We should get: init, content delta, result
	if len(collected) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(collected))
	}

	if collected[0].Kind != KindInit {
		t.Errorf("event 0: expected KindInit, got %d", collected[0].Kind)
	}
	if collected[1].Kind != KindContentDelta {
		t.Errorf("event 1: expected KindContentDelta, got %d", collected[1].Kind)
	}
	if collected[1].Text != "Hello world" {
		t.Errorf("event 1: expected text 'Hello world', got %q", collected[1].Text)
	}
	if collected[2].Kind != KindResult {
		t.Errorf("event 2: expected KindResult, got %d", collected[2].Kind)
	}
	if collected[2].ResultText != "Done" {
		t.Errorf("event 2: expected result 'Done', got %q", collected[2].ResultText)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	// Mock a script that traps signals and exits on them
	dir := t.TempDir()
	script := filepath.Join(dir, "mock-claude-slow")
	// Use a script that the shell can kill cleanly via exec (replaces shell process)
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 60\n"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := RunConfig{
		ClaudePath: script,
		WorkDir:    dir,
		SessionID:  "cancel-test-id",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	ch := Run(ctx, cfg, "test")

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	select {
	case <-done:
		// Channel closed, test passes
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for channel to close after context cancellation")
	}
}

func TestStderrBuffer_Add(t *testing.T) {
	b := &stderrBuffer{}
	b.Add("line1")
	b.Add("line2")
	got := b.String()
	if got != "line1\nline2" {
		t.Errorf("got %q, want %q", got, "line1\nline2")
	}
}

func TestStderrBuffer_Eviction(t *testing.T) {
	b := &stderrBuffer{}
	for i := 0; i < 25; i++ {
		b.Add(fmt.Sprintf("line%d", i))
	}
	lines := strings.Split(b.String(), "\n")
	if len(lines) != maxStderrLines {
		t.Errorf("got %d lines, want %d", len(lines), maxStderrLines)
	}
	// Should have evicted early lines
	if lines[0] != "line5" {
		t.Errorf("first line = %q, want %q", lines[0], "line5")
	}
}

func TestStderrBuffer_Empty(t *testing.T) {
	b := &stderrBuffer{}
	if got := b.String(); got != "" {
		t.Errorf("empty buffer should return empty string, got %q", got)
	}
}

func TestTitleFromContent_Truncation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int // max length check (0 = skip)
		wantEnd string
	}{
		{"short", "Hello", 5, "Hello"},
		{"exactly_60", strings.Repeat("a", 60), 60, strings.Repeat("a", 60)},
		{"long with space", "This is a very long message that should be truncated at a reasonable word boundary point somewhere", 0, "..."},
		{"long no good space", strings.Repeat("x", 100), 63, "..."},
		{"whitespace only", "   ", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TitleFromContent(tt.input)
			if tt.wantEnd != "" && !strings.HasSuffix(got, tt.wantEnd) {
				t.Errorf("TitleFromContent(%q) = %q, want suffix %q", tt.input, got, tt.wantEnd)
			}
			if tt.wantLen > 0 && len(got) > tt.wantLen {
				t.Errorf("TitleFromContent(%q) len=%d, want max %d", tt.input, len(got), tt.wantLen)
			}
		})
	}
}

func TestBuildStreamArgs_NamePassedOnResume(t *testing.T) {
	cfg := RunConfig{
		Resume:    true,
		SessionID: "sess-123",
		Name:      "My Thread",
	}
	args := buildStreamArgs(cfg)

	foundName := false
	for i, a := range args {
		if a == "--name" && i+1 < len(args) && args[i+1] == "My Thread" {
			foundName = true
			break
		}
	}
	if !foundName {
		t.Errorf("expected --name 'My Thread' in args on resume, got %v", args)
	}
}

func TestBuildStreamArgs_NamePassedWithoutResume(t *testing.T) {
	cfg := RunConfig{
		Name: "Fresh Session",
	}
	args := buildStreamArgs(cfg)

	foundName := false
	for i, a := range args {
		if a == "--name" && i+1 < len(args) && args[i+1] == "Fresh Session" {
			foundName = true
			break
		}
	}
	if !foundName {
		t.Errorf("expected --name 'Fresh Session' in args, got %v", args)
	}
}

func TestBuildStreamArgs_NoNameWhenEmpty(t *testing.T) {
	cfg := RunConfig{
		Resume:    true,
		SessionID: "sess-456",
	}
	args := buildStreamArgs(cfg)

	for _, a := range args {
		if a == "--name" {
			t.Errorf("expected no --name flag when Name is empty, got %v", args)
			break
		}
	}
}
