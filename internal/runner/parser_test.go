package runner

import (
	"strings"
	"testing"
)

func collectEvents(t *testing.T, input string) []Event {
	t.Helper()
	var events []Event
	err := ParseStream(strings.NewReader(input), func(e Event) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("ParseStream returned error: %v", err)
	}
	return events
}

func TestParseStream_AssistantTextBlock(t *testing.T) {
	input := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello world"}]}}`
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventAssistantText {
		t.Errorf("expected EventAssistantText, got %d", events[0].Type)
	}
	if events[0].Text != "Hello world" {
		t.Errorf("expected text %q, got %q", "Hello world", events[0].Text)
	}
}

func TestParseStream_AssistantMultipleBlocks(t *testing.T) {
	input := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Let me help."},{"type":"tool_use","name":"Read","input":{"path":"/tmp/file.txt"}}]}}`
	events := collectEvents(t, input)

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != EventAssistantText {
		t.Errorf("event 0: expected EventAssistantText, got %d", events[0].Type)
	}
	if events[0].Text != "Let me help." {
		t.Errorf("event 0: expected text %q, got %q", "Let me help.", events[0].Text)
	}
	if events[1].Type != EventToolUse {
		t.Errorf("event 1: expected EventToolUse, got %d", events[1].Type)
	}
	if events[1].ToolName != "Read" {
		t.Errorf("event 1: expected tool name %q, got %q", "Read", events[1].ToolName)
	}
	if events[1].Input != `{"path":"/tmp/file.txt"}` {
		t.Errorf("event 1: expected input %q, got %q", `{"path":"/tmp/file.txt"}`, events[1].Input)
	}
}

func TestParseStream_ToolUseWithNameAndInput(t *testing.T) {
	input := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls -la"}}]}}`
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventToolUse {
		t.Errorf("expected EventToolUse, got %d", e.Type)
	}
	if e.ToolName != "Bash" {
		t.Errorf("expected tool name %q, got %q", "Bash", e.ToolName)
	}
	if e.Input != `{"command":"ls -la"}` {
		t.Errorf("expected input %q, got %q", `{"command":"ls -la"}`, e.Input)
	}
}

