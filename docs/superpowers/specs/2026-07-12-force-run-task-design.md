# Force-Run a Queued Task (bypass rate-limit gates)

**Status:** Approved
**Date:** 2026-07-12

## Problem

When the Claude 5-hour usage limit is (near-)exhausted, the scheduler stops
pulling tasks off the queue. `Runner.tick()`
(`internal/runner/runner.go:503`) applies two gates that each `return`
before a task is ever picked:

1. **UsageMonitor** (`internal/runner/usage.go:95`, `IsRateLimited()`) —
   blocks when 5h utilization exceeds `USAGE_THRESHOLD_5H` (default 0.90)
   or 7d exceeds `USAGE_THRESHOLD_7D` (default 0.95).
2. **RateLimitGate** (`internal/runner/ratelimit.go`) — a cooldown pause
   armed after a task fails with a rate-limit error.

A queued task therefore sits in `queued` indefinitely. The existing
"Clear rate limit" control only clears gate #2 (the `RateLimitGate`); it
does **not** override gate #1 (the UsageMonitor threshold), which is the
usual cause of "vyčerpaný 5h limit". There is no way to say "run *this*
one task now anyway".

The user wants a manual, per-task escape hatch: force a specific queued
task to start immediately, bypassing both rate-limit gates, while other
queued tasks stay gated.

## Scope

- New runner method `Runner.ForceRunTask(taskID)`.
- New REST endpoint `POST /api/v1/tasks/:id/force-run`.
- New API client method + a "Spustit teď" button on queued tasks in the
  task detail page.

Out of scope (YAGNI):

- **No MCP tool** — this is a web-UI escape hatch only.
- **No force for `pending`/`failed`/`needs_review`** — those are moved to
  `queued` first via the existing Queue/Retry controls. Force operates on
  `queued` only, matching the exact "z queue do running" request.
- **No global gate clearing** — the two rate-limit gates are left armed so
  every *other* queued task stays blocked. Force is per-task.
- No new DB columns or migrations (no `force` flag persisted; the action
  is out-of-band, not scheduler-driven).
- No task-list-row button (detail page only, mirroring where Kill/Retry
  already live).

## Behavior

Force bypasses **throttling** but honors **correctness/resource**
constraints. Concretely, `ForceRunTask` does *not* consult either
rate-limit gate and does *not* require the scheduler loop to be actively
picking, but it still enforces:

- **Runner not hard-stopped.** Works in `StateRunning` and `StatePaused`
  (an explicit per-task override of the paused/gated loop). Refuses when
  `state == StateStopped` — the user turned the whole runner off.
- **MAX_WORKERS.** Refuses if all worker slots are occupied.
- **One running task per project.** Refuses if the task's project already
  has a running task (in-memory executor *and* the DB partial unique index
  `idx_one_running_per_project`).
- **Task is `queued`.** Refuses any other status.

Because the task then runs through the normal `executeTask` path, a forced
task that still fails on a genuine rate-limit error will re-arm the
`RateLimitGate` via `finishTask → maybeTripRateLimitGate` — the automatic
gate rearms itself. Force is a one-shot manual attempt, not a gate-off
switch.

### `Runner.ForceRunTask(taskID uuid.UUID) error`

Reuses the existing claim/launch machinery. Sequence:

1. Load the task read-only. Not found → return `gorm.ErrRecordNotFound`
   (handler maps to 404).
2. `task.Status != queued` → return `ErrTaskNotQueued`.
3. Under `r.mu`, pre-flight check (mirrors `collectTickState` +
   `launchTask` guards):
   - `r.state == StateStopped` → `ErrRunnerStopped`.
   - `len(r.executors) >= r.maxWorkers` → `ErrWorkersBusy`.
   - `r.executors[task.ProjectID]` present → `ErrProjectBusy`.
   Release the lock before the DB transaction (do not hold `r.mu` across
   I/O; the claim's DB unique index and `launchTask`'s re-check are the
   final authority, exactly as in the normal tick path).
4. `pickTaskByID(taskID)` — a by-ID variant of `pickNextTask`
   (`runner.go:575`): begins a tx, runs
   `WHERE id = ? AND status = 'queued'` plus the same
   `NOT EXISTS (running task on same project)` subquery and
   `FOR UPDATE SKIP LOCKED`, with **no gate checks and no ordering**, then
   calls the unchanged `claimTask` (`runner.go:634`). Record-not-found
   (status changed or project became busy between steps) →
   `ErrTaskNotQueued`. Unique violation (`isUniqueViolation`) →
   `ErrProjectBusy`.
