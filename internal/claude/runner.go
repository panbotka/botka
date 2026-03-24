package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// StreamEvent represents a parsed event from Claude Code's stream-json output.
type StreamEvent struct {
	// High-level event type for consumers
	Kind EventKind

	// Text content chunk (for KindContentDelta)
	Text string

	// Thinking content chunk (for KindThinkingDelta)
	Thinking string

	// Tool use info (for KindToolUse)
	ToolName  string
	ToolInput json.RawMessage
	ToolID    string

	// Tool result (for KindToolResult)
	ToolUseID   string
	ToolContent string
	ToolIsError bool

	// Result info (for KindResult)
	ResultText   string
	CostUSD      float64
	DurationMs   int
	NumTurns     int
	InputTokens  int
	OutputTokens int
	IsError      bool
	ErrorMsg     string

	// Raw JSON for pass-through
	Raw json.RawMessage
}

// EventKind classifies stream events from the Claude Code subprocess.
type EventKind int

const (
	KindInit          EventKind = iota
	KindContentDelta            // text_delta streaming chunk
	KindContentDone             // content block finished
	KindThinkingDelta           // thinking text chunk
	KindThinkingDone            // thinking block finished
	KindToolUse                 // tool call started
	KindToolResult              // tool result received
	KindResult                  // turn complete (success or error)
	KindError                   // process error
	KindIgnored                 // hook events, rate limits, etc.
)

// RunConfig holds configuration for a Claude Code subprocess invocation.
type RunConfig struct {
	ClaudePath       string // path to claude binary
	WorkDir          string // working directory for the process
	SessionID        string // UUID for session tracking
	Resume           bool   // whether to resume an existing session
	Model            string // AI model to use
	SystemPromptFile string // path to assembled context file
	Name             string // display name for the session
}

// Run spawns a Claude Code subprocess with the given prompt and streams events.
// The returned channel receives parsed events until the process completes.
func Run(ctx context.Context, cfg RunConfig, prompt string) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)

		args := []string{
			"-p",
			"--verbose",
			"--output-format", "stream-json",
			"--include-partial-messages",
			"--dangerously-skip-permissions",
		}

		if cfg.Resume && cfg.SessionID != "" {
			args = append(args, "--resume", cfg.SessionID)
		} else if cfg.SessionID != "" {
			args = append(args, "--session-id", cfg.SessionID)
		}

		// Only pass --model for Claude Code model names (sonnet, opus, haiku)
		// Skip OpenClaw-format models (e.g. "anthropic/claude-sonnet-4-20250514")
		if cfg.Model != "" && !strings.Contains(cfg.Model, "/") {
			args = append(args, "--model", cfg.Model)
		}

		if cfg.SystemPromptFile != "" && !cfg.Resume {
			args = append(args, "--append-system-prompt-file", cfg.SystemPromptFile)
		}

		if cfg.Name != "" && !cfg.Resume {
			args = append(args, "--name", cfg.Name)
		}

		// The prompt is the last argument
		args = append(args, prompt)

		cmd := exec.CommandContext(ctx, cfg.ClaudePath, args...)
		if cfg.WorkDir != "" {
			cmd.Dir = cfg.WorkDir
		}

		sessionPrefix := cfg.SessionID
		if len(sessionPrefix) > 8 {
			sessionPrefix = sessionPrefix[:8]
		}

		log.Printf("[claude] spawning: %s %v (dir=%s)", cfg.ClaudePath, args, cmd.Dir)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("stdout pipe: %v", err)}
			return
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("stderr pipe: %v", err)}
			return
		}

		if err := cmd.Start(); err != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("start claude: %v", err)}
			return
		}

		// Drain stderr in background to prevent pipe deadlock
		go func() {
			scanner := bufio.NewScanner(stderr)
			scanner.Buffer(make([]byte, 0), 1<<20)
			for scanner.Scan() {
				log.Printf("[claude:%s] stderr: %s", sessionPrefix, scanner.Text())
			}
		}()

		// Read stdout NDJSON events
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0), 1<<20) // 1MB buffer for large events

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			event := parseEvent(line)
			if event.Kind == KindIgnored {
				continue
			}
			ch <- event
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("read stdout: %v", err)}
		}

		if err := cmd.Wait(); err != nil {
			// Only report if context wasn't cancelled
			if ctx.Err() == nil {
				ch <- StreamEvent{Kind: KindError, ErrorMsg: fmt.Sprintf("claude exited: %v", err)}
			}
		}
	}()

	return ch
}