func TestParseStream_ToolUseNoInput(t *testing.T) {
	input := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"ListFiles"}]}}`
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventToolUse {
		t.Errorf("expected EventToolUse, got %d", e.Type)
	}
	if e.ToolName != "ListFiles" {
		t.Errorf("expected tool name %q, got %q", "ListFiles", e.ToolName)
	}
	if e.Input != "{}" {
		t.Errorf("expected default input %q, got %q", "{}", e.Input)
	}
}

func TestParseStream_ResultSuccess(t *testing.T) {
	input := `{"type":"result","subtype":"success","cost_usd":0.05,"duration_ms":1200,"duration_api_ms":1000}`
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventResult {
		t.Errorf("expected EventResult, got %d", e.Type)
	}
	if e.IsError {
		t.Error("expected IsError=false for success result")
	}
}

func TestParseStream_ResultError(t *testing.T) {
	input := `{"type":"result","subtype":"error","cost_usd":0.01,"duration_ms":500}`
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventResult {
		t.Errorf("expected EventResult, got %d", e.Type)
	}
	if !e.IsError {
		t.Error("expected IsError=true for error result")
	}
}

func TestParseStream_ResultCostAndDuration(t *testing.T) {
	input := `{"type":"result","subtype":"success","cost_usd":0.1234,"duration_ms":5678}`
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.CostUSD != 0.1234 {
		t.Errorf("expected cost 0.1234, got %f", e.CostUSD)
	}
	if e.DurationMs != 5678 {
		t.Errorf("expected duration 5678, got %d", e.DurationMs)
	}
}

func TestParseStream_SystemErrorWithMessage(t *testing.T) {
	input := `{"type":"system","message":{"message":"rate limit exceeded"}}`
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventSystemError {
		t.Errorf("expected EventSystemError, got %d", e.Type)
	}
	if e.Message != "rate limit exceeded" {
		t.Errorf("expected message %q, got %q", "rate limit exceeded", e.Message)
	}
}

func TestParseStream_SystemErrorRawFallback(t *testing.T) {
	// message field is not a JSON object (it's a string), so Unmarshal into struct fails
	input := `{"type":"system","message":"some raw error string"}`
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventSystemError {
		t.Errorf("expected EventSystemError, got %d", e.Type)
	}
	// The raw JSON value is a quoted string: "some raw error string"
	if e.Message != `"some raw error string"` {
		t.Errorf("expected raw message %q, got %q", `"some raw error string"`, e.Message)
	}
}

func TestParseStream_EmptyLinesSkipped(t *testing.T) {
	input := "\n\n" + `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}` + "\n\n"
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event (empty lines skipped), got %d", len(events))
	}
	if events[0].Text != "hi" {
		t.Errorf("expected text %q, got %q", "hi", events[0].Text)
	}
}

func TestParseStream_NonJSONLineEmittedAsText(t *testing.T) {
	input := "this is not json"
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventAssistantText {
		t.Errorf("expected EventAssistantText, got %d", e.Type)
	}
	if e.Text != "this is not json" {
		t.Errorf("expected text %q, got %q", "this is not json", e.Text)
	}
}

func TestParseStream_MalformedJSONTreatedAsText(t *testing.T) {
	input := `{"type": "assistant", broken json here`
	events := collectEvents(t, input)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventAssistantText {
		t.Errorf("expected EventAssistantText for malformed JSON, got %d", e.Type)
	}
	if e.Text != input {
		t.Errorf("expected text to be the raw line, got %q", e.Text)
	}
}

func TestParseStream_UnknownTypeSilentlySkipped(t *testing.T) {
	input := `{"type":"unknown_type","message":"something"}`
	events := collectEvents(t, input)

	if len(events) != 0 {
		t.Fatalf("expected 0 events for unknown type, got %d", len(events))
	}
}

func TestParseStream_EmptyAssistantMessage(t *testing.T) {
	// Assistant line with no message field — raw is empty
	input := `{"type":"assistant"}`
	events := collectEvents(t, input)

	if len(events) != 0 {
		t.Fatalf("expected 0 events for empty assistant message, got %d", len(events))
	}
}

func TestParseStream_EmptySystemMessage(t *testing.T) {
	input := `{"type":"system"}`
	events := collectEvents(t, input)

	if len(events) != 0 {
		t.Fatalf("expected 0 events for empty system message, got %d", len(events))
	}
}

func TestParseStream_FullMultiLineSequence(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I will read the file."}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"path":"/tmp/x"}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Here is the content."}]}}`,
		`{"type":"result","subtype":"success","cost_usd":0.03,"duration_ms":2500}`,
	}
	input := strings.Join(lines, "\n")
	events := collectEvents(t, input)

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// Event 0: text
	if events[0].Type != EventAssistantText || events[0].Text != "I will read the file." {
		t.Errorf("event 0: expected assistant text, got type=%d text=%q", events[0].Type, events[0].Text)
	}

	// Event 1: tool use
	if events[1].Type != EventToolUse || events[1].ToolName != "Read" {
		t.Errorf("event 1: expected tool use Read, got type=%d name=%q", events[1].Type, events[1].ToolName)
	}

	// Event 2: text
	if events[2].Type != EventAssistantText || events[2].Text != "Here is the content." {
		t.Errorf("event 2: expected assistant text, got type=%d text=%q", events[2].Type, events[2].Text)
	}

	// Event 3: result
	if events[3].Type != EventResult || events[3].IsError || events[3].CostUSD != 0.03 || events[3].DurationMs != 2500 {
		t.Errorf("event 3: expected success result with cost=0.03 duration=2500, got err=%v cost=%f dur=%d",
			events[3].IsError, events[3].CostUSD, events[3].DurationMs)
	}
}

func TestParseStream_EmptyInput(t *testing.T) {
	events := collectEvents(t, "")

	if len(events) != 0 {
		t.Fatalf("expected 0 events for empty input, got %d", len(events))
	}
}

func TestParseStream_TextBlockWithEmptyTextSkipped(t *testing.T) {
	input := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":""}]}}`
	events := collectEvents(t, input)

	if len(events) != 0 {
		t.Fatalf("expected 0 events for empty text block, got %d", len(events))
	}
}
