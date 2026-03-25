## Problem

When the user has the same chat thread open in multiple browser tabs, messages (both sent and received/streamed) only appear in the tab that initiated the conversation. Other tabs miss the messages entirely until a manual refresh.

## Requirements

1. **Cross-tab message sync** — when a message is sent or streamed in one tab, all other tabs showing the same thread should receive and display it in real-time.
2. Both user messages and assistant responses (including streaming) should sync.
3. No message duplication — if a tab already has the message, don't add it again.

## Implementation approach

Use the **BroadcastChannel API** — a simple browser-native API for cross-tab communication. No server changes needed.

### How it works

1. Create a `BroadcastChannel` named e.g. `botka-chat-sync`
2. When a tab adds a new message (user sends or assistant response completes), broadcast it: `channel.postMessage({ type: 'new-message', threadId, message })`
3. When a tab receives a streaming chunk, optionally broadcast partial updates (or just broadcast the final complete message to keep it simple)
4. When other tabs receive the broadcast, check if it's for the currently displayed thread, and if so, append the message to their local state (if not already present, checking by message ID)

### Files to modify

- `frontend/src/components/ChatView.tsx` — this is where messages are managed. Add BroadcastChannel logic:
  - On mount: create channel, listen for messages
  - On new message added to state: broadcast it
  - On receiving broadcast: append to local messages if same thread and not duplicate
  - On unmount: close channel
- Alternatively, create a new hook `frontend/src/hooks/useChatSync.ts` to encapsulate the BroadcastChannel logic and use it in ChatView.

### Key considerations

- **Dedup by message ID** — messages from the DB have numeric IDs. Only add if not already in the local messages array.
- **Streaming messages** — the simplest approach is to only broadcast completed messages (after streaming finishes), not partial streaming chunks. This avoids complexity with partial state sync. Other tabs will see the full message appear once streaming completes.
- **User messages** — broadcast immediately when the user sends a message, so other tabs see it right away.
- **Thread matching** — only process broadcasts for the currently active thread.
- **Tab focus** — when a tab regains focus, it could also refetch messages from the API as a fallback to catch anything missed.

### Example

```typescript
// In ChatView or a custom hook
const channel = new BroadcastChannel('botka-chat-sync');

// When a new message is confirmed (has ID from server)
channel.postMessage({ type: 'new-message', threadId, message });

// Listen
channel.onmessage = (event) => {
  if (event.data.type === 'new-message' && event.data.threadId === currentThreadId) {
    setMessages(prev => {
      if (prev.some(m => m.id === event.data.message.id)) return prev;
      return [...prev, event.data.message];
    });
  }
};
```

## Verification

1. Run `make check`
2. Open same thread in two tabs, send a message in one — it should appear in both

## CRITICAL: Do NOT deploy or restart

**NEVER run `make deploy`, `make install-service`, `systemctl restart`, or any command that would restart the Botka service.** You are running inside Botka — deploying would kill your own process and leave the task stuck in "running" state forever.