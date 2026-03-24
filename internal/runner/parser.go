package runner

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
)

// EventType identifies the kind of parsed stream event.
type EventType int

const (
	// EventAssistantText is emitted for each text block in an assistant message.
	EventAssistantText EventType = iota
	// EventToolUse is emitted for each tool_use block in an assistant message.
	EventToolUse
	// EventResult is emitted for the final result message.
	EventResult
	// EventSystemError is emitted for system-level errors.
	EventSystemError
)

// Event represents a parsed event from Claude's stream-json output.
type Event struct {
	Type EventType

	// AssistantText
	Text string

	// ToolUse
	ToolName string
	Input    string // raw JSON

	// Result
	CostUSD    float64
	DurationMs int64
	IsError    bool

	// SystemError
	Message string
}

// streamLine is the top-level JSON structure for each line of output.
type streamLine struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype"`
	Message json.RawMessage `json:"message"`

	// Result fields (top-level)
	CostUSD       float64 `json:"cost_usd"`
	DurationMs    int64   `json:"duration_ms"`
	DurationAPIMs int64   `json:"duration_api_ms"`
}

// streamMessage represents the nested message object in assistant lines.
type streamMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// contentBlock represents a single content block (text or tool_use).
type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ParseStream reads line-by-line from reader, parses each JSON line from
// Claude's stream-json output format, and calls onEvent for each extracted event.
// Non-JSON lines are emitted as AssistantText. Empty lines are skipped.
func ParseStream(reader io.Reader, onEvent func(Event)) error {
	scanner := bufio.NewScanner(reader)
	// Allow up to 1MB per line for large tool inputs
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var sl streamLine
		if err := json.Unmarshal(line, &sl); err != nil {
			// Not valid JSON — treat as raw text output
			slog.Debug("stream parser: non-JSON line")
			onEvent(Event{Type: EventAssistantText, Text: string(line)})
			continue
		}

		switch sl.Type {
		case "assistant":
			parseAssistantMessage(sl.Message, onEvent)
		case "result":
			onEvent(Event{
				Type:       EventResult,
				CostUSD:    sl.CostUSD,
				DurationMs: sl.DurationMs,
				IsError:    sl.Subtype != "success",
			})
		case "system":
			parseSystemMessage(sl.Message, onEvent)
		default:
			// Unknown type — skip silently
			slog.Debug("stream parser: unknown line type", "type", sl.Type)
		}
	}

	return scanner.Err()
}

// parseAssistantMessage extracts text and tool_use blocks from the message content.
func parseAssistantMessage(raw json.RawMessage, onEvent func(Event)) {
	if len(raw) == 0 {
		return
	}

	var msg streamMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		slog.Warn("stream parser: cannot parse assistant message", "error", err)
		return
	}

	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		slog.Warn("stream parser: cannot parse content blocks", "error", err)
		return
	}

	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				onEvent(Event{Type: EventAssistantText, Text: block.Text})
			}
		case "tool_use":
			inputStr := "{}"
			if len(block.Input) > 0 {
				inputStr = string(block.Input)
			}
			onEvent(Event{
				Type:     EventToolUse,
				ToolName: block.Name,
				Input:    inputStr,
			})
		}
	}
}

// parseSystemMessage extracts the error message from a system event.
func parseSystemMessage(raw json.RawMessage, onEvent func(Event)) {
	if len(raw) == 0 {
		return
	}

	// System messages may have various formats; try to extract a message string
	var msg struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		// Fall back to using the raw JSON as the message
		onEvent(Event{Type: EventSystemError, Message: string(raw)})
		return
	}
	if msg.Message != "" {
		onEvent(Event{Type: EventSystemError, Message: msg.Message})
	}
}