// parseEvent converts a raw JSON line from stream-json into a StreamEvent.
func parseEvent(line []byte) StreamEvent {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return StreamEvent{Kind: KindIgnored}
	}

	var eventType string
	if t, ok := raw["type"]; ok {
		_ = json.Unmarshal(t, &eventType)
	}

	switch eventType {
	case "system":
		var subtype string
		if st, ok := raw["subtype"]; ok {
			_ = json.Unmarshal(st, &subtype)
		}
		if subtype == "init" {
			return StreamEvent{Kind: KindInit, Raw: line}
		}
		return StreamEvent{Kind: KindIgnored}

	case "stream_event":
		return parseStreamEvent(raw)

	case "assistant":
		return parseAssistantEvent(raw)

	case "result":
		return parseResultEvent(raw, line)

	case "rate_limit_event", "user":
		return StreamEvent{Kind: KindIgnored}

	default:
		return StreamEvent{Kind: KindIgnored}
	}
}

// parseStreamEvent handles the wrapped Anthropic API streaming events.
func parseStreamEvent(raw map[string]json.RawMessage) StreamEvent {
	eventData, ok := raw["event"]
	if !ok {
		return StreamEvent{Kind: KindIgnored}
	}

	var event struct {
		Type         string `json:"type"`
		Index        int    `json:"index"`
		ContentBlock *struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content_block"`
		Delta *struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
	}

	if err := json.Unmarshal(eventData, &event); err != nil {
		return StreamEvent{Kind: KindIgnored}
	}

	switch event.Type {
	case "content_block_start":
		if event.ContentBlock != nil && event.ContentBlock.Type == "thinking" {
			return StreamEvent{Kind: KindThinkingDelta, Thinking: ""}
		}
		return StreamEvent{Kind: KindIgnored}

	case "content_block_delta":
		if event.Delta == nil {
			return StreamEvent{Kind: KindIgnored}
		}
		switch event.Delta.Type {
		case "text_delta":
			return StreamEvent{Kind: KindContentDelta, Text: event.Delta.Text}
		case "thinking_delta":
			return StreamEvent{Kind: KindThinkingDelta, Thinking: event.Delta.Text}
		case "input_json_delta":
			// Tool input streaming - ignore (we get the full tool_use from assistant event)
			return StreamEvent{Kind: KindIgnored}
		}
		return StreamEvent{Kind: KindIgnored}

	case "content_block_stop":
		return StreamEvent{Kind: KindContentDone}

	case "message_start", "message_delta", "message_stop":
		return StreamEvent{Kind: KindIgnored}

	default:
		return StreamEvent{Kind: KindIgnored}
	}
}

// parseAssistantEvent extracts tool_use blocks from complete assistant messages.
func parseAssistantEvent(raw map[string]json.RawMessage) StreamEvent {
	msgData, ok := raw["message"]
	if !ok {
		return StreamEvent{Kind: KindIgnored}
	}

	var msg struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}

	if err := json.Unmarshal(msgData, &msg); err != nil {
		return StreamEvent{Kind: KindIgnored}
	}

	// Look for tool_use blocks in the assistant message
	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			inputBytes, _ := json.Marshal(block.Input)
			return StreamEvent{
				Kind:      KindToolUse,
				ToolName:  block.Name,
				ToolInput: inputBytes,
				ToolID:    block.ID,
				Raw:       msgData,
			}
		}
	}

	return StreamEvent{Kind: KindIgnored}
}

// parseResultEvent extracts the final result with cost and duration.
func parseResultEvent(_ map[string]json.RawMessage, fullLine []byte) StreamEvent {
	var result struct {
		Subtype      string  `json:"subtype"`
		IsError      bool    `json:"is_error"`
		Result       string  `json:"result"`
		DurationMs   int     `json:"duration_ms"`
		NumTurns     int     `json:"num_turns"`
		TotalCostUSD float64 `json:"total_cost_usd"`
		Usage        struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(fullLine, &result); err != nil {
		return StreamEvent{Kind: KindIgnored}
	}

	evt := StreamEvent{
		Kind:         KindResult,
		ResultText:   result.Result,
		CostUSD:      result.TotalCostUSD,
		DurationMs:   result.DurationMs,
		NumTurns:     result.NumTurns,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
		Raw:          fullLine,
	}

	if result.IsError || result.Subtype == "error" {
		evt.IsError = true
		evt.ErrorMsg = result.Result
	}

	return evt
}

// Compact sends a /compact command to a Claude Code session.
func Compact(ctx context.Context, cfg RunConfig) <-chan StreamEvent {
	return Run(ctx, cfg, "/compact")
}

// TitleFromContent returns a truncated version of content suitable as a thread title.
func TitleFromContent(content string) string {
	content = strings.TrimSpace(content)
	if len(content) > 60 {
		// Find the last space before 60 chars
		if idx := strings.LastIndex(content[:60], " "); idx > 20 {
			content = content[:idx] + "..."
		} else {
			content = content[:60] + "..."
		}
	}
	return content
}
