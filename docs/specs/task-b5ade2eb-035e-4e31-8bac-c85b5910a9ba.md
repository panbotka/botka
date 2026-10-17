# "Running now / Up next" panel on the Tasks page

The user's two real questions on the Tasks page are: "what is running and what comes next?" and (when nothing runs) "which pending tasks do I queue?". The current "All" tab answers neither. Add a compact panel above the status tabs that answers the first question, and make Pending the default tab for the second.

## Requirements

### Panel (above the status tabs, `frontend/src/pages/TasksPage.tsx`)
- Section "Běží teď": every task with status `running`. Each row: title, project name, run phase label (reuse `frontend/src/utils/runPhase.ts`, e.g. "ověřování"), and a live elapsed time since `started_at` (ticks every second). Click opens the task detail.
- Section "Následuje": queued tasks in the exact order the runner will pick them — `priority DESC, created_at ASC` (same as `buildPickQuery` in `internal/runner/runner.go`). Show the first 5 with a "+N dalších" link that switches to the Queued tab. Each row: position number, title, project, priority.
- A queued task whose project already has a running task gets a small muted badge "čeká na projekt" (the runner skips it until that project frees up).
- Panel header shows the runner state (running / paused / stopped) and "X/Y workerů" using the existing runner status API/hook, so the user can see at a glance why nothing is moving (e.g. runner paused or rate-limit gate).
- When nothing is running AND the queue is empty, the panel collapses into a single muted line ("Nic neběží, fronta je prázdná").
- When nothing is running but the queue is non-empty, show "Nic neběží" plus the Up-next list.
- The panel updates live via the existing task SSE events (`task_status` etc.) — no polling timer beyond what the page already does.
- Mobile-friendly: rows are single-line with truncation, tap targets at least 44px tall.

### Tabs
- Remove the "All" tab entirely (and its count). An old URL `?status=` empty/`all` falls back to the default.
- Default tab when there is no `?status=` in the URL: **Pending** (running/queued are already visible in the panel). Remove the old "Running → Queued → Pending → All" auto-selection logic.
- Other tabs (Pending, Queued, Running, Done, Failed, Needs Review, Deleted) stay.

## Edge Cases
- Many running tasks (MAX_WORKERS > 2): list them all.
- Project filter (`?project=`) active: the panel respects it (shows only that project's running/queued tasks).
- Search query active: the panel is unaffected by search.

## Implementation Notes
If the list API cannot cheaply return what the panel needs, fetch `status=running` and `status=queued` (limit 50) separately; the default list sort is already priority DESC, created_at ASC (`internal/handlers/task.go`). Put the panel in its own component with a Vitest test covering ordering, the "čeká na projekt" badge and the collapsed empty state. `make check` must pass.