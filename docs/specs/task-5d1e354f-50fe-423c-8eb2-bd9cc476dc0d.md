## Problem

When the user sends a message in chat and then navigates to another page (e.g. Settings), the SSE connection for the streaming response gets disconnected because the ChatView component unmounts. When they navigate back, the response is lost — they see no reply at all.

## Root Cause

The SSE event stream for chat responses is managed inside `ChatView.tsx` (or a hook it uses). When React Router navigates away from `/chat`, the component unmounts, the SSE `EventSource` or `fetch` stream is aborted, and any in-flight response is lost. The backend Claude process may still be running and producing output, but no browser tab is listening.

## Requirements

1. **Don't lose responses when navigating away** — if the user sends a message and leaves the chat page, the response should still be captured and visible when they return.
2. Streaming can stop visually when navigating away — the user doesn't need to see live streaming on non-chat pages. But the completed response must be there when they come back.

## Implementation options (pick the simplest)

### Option A: Refetch messages on mount (simplest)

When `ChatView` mounts (or when the user navigates back to the chat thread), always fetch the latest messages from the API. If the Claude process finished while the user was away, the completed response will already be in the database and will appear.

This likely already happens on mount, but the issue may be that:
- The assistant response isn't saved to DB until streaming completes
- Or the component shows stale cached messages instead of refetching

Check the flow:
1. Does the backend save the assistant message to the database during/after streaming?
2. Does ChatView fetch messages from API on mount, or does it rely on local state?

If messages are saved to DB after streaming completes, then simply ensuring ChatView refetches messages from the API when it mounts (and not relying on stale React state) should fix it.

### Option B: Keep SSE connection alive outside ChatView

Move the SSE streaming logic out of ChatView into a higher-level component or context that doesn't unmount on navigation. For example:
- A `ChatStreamContext` provider at the App level
- Store incoming stream events in a ref or state that persists across navigations
- ChatView reads from this shared state when it mounts

This is more complex but gives a better UX.

### Recommendation

Start with Option A — it's simpler and covers the main case. The user navigates away, comes back, and sees the completed response. If the response is still streaming when they return, reconnect to the SSE stream for that thread.

## Files to check

- `frontend/src/components/ChatView.tsx` — SSE connection lifecycle, message fetching on mount
- `frontend/src/hooks/` — any streaming or message hooks
- `frontend/src/api/client.ts` — how messages are fetched and how SSE is established
- `internal/handlers/chat.go` — when/how assistant messages are persisted to DB

## Verification

1. Run `make check`
2. Send a message in chat, navigate to Settings, wait 10s, navigate back — response should be visible

## CRITICAL: Do NOT deploy or restart

**NEVER run `make deploy`, `make install-service`, `systemctl restart`, or any command that would restart the Botka service.** You are running inside Botka — deploying would kill your own process and leave the task stuck in "running" state forever.