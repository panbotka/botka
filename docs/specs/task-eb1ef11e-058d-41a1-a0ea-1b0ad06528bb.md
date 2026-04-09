## Problem

The dashboard RunnerStatus component shows "Stopped 0/2 active" even when there are tasks with status `running` in the database. This happens because the HTTP API `GET /api/v1/runner/status` returns only in-memory state from `Runner.GetStatus()` (which reads `r.state` and `r.executors`), while the MCP `get_runner_status` tool queries the database directly and correctly shows running tasks.

When the Botka process restarts while a Claude Code subprocess is executing a task, the in-memory executors map is empty and the runner state may be "stopped", but the task's DB status remains "running" (the Claude Code subprocess keeps running independently).

## Root Cause

`Runner.GetStatus()` at `internal/runner/runner.go:268` builds the response purely from in-memory state:
- `r.state` — may be "stopped" after a restart
- `r.executors` — empty after restart, only populated when the runner loop launches a task

Meanwhile, the DB may have tasks with `status = 'running'` that are orphaned from the in-memory perspective.

`RestoreState()` calls `recoverOrphanedTasks()` which requeues running tasks on startup, but this only works if the runner state was loaded correctly AND the process restart happened cleanly. If a task's Claude Code subprocess is still running after a Botka restart, there's a mismatch.

## Expected Behavior

The dashboard should never show "Stopped 0/2 active" when there are actually running tasks in the database. The runner status display should be consistent with reality.

## Proposed Fix

In `Runner.GetStatus()`, cross-reference the in-memory `r.executors` with the database. If there are tasks with `status = 'running'` in the DB that are NOT in `r.executors`, include them in the response with a flag indicating they are orphaned/untracked. This way the dashboard accurately reflects DB state.

Specifically:
1. In `GetStatus()`, query the DB for tasks with `status = 'running'` 
2. For any DB-running task NOT in `r.executors`, add it to `active_tasks` with an `orphaned: true` flag
3. Update the `ActiveTaskInfo` struct to include an `Orphaned bool` field
4. Update the frontend `RunnerStatus` component to show orphaned tasks differently (e.g., with a warning indicator)
5. Optionally, if there are orphaned running tasks and runner state is "stopped", show a warning banner

Alternative simpler approach: make the MCP `get_runner_status` and HTTP `GetStatus()` consistent by having both use DB state as the source of truth for active tasks, with in-memory state only for the runner loop status (running/paused/stopped).

## Files to Modify

- `internal/runner/runner.go` — `GetStatus()` method, `ActiveTaskInfo` struct
- `frontend/src/types/index.ts` — `ActiveTaskInfo` type
- `frontend/src/components/RunnerStatus.tsx` — display orphaned tasks

## Testing

- Unit test: `GetStatus()` returns orphaned tasks from DB when executors map is empty
- Integration test: simulate process restart scenario (task in DB as running, empty executors)
- Verify dashboard displays correctly with orphaned tasks
- Run `make check` before finishing