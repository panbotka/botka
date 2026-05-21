# Manual Task Status Transitions (REST + MCP)

**Status:** Approved
**Date:** 2026-05-21

## Problem

Tasks that the user works on directly inside Claude Code CLI (rather than
through the autonomous executor) cannot have their lifecycle reflected in
Botka. The executor sets `running`, `done`, `failed`, etc. internally, but
external clients (REST `PUT /api/v1/tasks/:id` and MCP `update_task`) are
gated by `allowedTransitions`, which permits none of those targets and also
blocks every update against a running task with "cannot update a running
task". The user wants to mark a task `running` when starting manual work,
and `failed` (or any other status) when finishing.

## Scope

Two call sites only:

- REST `PUT /api/v1/tasks/:id` — `TaskHandler.Update` in
  `internal/handlers/task.go`.
- MCP `update_task` tool — `Server.handleUpdateTask` in
  `internal/mcp/tools.go`.

Other call sites that consult `allowedTransitions` (`Retry`, `Bulk`,
`BatchUpdateStatus`, `applyBulkRequeue`) keep their current gating — they
are bulk/shortcut UI flows, not the single-task manual-management path.

Out of scope: new endpoints, new MCP tools, DB migrations, frontend
changes.

## Behavior

In both call sites:

1. **Open transitions.** Any value in `models.ValidStatuses` is accepted
   for the `status` field regardless of the current `task.Status`. The
   global `allowedTransitions` map stays in place for other handlers.

2. **Narrow the running lock.** Today the handler rejects every update
   whose task is currently `running`. After this change: when
   `task.Status == running`, reject only edits that change `title`,
   `spec`, or `priority`; status-only changes are allowed so the caller
   can move the task out of `running`.

3. **Auto-update timestamps** when a status change is part of the update:

   | New status | Side effect |
   |---|---|
   | `running` | `started_at = now()` only if currently `NULL` |
   | `done`, `failed`, `needs_review`, `cancelled` | `completed_at = now()` (unconditional, matches `runner.finalizeTask`) |
   | `pending`, `queued`, `deleted` | no timestamp change |

4. **Conflict on concurrent running.** The DB-level partial unique index
   `idx_one_running_per_project` (migration 016) prevents two `running`
   tasks per project. When the update tries to set `status=running` and
   the executor (or another caller) already has a running task on the
   same project, the unique violation surfaces as Postgres error code
   `23505`. Catch it and return:
   - REST: `409 Conflict`, body
     `{"error":"another task on this project is already running"}`.
   - MCP: tool error with the same message.
   Generic update failures continue to return `500`.

5. **Event publish.** REST already publishes a `TaskEvent` when status
   changes — unchanged. MCP does not publish today; no change.

6. **Validation.** `IsValid()` still rejects unknown status strings.
   Length checks on `title` and `spec` are unchanged.

## Files Touched

- `internal/handlers/task.go`
  - `validateUpdate` — drop the `allowedTransitions` lookup; tighten the
    running-task lock to only fields other than `status`.
  - `buildTaskUpdates` — add `started_at` / `completed_at` side effects
    based on the new status. Needs the current task value to make the
    "only if null" check for `started_at`, so its signature gains a
    `current models.Task` parameter.
  - `Update` — detect Postgres unique-violation (`pgconn.PgError.Code ==
    "23505"`, or `errors.Is(err, gorm.ErrDuplicatedKey)` if the project
    uses that wrapper) and return 409.

- `internal/mcp/tools.go`
  - `updateTaskArgs.buildUpdates` — drop the transition check; add the
    same timestamp side effects; take the full `models.Task` rather than
    just `current models.TaskStatus`.
  - `handleUpdateTask` — replace the blanket "cannot update a running
    task" check with the narrower title/spec/priority guard; catch
    unique violation and return a clean error.

- `internal/handlers/task_test.go`
  - Flip existing "invalid transition" cases to assert success.
  - Add: `queued → running` sets `started_at`; `running → failed` sets
    `completed_at` and is allowed; running task with non-status edit is
    rejected; concurrent-running returns 409.

- `internal/mcp/tools_test.go`
  - Same flip on transition cases; new cases for timestamp side effects
    and running-lock narrowing.

## Testing

Verification gate: `make check` (fmt + vet + lint + test + frontend
type-check) must pass before commit.

New tests live alongside existing ones — no new test files needed.
