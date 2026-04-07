## Problem

When Claude Code calls `AskUserQuestion` during a chat session (e.g. from the product-manager skill), the question appears as a generic ToolCallPanel but there's no way for the user to respond. Claude Code waits on stdin for a `tool_result` NDJSON message that never arrives, causing the session to hang.

Observed in thread 131: assistant said "otázky se nezobrazily správně" because the AskUserQuestion tool call was shown as a non-interactive tool panel with no input mechanism.

## Root Cause

The chat pipeline is one-directional for tool calls. Tool_use events flow from Claude Code → Go parser → SSE → Frontend, but there's no reverse path for the user to send tool results back to Claude Code's stdin.

## Required Changes

### 1. Backend: Tool result endpoint

Add `POST /api/v1/threads/:id/tool-results` endpoint in `internal/handlers/chat.go`:

```go
// Request body:
{
  "tool_use_id": "toolu_...",
  "content": "user's answer text",
  "is_error": false
}
```

This endpoint must:
- Look up the active session for the thread via SessionManager
- Write a `tool_result` NDJSON message to the session's stdin
- Continue reading the session's stdout for further events (the response after the tool result)

### 2. Backend: SessionManager tool result writer

Add a method to `internal/claude/pool.go` to write tool results back to Claude Code:

```go
func (m *SessionManager) SendToolResult(threadID int64, toolUseID, content string, isError bool) error
```

The NDJSON format for tool results sent to Claude Code's stdin (stream-json input format) needs to be verified by checking Claude Code's documentation or testing. It likely looks like:

```json
{"type": "tool_result", "tool_use_id": "toolu_...", "content": "answer text", "is_error": false}
```

**Important:** After writing the tool result, Claude Code will continue its turn — more stdout events will follow. The existing event reading loop in `SendMessage()` needs to handle this. Consider whether `SendToolResult` should:
- Reuse the existing event channel from the original `SendMessage` call, OR
- Start a new event reading goroutine that feeds into the same SSE stream

The SSE stream from the original message send is likely still active (the frontend is still connected waiting for the turn to complete), so the simplest approach is to have the original reading goroutine still running and just write to stdin.

### 3. Backend: Parse tool_result events from Claude Code

In `internal/claude/runner.go`, the `parseEvent()` function defines `KindToolResult` but never creates it. Add parsing for tool result events that Claude Code emits after processing a tool result:

```go
case "tool_result":
    return StreamEvent{Kind: KindToolResult, ToolID: ..., Content: ...}, nil
```

### 4. Frontend: Interactive AskUserQuestion UI

In `frontend/src/components/ToolCallPanel.tsx` (or a new component):

- Detect when `tool_use.name === "AskUserQuestion"`
- Extract the question text from `tool_use.input.question`
- Render the question prominently (not as a collapsed tool panel)
- Show a text input field + submit button
- On submit, POST to `/api/v1/threads/:id/tool-results` with the answer
- After submission, disable the input and show the submitted answer
- Style it distinctly from regular messages — it should look like a question/prompt

### 5. Frontend: SSE state management

In `frontend/src/context/SSEContext.tsx`:

- Track which tool calls are "awaiting input" (AskUserQuestion with no result yet)
- When tool_result arrives via SSE, update the tool call state
- The SSE connection should remain open during the AskUserQuestion interaction (the turn isn't complete yet)

## Testing

1. Trigger AskUserQuestion by using the product-manager skill (ask to plan a new feature)
2. Verify the question renders with an input field
3. Submit an answer and verify Claude Code receives it and continues
4. Verify the answer appears in the conversation
5. Test multiple sequential AskUserQuestion calls in one turn
6. Test edge cases: disconnect during question, page refresh, session timeout

## Key Files

| File | What to change |
|------|---------------|
| `internal/handlers/chat.go` | New endpoint + wire into router |
| `internal/claude/pool.go` | `SendToolResult()` method |
| `internal/claude/runner.go` | Parse tool_result events |
| `internal/claude/events.go` | Verify KindToolResult struct fields |
| `frontend/src/components/ToolCallPanel.tsx` | Interactive input for AskUserQuestion |
| `frontend/src/context/SSEContext.tsx` | Track awaiting-input state |
| `frontend/src/api/client.ts` | API call for submitting tool results |

## Notes

- The `--input-format stream-json` stdin protocol is key — verify the exact NDJSON format Claude Code expects for tool results by testing manually or checking docs
- The existing SSE stream must stay open while waiting for user input — don't close it prematurely
- Consider generalizing this for other interactive tools in the future, not just AskUserQuestion
- Session pool keepalive (5 min timeout) must account for the time user spends answering questions