Fix the Shift+Tab keyboard shortcut for toggling plan mode so it works regardless of focus, not just when the textarea is focused.

## Problem

In `frontend/src/components/ChatInput.tsx:113-117`, the Shift+Tab handler is on the textarea's `onKeyDown`:

```tsx
const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Tab' && e.shiftKey) {
      e.preventDefault();
      onTogglePlanMode?.();
      return;
    }
```

This only fires when the textarea has focus. If the user clicks anywhere else on the page (messages, sidebar, etc.), Shift+Tab does nothing.

## Fix

Add a global `keydown` listener for Shift+Tab that toggles plan mode. The best place is in `ChatView.tsx` where `planMode` state lives (`line 65: const [planMode, setPlanMode] = useState(false)`).

Add a `useEffect` in `ChatView.tsx` that:
1. Listens for `keydown` on `window`
2. Checks for `e.key === 'Tab' && e.shiftKey`
3. Calls `e.preventDefault()` and toggles `setPlanMode(p => !p)`
4. Only activates when the chat view is the active view (thread is selected / `threadId` exists)

Keep the existing textarea handler too — it provides a fast path and prevents the default Tab behavior inside the input.

Make sure to `e.preventDefault()` to avoid the browser's default reverse-tab-navigation behavior.

## Verification

- Build frontend with `cd frontend && npm run build` to ensure no errors
- The Shift+Tab shortcut should toggle plan mode from anywhere on the chat page, not just the textarea
- The mouse toggle button should continue to work as before