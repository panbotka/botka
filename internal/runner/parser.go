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
	// EventSystemInit is emitted on the initial system event and carries the model name.
	EventSystemInit
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
	CostUSD             float64
	DurationMs          int64
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	IsError             bool

	// SystemInit
	Model string

	// SystemError
	Message string
}

// streamUsage is the nested usage object on the result event.
type streamUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// streamLine is the top-level JSON structure for each line of output.
type streamLine struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype"`
	Message json.RawMessage `json:"message"`
	Model   string          `json:"model"`

	// Result fields (top-level)
	CostUSD       float64     `json:"cost_usd"`
	TotalCostUSD  float64     `json:"total_cost_usd"`
	DurationMs    int64       `json:"duration_ms"`
	DurationAPIMs int64       `json:"duration_api_ms"`
	InputTokens   int64       `json:"input_tokens"`
	OutputTokens  int64       `json:"output_tokens"`
	Usage         streamUsage `json:"usage"`
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
//
// Lines are read with bufio.Reader.ReadBytes, which accepts lines of any size.
// A previous bufio.Scanner implementation capped a line at 1MB and aborted the
// whole stream on anything larger — which newer Claude Code hits routinely when
// a tool returns a base64 image/screenshot as a single tool_result line. When
// that happened the trailing "result" event was never parsed and the task was
// misclassified as "claude process crashed". This mirrors the chat runner
// (internal/claude/runner.go), which reads stdout the same way for the same reason.
func ParseStream(reader io.Reader, onEvent func(Event)) error {
	br := bufio.NewReader(reader)
	for {
		line, err := br.ReadBytes('\n')
		if n := len(line); n > 0 && line[n-1] == '\n' {
			line = line[:n-1]
		}
		if len(line) > 0 {
			parseLine(line, onEvent)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// parseLine parses a single stream-json line and emits the extracted events via
// onEvent. Lines that are not valid JSON are emitted as raw assistant text.
func parseLine(line []byte, onEvent func(Event)) {
	var sl streamLine
	if err := json.Unmarshal(line, &sl); err != nil {
		// Not valid JSON — treat as raw text output
		slog.Debug("stream parser: non-JSON line")
		onEvent(Event{Type: EventAssistantText, Text: string(line)})
		return
	}

	switch sl.Type {
	case "assistant":
		parseAssistantMessage(sl.Message, onEvent)
	case "result":
		cost := sl.CostUSD
		if cost == 0 {
			cost = sl.TotalCostUSD
		}
		inputTokens := sl.InputTokens
		if inputTokens == 0 {
			inputTokens = sl.Usage.InputTokens
		}
		outputTokens := sl.OutputTokens
		if outputTokens == 0 {
			outputTokens = sl.Usage.OutputTokens
		}
		onEvent(Event{
			Type:                EventResult,
			CostUSD:             cost,
			DurationMs:          sl.DurationMs,
			InputTokens:         inputTokens,
			OutputTokens:        outputTokens,
			CacheReadTokens:     sl.Usage.CacheReadInputTokens,
			CacheCreationTokens: sl.Usage.CacheCreationInputTokens,
			IsError:             sl.Subtype != "success",
		})
	case "system":
		if sl.Subtype == "init" && sl.Model != "" {
			onEvent(Event{Type: EventSystemInit, Model: sl.Model})
		}
		parseSystemMessage(sl.Message, onEvent)
	default:
		// Unknown type — skip silently
		slog.Debug("stream parser: unknown line type", "type", sl.Type)
	}
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
