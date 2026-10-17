# Selection mode for the task list

On a phone, the row checkbox in the task list is 14px with no enlarged hit area, and the row itself navigates to the task detail on click. Missing the checkbox by a few pixels throws the user out of the list. The most common bulk action is moving several pending tasks to queued. Make selecting tasks reliable and fast.

## Requirements

### Selection mode
- Add a "Vybrat" button in the task list toolbar (next to search/filters). Tapping it enters selection mode; it becomes "Hotovo" to exit.
- Long-press (~500ms) on any row also enters selection mode and selects that row. A long-press must not also trigger navigation or the browser context menu/text selection.
- In selection mode, tapping ANYWHERE on a row toggles its selection and NEVER navigates. Selected rows get a clear highlighted background.
- Selecting the first checkbox outside selection mode (desktop) automatically enters selection mode too.
- Exiting selection mode clears the selection. Selection is also cleared when switching tabs.
- Outside selection mode, row click navigates to the detail as today.

### Hit area
- The checkbox renders visibly larger (~20px) and its clickable area is at least 44×44px (padding / label wrapping the whole cell), on all screen sizes.
- In selection mode, the drag handle and the inline status dropdown are disabled/hidden so they can't be hit by accident.

### Select helpers
- In selection mode show a "Vybrat vše" toggle that selects all rows on the current tab/page (and deselects if all are selected).

### Bulk bar
- When at least one task is selected, the bulk actions bar shows "N vybráno".
- On mobile the bar is docked at the bottom ABOVE the bottom navigation (never covering it, respecting `env(safe-area-inset-bottom)`), full width, not a floating pill that wraps.
- The primary action "Do fronty" (pending → queued, the existing Requeue action) is visually prominent (filled accent button) and shown first. Less frequent actions (move to pending, priority, cancel, delete) go into a "⋯" overflow menu on mobile; on desktop they can stay inline.
- After a successful bulk action, exit selection mode.
- The list content gets enough bottom padding that the last row is not hidden behind the bar.

## Implementation Notes
Files: `frontend/src/components/TaskList.tsx`, `frontend/src/components/BulkActionsBar.tsx`, `frontend/src/pages/TasksPage.tsx`; use the existing `useIsMobile` hook. Note: the app shell uses an in-flow bottom nav (not `position: fixed`) and `lvh` heights — read the "Bottom nav" and "Mobile app-shell height" notes in CLAUDE.md and do not break them. Add Vitest tests: row click in selection mode toggles and does not navigate; outside selection mode it navigates. `make check` must pass.