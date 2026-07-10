# Make a retried task's state honest

When a task fails and the runner retries it (`maxRetries = 1`), three pieces of state lie about
what happened. Two mislead the reader; the third can mislead the agent into corrupting the work.

Observed on task `1fb2cbab-0881-4127-855d-5e5a96f57930`: attempt 1 timed out after 30 minutes with
`duration=0ms`, attempt 2 succeeded in 6 minutes. The task is now `done` and still carries
`failure_reason = "execution timed out"`, and its `started_at`/`completed_at` span 36 minutes.

## Requirements

### The working tree must be clean when a retry starts

- `GitRevert` currently runs only when `result.ErrorMessage == "Killed by user"` (`runner.go:772`).
  A timed-out or failed attempt leaves its partial work — possibly a commit — in the tree.
- Before a retry is launched, reset the project to the task's `base_commit_sha` so attempt 2 starts
  from the same state attempt 1 did. Verify `base_commit_sha` is actually populated before relying
  on it; if it is empty, skip the reset and log a warning rather than resetting to a guess.
- This makes the existing "No session continuity — each execution is standalone" claim in
  `CLAUDE.md` true for the filesystem too, not just the Claude session.

### The retry prompt must describe the situation

- `buildPrompt` (`executor.go:239`) appends only `Previous attempt failed with: <reason>.`
  Extend it to also state that the working tree was reset to the task's starting commit, so the
  agent does not mistake its own reverted work for someone else's, and does not try to resume it.
- Keep it to one or two sentences. Do not feed the previous attempt's output into the prompt.

### Timings and errors must describe the attempt that actually ran

- On any successful terminal status (`done`, `needs_review`), clear `failure_reason`. A finished
  task must not carry an error from an attempt that was superseded.
- `started_at` must reflect the attempt currently running, not the first one, so a task's displayed
  duration is the duration of the work rather than the work plus a promoted timeout.
- Per-attempt history stays in `task_executions`, which already records each attempt separately.
  Do not collapse or rewrite it.

## Verification

`make check` passes. Add Go tests for: (a) `failure_reason` cleared when a retried task reaches
`done`, (b) `started_at` advanced on relaunch, (c) the retry path invoking the revert and the
non-retry path not invoking it, (d) `buildPrompt` mentioning the reset only when `retry_count > 0`.
Update the task-mode section of `CLAUDE.md` to describe the retry contract.