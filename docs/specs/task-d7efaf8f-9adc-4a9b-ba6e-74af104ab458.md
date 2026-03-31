# Kill Running Task with Git Revert

Add the ability to kill a running task execution — terminate the Claude Code process, revert all git changes the agent made, and mark the task as failed.

## Requirements

### 1. Store Git HEAD Before Execution

- In `internal/runner/executor.go`, before spawning the Claude process, capture the current `git rev-parse HEAD` SHA of the project directory
- Add a `GitHeadSHA` field (`git_head_sha`, string, nullable) to the `TaskExecution` model
- Store the captured SHA in the execution record so it can be used for revert after kill
- Create a database migration for the new column

### 2. Runner: KillTask Method

- Add a `KillTask(taskID uuid.UUID) error` method to `Runner` in `internal/runner/runner.go`
- Find the active executor for the given task ID in `r.executors`
- Return an error if the task is not currently running
- Call `cancel()` on the executor's context (this sends SIGTERM to the process group via the existing `cmd.Cancel` mechanism)
- The existing finalization flow in `launchTask` goroutine will handle the rest — the method just triggers the kill

### 3. Executor: Git Revert After Kill

- In the `finalize()` or post-execution logic in `executor.go`, detect when a task was killed (context cancelled, not timed out)
- Add a `Killed` boolean field to the `execOutput` struct to distinguish user-kill from timeout
- When a task is killed, perform git revert in the project directory:
  - Run `git reset --hard <saved-head-sha>` to undo any commits the agent made
  - Run `git clean -fd` to remove any untracked files
  - If the project uses `feature_branch` strategy: also `git checkout main` (or whatever the default branch is) and `git branch -D botka/task-<id>` to delete the feature branch
- Set the task status to `failed` with failure reason "Killed by user"
- Set retry count to max (so it won't auto-retry — killed tasks should stay failed)
- Log the revert operations

### 4. API Endpoint

- Add `POST /api/v1/tasks/:id/kill` endpoint in `internal/handlers/task.go`
- Handler should:
  - Parse task ID from URL param
  - Call `runner.KillTask(taskID)`
  - Return 200 on success with `{"data": {"message": "Task kill initiated"}}`
  - Return 404 if task not found or not running
  - Return 409 if task is not in running state
- Register the route in `internal/handlers/routes.go`
- The handler needs access to the Runner instance — follow the same pattern as other runner endpoints (RunnerStart, RunnerStop, etc.)

### 5. MCP Tool

- Add a `kill_task` tool in `internal/mcp/tools.go`
- Input: `task_id` (string, required) — UUID of the task to kill
- Calls the same Runner.KillTask method
- Returns success message or error
- Register the tool in the tool list and handler

### 6. Frontend: Kill Button

- In `frontend/src/pages/TaskDetailPage.tsx`, add a "Kill" or "Stop" button that appears ONLY when the task status is `running`
- The button should be visually distinct (red/destructive style, similar to delete buttons)
- Show a confirmation dialog before killing: "This will terminate the running task and revert all changes. Continue?"
- On confirm, call `POST /api/v1/tasks/:id/kill`
- After successful kill, refresh task data (the status will change to `failed`)
- Add the API call function in `frontend/src/api/tasks.ts`

### 7. Tests

- Unit test for `KillTask` method on Runner (mock executor, verify cancel is called)
- Handler test for `POST /tasks/:id/kill` — test success, task not found, task not running cases
- Test that git revert logic runs the correct commands (can mock exec)
- MCP tool test for kill_task

## Edge Cases

- If the Claude process has already exited by the time kill is called, the revert should still happen based on the stored HEAD SHA
- If git revert fails (e.g., corrupted repo), log the error but still mark the task as failed — don't leave it stuck in running
- If the task has no stored HEAD SHA (e.g., failed before git capture), skip the revert step gracefully
- The kill should work regardless of branch strategy (main or feature_branch)
- Concurrent kill requests for the same task should be idempotent (second call returns "not running")

## Implementation Notes

- The Runner already has `HardStop()` that kills ALL executors — `KillTask` is the single-task equivalent
- Process group kill via `syscall.Kill(-pid, SIGTERM)` is already implemented in executor's `cmd.Cancel`
- The `activeTask` struct in runner.go already holds `cancel context.CancelFunc` — this is what triggers the kill
- Follow existing patterns: look at how `HardStop`, `handleRunnerStop`, and retry logic work