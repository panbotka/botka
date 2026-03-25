# Fix chat scroll position when switching back to thread with new messages

When the user sends a message, switches to another thread, and the AI response arrives in the background, returning to the original thread shows the top of the conversation instead of the latest message.

## Requirements

- When switching to a thread, the chat view must scroll to the bottom (most recent message)
- This must work when new messages arrived while the thread was not actively viewed (e.g. AI finished responding in the background)
- Scroll-to-bottom should happen after messages are loaded/rendered, not before
- Do NOT break existing auto-scroll behavior during active streaming (user watching a response stream in real-time should still work as before)
- The fix should be in `frontend/src/components/ChatView.tsx` or the relevant scroll-handling logic

## Implementation Notes

- Look at how `ChatView.tsx` handles scroll position — there's likely a `scrollToBottom` or similar function that needs to trigger on thread switch / message list update
- The issue is probably that scroll-to-bottom only triggers during streaming events, not when re-entering a thread with new messages already loaded