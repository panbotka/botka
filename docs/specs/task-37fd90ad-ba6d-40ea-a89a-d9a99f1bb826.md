# Cron Jobs — Database Schema, Models & Scheduler

Add support for scheduled AI prompts that run on a cron schedule. This is the foundation: database tables, GORM models, and the cron scheduler goroutine that triggers executions.

## Context

Botka already has a task runner (`internal/runner/`) that executes one-shot Claude Code sessions for queued tasks. Cron jobs are a separate system — they run a prompt on a cron schedule in a project context, non-interactive, fire-and-forget. No retry logic, no branch strategy, no PR creation. Just run the prompt with `claude -p` and record the result.

## Requirements

### Migration (next sequential number after existing migrations)

Create two tables:

**cron_jobs:**
- `id` BIGSERIAL PRIMARY KEY
- `name` TEXT NOT NULL — display name (e.g. "Check outdated deps", "Weekly security scan")
- `schedule` TEXT NOT NULL — standard 5-field crontab expression (e.g. "0 9 * * 1" for Monday 9am)
- `prompt` TEXT NOT NULL — the prompt to send to Claude via `-p` flag
- `project_id` UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE — which project context to run in
- `enabled` BOOLEAN NOT NULL DEFAULT true — can be paused without deleting
- `timeout_minutes` INTEGER NOT NULL DEFAULT 30 — max execution time
- `model` TEXT — optional model override (NULL = use default from config)
- `last_run_at` TIMESTAMPTZ — when it last ran (denormalized for quick display)
- `last_status` TEXT — last execution status (denormalized: "success", "failed", "timeout")
- `created_at` TIMESTAMPTZ NOT NULL DEFAULT NOW()
- `updated_at` TIMESTAMPTZ NOT NULL DEFAULT NOW()

**cron_executions:**
- `id` BIGSERIAL PRIMARY KEY
- `cron_job_id` BIGINT NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE
- `status` TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failed', 'timeout'))
- `output` TEXT — full Claude response text
- `error_message` TEXT — error details if failed/timeout
- `cost_usd` DOUBLE PRECISION — API cost
- `input_tokens` INTEGER
- `output_tokens` INTEGER
- `duration_ms` INTEGER
- `started_at` TIMESTAMPTZ NOT NULL DEFAULT NOW()
- `finished_at` TIMESTAMPTZ

Add index: `CREATE INDEX idx_cron_executions_job_id ON cron_executions(cron_job_id, started_at DESC)` for efficient history queries.

Down migration drops both tables.

### GORM Models

Create `internal/models/cron_job.go`:

```go
type CronJob struct {
    ID             int64      `json:"id" gorm:"primaryKey"`
    Name           string     `json:"name" gorm:"not null"`
    Schedule       string     `json:"schedule" gorm:"not null"`
    Prompt         string     `json:"prompt" gorm:"not null"`
    ProjectID      uuid.UUID  `json:"project_id" gorm:"type:uuid;not null"`
    Project        *Project   `json:"project,omitempty"`
    Enabled        bool       `json:"enabled" gorm:"not null;default:true"`
    TimeoutMinutes int        `json:"timeout_minutes" gorm:"not null;default:30"`
    Model          *string    `json:"model"`
    LastRunAt      *time.Time `json:"last_run_at"`
    LastStatus     *string    `json:"last_status"`
    CreatedAt      time.Time  `json:"created_at"`
    UpdatedAt      time.Time  `json:"updated_at"`
}

type CronExecution struct {
    ID           int64      `json:"id" gorm:"primaryKey"`
    CronJobID    int64      `json:"cron_job_id" gorm:"not null"`
    CronJob      *CronJob   `json:"cron_job,omitempty"`
    Status       string     `json:"status" gorm:"not null;default:running"`
    Output       *string    `json:"output"`
    ErrorMessage *string    `json:"error_message"`
    CostUSD      float64    `json:"cost_usd"`
    InputTokens  int        `json:"input_tokens"`
    OutputTokens int        `json:"output_tokens"`
    DurationMs   int        `json:"duration_ms"`
    StartedAt    time.Time  `json:"started_at"`
    FinishedAt   *time.Time `json:"finished_at"`
}
```

Add `TableName()` methods following existing patterns.

### Cron Scheduler

Create `internal/runner/cron.go` with a `CronScheduler` struct:

```go
type CronScheduler struct {
    db        *gorm.DB
    cfg       *config.Config
    stop      chan struct{}
    running   map[int64]bool // track running jobs to prevent overlap
    mu        sync.Mutex
}
```

**Scheduler loop:**
1. Start a goroutine that ticks every 30 seconds
2. On each tick, query all `cron_jobs` where `enabled = true`
3. For each job, parse the `schedule` field and check if it should run now (current time matches cron expression, and it hasn't already run in this minute — compare with `last_run_at`)
4. If a job should run, check it's not already running (via `running` map), then spawn execution in a goroutine
5. Use a cron parsing library — `github.com/robfig/cron/v3` is the standard Go choice. Use `cron.ParseStandard()` for 5-field format.

**Integration:**
- Start the scheduler from `cmd/server/main.go` alongside the existing task runner
- Add a `Stop()` method for clean shutdown
- The scheduler should respect the existing usage thresholds (`USAGE_THRESHOLD_5H`, `USAGE_THRESHOLD_7D`) — skip execution if rate limits are near capacity (use the same check as the task runner)

**Purge support:**
- The existing task output purge mechanism should also purge `cron_executions`. Add a similar purge query: delete executions older than the configured retention period. Look at how task execution purge works and follow the same pattern.

## Testing

- Model tests: TableName, default values
- Migration applies and rolls back
- Cron schedule parsing: valid expressions, invalid expressions
- Scheduler tick logic: job should run, job already ran this minute (skip), job disabled (skip), job already running (skip)
- Purge deletes old executions

## Notes

- Do NOT implement the actual Claude execution in this task — that's the next task. Just create a placeholder `executeCronJob()` method that creates a CronExecution record with status "success" and a dummy output.
- The scheduler should log: `[cron] triggering job %d (%s) for project %s`
- Add `go get github.com/robfig/cron/v3` to go.mod