5. `launchTask(task, execution)` — **change its signature to return
   `bool`** (`true` when the goroutine was started, `false` on the two
   existing early-return unclaim paths). `tick()` ignores the return
   value; `ForceRunTask` treats `false` as a lost race and returns
   `ErrProjectBusy`. This makes the API honest: it returns success only
   when the task actually started.

Typed sentinel errors live in the runner package
(`ErrRunnerStopped`, `ErrTaskNotQueued`, `ErrWorkersBusy`,
`ErrProjectBusy`) so the handler can map each to a message without string
matching.

### REST `POST /api/v1/tasks/:id/force-run`

New method `RunnerHandler.ForceRun`, registered next to the existing
`POST /api/v1/tasks/:id/kill` route (`RunnerHandler.KillTask`,
`internal/handlers/runner.go`). It parses the UUID, calls
`runner.ForceRunTask(id)`, and maps results:

| Result | HTTP | Body |
|---|---|---|
| launched | `200` | `{"data": <task>}` (reloaded, now `running`) |
| `gorm.ErrRecordNotFound` | `404` | `{"error":"task not found"}` |
| `ErrTaskNotQueued` | `409` | `{"error":"task is not queued"}` |
| `ErrRunnerStopped` | `409` | `{"error":"runner is stopped; start it first"}` |
| `ErrWorkersBusy` | `409` | `{"error":"all workers are busy"}` |
| `ErrProjectBusy` | `409` | `{"error":"another task on this project is already running"}` |
| other | `500` | `{"error": <msg>}` |

### Frontend

- `frontend/src/api/client.ts` — add `forceRunTask(id: string):
  Promise<Task>` mirroring `retryTask`/`killTask` (lines 169-173),
  hitting `POST /tasks/:id/force-run` and returning `data`.
- `frontend/src/pages/TaskDetailPage.tsx` — the Actions block
  (`task.status !== 'running'`, ~lines 417-448) currently has no branch
  for `queued`. Add one: a "Spustit teď" button (Play/Zap icon) shown when
  `task.status === 'queued'`, wired to a `handleForceRun` handler that
  mirrors `handleRetry` (set `acting`, call `forceRunTask`, refetch;
  surface the 409 error message to the existing error UI). A `title`
  tooltip notes it bypasses the rate-limit gate. No confirmation modal.

## Files Touched

- `internal/runner/runner.go`
  - Add `ErrRunnerStopped`, `ErrTaskNotQueued`, `ErrWorkersBusy`,
    `ErrProjectBusy` sentinel errors.
  - Add `ForceRunTask(taskID uuid.UUID) error`.
  - Add `pickTaskByID(taskID uuid.UUID) (*models.Task,
    *models.TaskExecution, error)` (by-ID sibling of `pickNextTask`).
  - Change `launchTask` to return `bool`; update the single `tick()`
    call site to ignore the value.
- `internal/handlers/runner.go`
  - Add `RunnerHandler.ForceRun`; register
    `POST /api/v1/tasks/:id/force-run`.
- `frontend/src/api/client.ts` — add `forceRunTask`.
- `frontend/src/pages/TaskDetailPage.tsx` — add `handleForceRun` + the
  `queued` "Spustit teď" button.

## Testing

Verification gate: `make check` (fmt + vet + lint + test + frontend
type-check) must pass before commit.

- `internal/runner/*_test.go`
  - Force-run launches a `queued` task even when the injected
    `UsageMonitor` reports `IsRateLimited() == true` (proves gate #1 is
    bypassed) — assert the task transitions to `running` and an executor
    is registered.
  - Refuses with `ErrWorkersBusy` when `maxWorkers` slots are full.
  - Refuses with `ErrProjectBusy` when the project already has a running
    executor; no second executor is created (one-per-project preserved).
  - Refuses with `ErrTaskNotQueued` for a non-queued task.
  - Refuses with `ErrRunnerStopped` when `state == StateStopped`.
- `internal/handlers/runner_test.go` (integration, needs `botka_test`)
  - `200` + status flips to `running` on success.
  - `409` with the right message for a busy/non-queued task.
  - `404` for a missing task id.
- `frontend/src/pages/TaskDetailPage.test.tsx` (or a focused test)
  - "Spustit teď" renders only for `queued` tasks.
  - Clicking calls `forceRunTask` and refetches.
