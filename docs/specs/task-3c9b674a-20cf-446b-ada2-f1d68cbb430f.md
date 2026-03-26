Show the time each message was sent in the chat UI. The timestamp should appear on hover (tooltip or subtle inline reveal), not permanently visible, to keep the UI clean.

## Requirements

### Frontend — Timestamp display
- For each message bubble in the chat, show the sent time (HH:MM format) on mouse hover
- Implementation options (pick whichever fits best with existing UI):
  - CSS-only: hidden by default, `opacity-0 group-hover:opacity-100` transition
  - Or a native `title` attribute tooltip with the full datetime
- Position: next to or below the message bubble, small muted text (e.g. `text-xs text-zinc-400`)
- For today's messages: show just time (e.g. "14:32")
- For older messages: show date + time (e.g. "25. 3. 14:32")

### Data
- Messages already have `created_at` timestamp from the database
- The frontend `Message` type should already include this field from the API response
- No backend changes should be needed — just use the existing timestamp

### Styling
- Subtle and unobtrusive — should not change the visual weight of the chat
- Follow existing design language (Tailwind, zinc palette)
- Works for both user and assistant message bubbles

## Implementation Notes
- Changes are in `MessageBubble.tsx` or wherever individual messages are rendered
- Existing tests must pass (`make check`)

## Safety
**NEVER run `make deploy`, `make install-service`, `systemctl restart botka`, or any command that would restart the Botka service.**