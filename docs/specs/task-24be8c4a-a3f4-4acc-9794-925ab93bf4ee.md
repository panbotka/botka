## Problem

Tool use events (e.g. creating a task, reading a file) should display live during streaming as compact informational boxes via `ToolCallPanel`. The infrastructure is implemented end-to-end but it's **not working** — tool calls are not visible during streaming.

**No database persistence needed** — just fix the live display during streaming.

## Current Architecture (already implemented)

- **Backend parsing** (`internal/claude/runner.go`): Parses `KindToolUse` and `KindToolResult` from NDJSON
- **Backend SSE** (`internal/handlers/chat.go`): Forwards `tool_use` and `tool_result` as separate SSE event types
- **Frontend streaming** (`ChatView.tsx`): Tracks `activeToolCalls` state, renders via `ToolCallPanel`
- **Frontend display** (`ToolCallPanel.tsx`): Renders tool name, input summary, status indicator

## Debugging steps

1. **Check backend**: Is `parseAssistantEvent()` in `runner.go` actually matching tool_use blocks? Add logging or check if the NDJSON events from Claude Code contain tool_use data in the expected format.

2. **Check SSE forwarding**: In `streamResponse()` in `chat.go`, are the `case claude.KindToolUse:` and `case claude.KindToolResult:` branches being reached? The SSE events use custom event types (`event: tool_use\n`) — verify the format.

3. **Check frontend SSE parsing**: In `parseSSE()` in `client.ts`, does it handle custom SSE event types like `event: tool_use`? Standard SSE parsers often only handle `event: message` or unnamed events. If `parseSSE()` doesn't recognize the `tool_use` event type, the data is silently dropped. **This is the most likely bug.**

4. **Check frontend state**: In `consumeStream()` in `ChatView.tsx`, are `chunk.tool_use` and `chunk.tool_result` ever truthy? Is `activeToolCalls` state being populated?

5. **Check rendering**: The rendering condition (~line 819) requires `isStreamingThisThread && activeToolCalls.length > 0`. Verify both conditions are met.

## Likely root cause

The SSE parser (`parseSSE()` in `client.ts`) probably doesn't handle custom event types. Standard SSE sends:
```
event: tool_use
data: {"id":"...","name":"Bash","input":{...}}
```

But if the parser only looks for `data:` lines and ignores `event:` lines, the tool_use data gets parsed but without the event type context, so `chunk.tool_use` is never set.

## Fix

1. Find the bug in the SSE parsing/forwarding chain
2. Fix it so tool_use and tool_result events reach the frontend state
3. Ensure `ToolCallPanel` renders correctly with the live data — compact boxes showing tool name and brief input

## Files to check

- `frontend/src/api/client.ts` — `parseSSE()` function, how it handles `event:` lines
- `frontend/src/components/ChatView.tsx` — `consumeStream()`, `activeToolCalls` state, rendering logic
- `internal/handlers/chat.go` — `streamResponse()`, tool_use/tool_result SSE emission
- `internal/claude/runner.go` — `parseAssistantEvent()`, event parsing

## Verification

1. Run `make check`
2. Send a message that triggers tool use (e.g. ask Claude to create a file or run a command)
3. Tool call boxes should appear live during streaming — small compact boxes with tool name and input summary

## CRITICAL: Do NOT deploy or restart

**NEVER run `make deploy`, `make install-service`, `systemctl restart`, or any command that would restart the Botka service.** You are running inside Botka — deploying would kill your own process and leave the task stuck in "running" state forever.