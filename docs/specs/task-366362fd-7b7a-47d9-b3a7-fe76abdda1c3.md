# Cron Jobs — API Handlers & Claude Executor

Add REST API endpoints for managing cron jobs and implement the actual Claude Code execution for scheduled prompts.

## Context

The database tables (`cron_jobs`, `cron_executions`), GORM models, and cron scheduler goroutine already exist (from a previous task). The scheduler calls a placeholder `executeCronJob()` method. This task replaces the placeholder with real Claude execution and adds the full CRUD API.

## Requirements

### API Endpoints

Create `internal/handlers/cron_handler.go` following existing handler patterns.

**CRUD:**

- `GET /api/v1/cron-jobs` — List all cron jobs with their project name (preload Project). Ordered by name. Response: `{"data": [...], "total": N}`
- `GET /api/v1/cron-jobs/:id` — Get single cron job with project info. Response: `{"data": {...}}`
- `POST /api/v1/cron-jobs` — Create cron job. Required: name, schedule, prompt, project_id. Optional: enabled, timeout_minutes, model. Validate schedule is a valid 5-field cron expression using `cron.ParseStandard()`. Response: `{"data": {...}}` with 201
- `PATCH /api/v1/cron-jobs/:id` — Partial update. Same validation on schedule if provided. Response: `{"data": {...}}`
- `DELETE /api/v1/cron-jobs/:id` — Delete cron job and all executions (CASCADE). Response: 204

**Executions:**

- `GET /api/v1/cron-jobs/:id/executions` — List executions for a cron job, ordered by started_at DESC. Support `limit` (default 20) and `offset` query params for pagination. Response: `{"data": [...], "total": N}`
- `POST /api/v1/cron-jobs/:id/run` — Manually trigger a cron job immediately (regardless of schedule or enabled status). Response: `{"data": {"execution_id": N}}` with 202. Execute async in a goroutine.

**Purge:**

- Extend the existing task output purge endpoint/logic to also purge `cron_executions` older than the retention period. Find where task execution purge is implemented and add a similar DELETE query for cron_executions.

### Route Registration

Register all routes under the existing router setup with auth middleware. Group under `/api/v1/cron-jobs`.

### Claude Executor

Implement the real `executeCronJob()` in `internal/runner/cron.go` (replacing the placeholder):

1. Create a `CronExecution` record with status "running"
2. Build Claude command args:
   ```
   claude -p <prompt> --dangerously-skip-permissions --verbose --output-format stream-json
   ```
   - Add `--model <model>` if the cron job has a model override
   - Working directory: the project's `Path` field
3. Use `SanitizedEnv()` from `internal/claude/env.go` for the environment
4. Set process group (`Setpgid: true`) for reliable kill on timeout (same pattern as task executor)
5. Set timeout via `context.WithTimeout` using `TimeoutMinutes` from the cron job
6. Parse NDJSON output from stdout — collect the full response text from content_block_delta events (same parsing as task executor, see `internal/runner/executor.go` for reference)
7. Extract cost/usage from the result event (cost_usd, input_tokens, output_tokens, duration_ms)
8. On completion:
   - Update CronExecution: status ("success"/"failed"/"timeout"), output, error_message, cost fields, finished_at
   - Update CronJob: last_run_at, last_status
9. On timeout: kill the process group (`syscall.Kill(-pid, syscall.SIGKILL)`), set status to "timeout"
10. On error: capture stderr, set status to "failed", store error in error_message

**Important execution rules:**
- One cron job execution at a time per cron job (the scheduler's `running` map handles this)
- Respect usage thresholds — skip if rate limits near capacity
- Log: `[cron] executing job %d (%s) in %s`, `[cron] job %d completed: %s (%.4f USD, %dms)`

### Error Handling

- 400 for validation errors (invalid cron expression, missing fields)
- 404 for not found
- Follow existing `{"error": "message"}` pattern

## Testing

Write integration tests in `internal/handlers/cron_handler_test.go`:
- List empty, list with data
- Create valid cron job, create with invalid schedule (400), missing fields (400)
- Update schedule, toggle enabled
- Delete existing, delete non-existent (404)
- List executions with pagination
- Manual trigger returns 202
- Purge removes old executions
