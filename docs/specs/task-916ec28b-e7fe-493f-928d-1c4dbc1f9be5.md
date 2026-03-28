# Fix Scheduler: Enforce One Task Per Project

## Problem

The scheduler allowed two tasks from the same project (botka) to run simultaneously. The constraint "one task per project" is supposed to prevent git conflicts, but it was violated.

Observed: Two botka tasks running at the same time (both started at 11:15):
- "Fix command execution: track child processes and support long-running dev commands"  
- "Add MCP tools for project settings, commands, threads, and thread sources"

## Current Architecture

The scheduler loop in `internal/runner/runner.go`:

1. `tick()` is called every 5 seconds from a single goroutine (lines 276-293)
2. `collectTickState()` (line 319) gathers `activeProjectIDs` from `r.executors` map under lock
3. `pickNextTask()` (line 355) queries DB with `WHERE project_id NOT IN (activeProjectIDs)` and `FOR UPDATE SKIP LOCKED`
4. `launchTask()` (line 433) adds to `r.executors[task.ProjectID]` under lock

Since the loop runs in a single goroutine, ticks are sequential. Between tick N adding a task to `executors` and tick N+1 reading `executors`, the project should be excluded.

## Investigate & Fix

The root cause needs investigation. Possible causes:

1. **Double-add in launchTask:** `r.executors[task.ProjectID]` uses project ID as key. If two tasks from the same project get picked, the second overwrites the first in the map silently. The DB query filters by active project IDs, but verify the filtering actually works correctly — maybe the project_id column has NULL or unexpected values.

2. **recoverOrphanedTasks race:** On startup, `recoverOrphanedTasks()` requeues stuck running tasks. If both tasks were running when Botka restarted, they get requeued. The first tick picks one, but check if there's a path where the loop processes multiple tasks per tick or where `startLocked` calls both `recoverOrphanedTasks` and starts the loop before the orphaned tasks are fully committed to the DB.

3. **Manual status change:** If someone queued a task via API while another was already running for the same project, the scheduler picks it on the next tick. The DB filter should prevent this, but verify the query is correct.

4. **startLocked called multiple times:** If `Start()` or `RestoreState()` gets called while a loop is already running, there might be two concurrent loops. The `r.stopCh == nil` check should prevent this, but verify.

## Required Fix

Regardless of root cause, add a **belt-and-suspenders check** in `launchTask()`:

```go
func (r *Runner) launchTask(task *models.Task, execution *models.TaskExecution) {
    r.mu.Lock()
    if existing, ok := r.executors[task.ProjectID]; ok {
        r.mu.Unlock()
        slog.Error("scheduler: refusing to launch second task for same project",
            "project_id", task.ProjectID,
            "existing_task", existing.task.ID,
            "new_task", task.ID)
        // Requeue the task so it can be picked up later
        r.db.Model(task).Update("status", models.TaskStatusQueued)
        return
    }
    // ... rest of launch logic
}
```

Also add logging to `collectTickState` and `buildPickQuery` to make it easier to diagnose if this happens again:
- Log the active project IDs being excluded
- Log the task that was picked and its project ID

## Tests

- Add a test that verifies: if a task is running for project X, `buildPickQuery` does NOT return another task for project X
- Add a test that verifies: `launchTask` refuses to launch if an executor already exists for the same project
- Run `make check` to ensure everything passes