## Problem

When the user navigates away from a chat thread (e.g. to Settings or another page), the SSE connection is closed because ChatView unmounts. If the user returns within a few minutes, they've lost the streaming session and can't resume it.

The user expects to be able to leave and come back to an active session within 5 minutes.

## Requirements

1. **SSE streaming sessions must stay alive for at least 5 minutes** after the user navigates away from the chat.
2. When the user returns to the same thread, they should reconnect to the still-active session and see any messages that arrived while away.
3. If 5 minutes pass with no reconnection, the session can be cleaned up normally.

## Context

The backend already has a session pool (`internal/claude/pool.go`) that pre-warms Claude processes and keeps them alive for 5 minutes via a stdin keepalive. The issue is purely on the **frontend side** — the SSE `EventSource` is tied to the ChatView component lifecycle.

## Implementation approach

### Move SSE management out of ChatView to a persistent layer

The SSE connection should live **above** the route-level components so it survives navigation. Options:

1. **Global SSE context/manager** — Create a React context or singleton that manages SSE connections per thread. ChatView subscribes to it on mount and unsubscribes on unmount, but the actual EventSource stays open.

2. **Implementation sketch:**
   - Create `frontend/src/hooks/useSSEManager.ts` or `frontend/src/context/SSEContext.tsx`
   - The manager holds a `Map<threadId, { eventSource, messages[], startedAt }>`
   - When a chat sends a message and starts streaming, the manager opens the SSE connection
   - ChatView subscribes to the manager for the current threadId — receives live events
   - When ChatView unmounts, it unsubscribes but the manager keeps the EventSource open
   - When ChatView remounts for the same thread, it resubscribes and gets buffered messages + live stream
   - After 5 minutes of no subscribers, the manager closes the EventSource and cleans up

3. **Place the provider in App.tsx** above the Router so it persists across all routes.

### Message buffering

While no ChatView is subscribed:
- The manager continues receiving SSE events
- New messages are buffered in the manager's state
- When ChatView remounts and subscribes, it receives all buffered messages

### Cleanup

- Sessions older than 5 minutes with no active subscriber are cleaned up
- When streaming completes (SSE closes), mark the session as done but keep buffered messages for when the user returns

## Files to modify

- **New file:** `frontend/src/context/SSEContext.tsx` or `frontend/src/hooks/useSSEManager.ts` — persistent SSE connection manager
- `frontend/src/App.tsx` — wrap routes with SSE provider
- `frontend/src/components/ChatView.tsx` — refactor to use SSE manager instead of managing EventSource directly
- Check current SSE connection code in ChatView to understand what events are handled and how messages are processed

## Verification

1. Run `make check`
2. Send a message, navigate to Settings, wait 10 seconds, come back — streaming should continue or completed message should be visible
3. Send a message, navigate away, wait 6 minutes, come back — session can be gone, messages should still be persisted from API fetch

## CRITICAL: Do NOT deploy or restart

**NEVER run `make deploy`, `make install-service`, `systemctl restart`, or any command that would restart the Botka service.** You are running inside Botka — deploying would kill your own process and leave the task stuck in "running" state forever.