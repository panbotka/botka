Chat messages are not scrollable on mobile (Safari and PWA). Instead of scrolling within the chat message area, the entire page scrolls. This makes it impossible to scroll through conversation history on phones.

## Current behavior
- On mobile (iPhone Safari + PWA), swiping up/down in the chat area scrolls the whole page instead of just the messages
- The chat message container doesn't capture scroll events
- This is a fundamental usability issue — long conversations are unusable on mobile

## Expected behavior
- The chat message area scrolls independently within its container
- The page layout is fixed: header at top, input at bottom, messages scroll in between
- Swiping in the message area scrolls only the messages, not the whole page
- Auto-scroll to bottom on new messages still works

## Root cause (likely)
The chat message container is probably missing the correct CSS to make it a scrollable container. On mobile Safari, `overflow-y: auto/scroll` needs specific conditions to work:
1. The container must have a **fixed/explicit height** (not just `flex-grow`)
2. On iOS, `-webkit-overflow-scrolling: touch` may be needed (though modern iOS should handle this)
3. The parent layout must constrain the height so the chat container doesn't just grow to fit all content

## Requirements

### Fix the chat layout to be a fixed viewport layout
- The chat page should use a full-viewport-height layout:
  - Header: fixed height at top
  - Messages: fills remaining space, scrolls internally (`overflow-y: auto`)
  - Input area: fixed height at bottom
- Use `h-dvh` (dynamic viewport height) or `h-screen` on the outer container
- The message container needs `overflow-y: auto` AND a constrained height (via flex layout)
- Typical pattern:
  ```
  <div class="flex flex-col h-dvh">        <!-- full viewport -->
    <header class="shrink-0">...</header>   <!-- fixed header -->
    <div class="flex-1 overflow-y-auto min-h-0">  <!-- scrollable messages -->
      ...messages...
    </div>
    <div class="shrink-0">...</div>         <!-- fixed input -->
  </div>
  ```
- The key CSS trick: `min-h-0` on the flex child is critical — without it, flex children won't shrink below their content size and overflow won't activate

### Mobile-specific considerations
- Test with `overflow-y: auto` (not `scroll` — `auto` is preferred on mobile)
- `min-h-0` on the scrollable flex child is the most common fix for this exact issue
- Ensure `overscroll-behavior: contain` on the message container to prevent scroll chaining to the page
- If the page body itself scrolls, consider `overflow: hidden` on the body/html when on the chat page

### Preserve existing behavior
- Auto-scroll to bottom when new messages arrive must still work
- The scroll-to-bottom button (if any) must still work
- Desktop scrolling should not be broken
- The input area should stay visible (not scroll away)

## Implementation Notes
- The fix is in `ChatPage.tsx` or `ChatView.tsx` — wherever the chat layout is structured
- Look for the message list container and check its CSS: does it have `overflow-y-auto`? Does it have a constrained height?
- The most common cause is a missing `min-h-0` on a flex child, or the container using `h-full` without a height-constrained parent chain all the way up to the viewport
- Check if `ChatPage` wraps content in a full-height layout or if it relies on the app shell
- Existing tests must pass (`make check`)

## Safety
**NEVER run `make deploy`, `make install-service`, `systemctl restart botka`, or any command that would restart the Botka service.**