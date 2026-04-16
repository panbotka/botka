package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"botka/internal/claude"
	"botka/internal/config"
	"botka/internal/models"
)

const cronTickInterval = 30 * time.Second

// CronScheduler checks cron job schedules on a 30-second tick and triggers
// executions when they are due. It prevents overlapping runs of the same job
// and respects API rate limits via the shared UsageMonitor.
type CronScheduler struct {
	db         *gorm.DB
	cfg        *config.Config
	claudePath string
	usageMon   *UsageMonitor
	stop       chan struct{}
	wg         sync.WaitGroup
	running    map[int64]bool // track running jobs to prevent overlap
	mu         sync.Mutex
}

// NewCronScheduler creates a new CronScheduler. It resolves the Claude CLI
// binary path from the config.
func NewCronScheduler(db *gorm.DB, cfg *config.Config, usageMon *UsageMonitor) (*CronScheduler, error) {
	claudePath, err := exec.LookPath(cfg.ClaudePath)
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found at %q: %w", cfg.ClaudePath, err)
	}
	return &CronScheduler{
		db:         db,
		cfg:        cfg,
		claudePath: claudePath,
		usageMon:   usageMon,
		running:    make(map[int64]bool),
	}, nil
}

// Start begins the cron scheduler loop in a background goroutine.
func (cs *CronScheduler) Start() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.stop != nil {
		return // already running
	}

	cs.stop = make(chan struct{})
	cs.wg.Add(1)
	go cs.loop(cs.stop)
	slog.Info("cron scheduler started")
}

// Stop terminates the scheduler loop and waits for all running jobs to finish.
func (cs *CronScheduler) Stop() {
	cs.mu.Lock()
	if cs.stop != nil {
		close(cs.stop)
		cs.stop = nil
	}
	cs.mu.Unlock()

	cs.wg.Wait()
	slog.Info("cron scheduler stopped")
}

// TriggerJob manually triggers a cron job execution, regardless of schedule or
// enabled status. Returns the execution ID for the caller to track progress.
// The actual Claude execution runs asynchronously in a goroutine.
func (cs *CronScheduler) TriggerJob(jobID int64) (int64, error) {
	var job models.CronJob
	if err := cs.db.Preload("Project").First(&job, jobID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("cron job %d not found", jobID)
		}
		return 0, fmt.Errorf("failed to load cron job: %w", err)
	}

	cs.mu.Lock()
	if cs.running[job.ID] {
		cs.mu.Unlock()
		return 0, fmt.Errorf("job %d is already running", job.ID)
	}
	cs.running[job.ID] = true
	cs.mu.Unlock()

	// Create execution record synchronously so we can return its ID.
	execution := models.CronExecution{
		CronJobID: job.ID,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := cs.db.Create(&execution).Error; err != nil {
		cs.mu.Lock()
		delete(cs.running, job.ID)
		cs.mu.Unlock()
		return 0, fmt.Errorf("failed to create execution: %w", err)
	}

	cs.wg.Add(1)
	go func() {
		defer cs.wg.Done()
		defer func() {
			cs.mu.Lock()
			delete(cs.running, job.ID)
			cs.mu.Unlock()
		}()
		cs.runExecution(&job, &execution)
	}()

	return execution.ID, nil
}

func (cs *CronScheduler) loop(stopCh <-chan struct{}) {
	defer cs.wg.Done()

	ticker := time.NewTicker(cronTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			cs.tick()
		}
	}
}

func (cs *CronScheduler) tick() {
	if limited, reason := cs.usageMon.IsRateLimited(); limited {
		slog.Debug("[cron] rate limited, skipping tick", "reason", reason)
		return
	}

	var jobs []models.CronJob
	if err := cs.db.Preload("Project").Where("enabled = ?", true).Find(&jobs).Error; err != nil {
		slog.Error("[cron] failed to query jobs", "error", err)
		return
	}

	now := time.Now()
	for i := range jobs {
		cs.maybeRun(&jobs[i], now)
	}
}

// maybeRun checks whether a job should fire at the given time and spawns
// its execution if so.
func (cs *CronScheduler) maybeRun(job *models.CronJob, now time.Time) {
	sched, err := cron.ParseStandard(job.Schedule)
	if err != nil {
		slog.Error("[cron] invalid schedule", "job_id", job.ID, "schedule", job.Schedule, "error", err)
		return
	}

	if !cs.shouldRun(job, sched, now) {
		return
	}

	cs.mu.Lock()
	if cs.running[job.ID] {
		cs.mu.Unlock()
		slog.Debug("[cron] job already running, skipping", "job_id", job.ID)
		return
	}
	cs.running[job.ID] = true
	cs.mu.Unlock()

	projectName := ""
	if job.Project != nil {
		projectName = job.Project.Name
	}
	slog.Info("[cron] triggering job",
		"job_id", job.ID, "name", job.Name, "project", projectName)

	cs.wg.Add(1)
	go cs.executeCronJob(job)
}

// shouldRun determines if a job's schedule matches the current time and it
// hasn't already run in this minute.
func (cs *CronScheduler) shouldRun(job *models.CronJob, sched cron.Schedule, now time.Time) bool {
	// Truncate to minute for comparison — cron expressions are minute-granularity.
	nowMinute := now.Truncate(time.Minute)

	// Check if the job already ran in this minute.
	if job.LastRunAt != nil && !job.LastRunAt.Truncate(time.Minute).Before(nowMinute) {
		return false
	}

	// The schedule's Next() returns the first matching time after the given time.
	// If Next(nowMinute - 1s) == nowMinute, the job should fire now.
	next := sched.Next(nowMinute.Add(-time.Second))
	return next.Equal(nowMinute)
}

// executeCronJob creates a CronExecution record and runs the Claude process.
// Called by the scheduler tick path.
func (cs *CronScheduler) executeCronJob(job *models.CronJob) {
	defer cs.wg.Done()
	defer func() {
		cs.mu.Lock()
		delete(cs.running, job.ID)
		cs.mu.Unlock()
	}()

	execution := models.CronExecution{
		CronJobID: job.ID,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := cs.db.Create(&execution).Error; err != nil {
		slog.Error("[cron] failed to create execution record", "job_id", job.ID, "error", err)
		return
	}

	cs.runExecution(job, &execution)
}

// runExecution runs the Claude process for a cron job and updates the execution
// and job records with the results.
func (cs *CronScheduler) runExecution(job *models.CronJob, execution *models.CronExecution) {
	projectPath := ""
	if job.Project != nil {
		projectPath = job.Project.Path
	}

	slog.Info(fmt.Sprintf("[cron] executing job %d (%s) in %s", job.ID, job.Name, projectPath))

	timeout := time.Duration(job.TimeoutMinutes) * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{
		"-p", job.Prompt,
		"--dangerously-skip-permissions", "--verbose",
		"--output-format", "stream-json",
	}
	if job.Model != nil && *job.Model != "" {
		args = append(args, "--model", *job.Model)
	}

	cmd := exec.CommandContext(ctx, cs.claudePath, args...) //nolint:gosec // args are controlled
	cmd.Dir = projectPath
	cmd.Env = claude.SanitizedEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = gracefulStopTimeout

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cs.finishExecution(job, execution, "failed", "", fmt.Sprintf("stdout pipe: %v", err), 0, 0, 0, 0)
		return
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		cs.finishExecution(job, execution, "failed", "", fmt.Sprintf("start claude: %v", err), 0, 0, 0, 0)
		return
	}

	var lastResult *Event
	var lastText string
	parseErr := ParseStream(io.Reader(stdout), func(ev Event) {
		switch ev.Type {
		case EventResult:
			evCopy := ev
			lastResult = &evCopy
		case EventAssistantText:
			lastText = ev.Text
		}
	})

	waitErr := cmd.Wait()
	stderrStr := stderrBuf.String()

	// Classify the outcome.
	if ctx.Err() != nil {
		// Timeout — kill the process group.
		errMsg := "execution timed out"
		if stderrStr != "" {
			errMsg = fmt.Sprintf("execution timed out: %s", truncate(stderrStr, maxErrLen))
		}
		cs.finishExecution(job, execution, "timeout", lastText, errMsg, 0, 0, 0, 0)
		slog.Warn("[cron] job timed out", "job_id", job.ID, "timeout", timeout)
		return
	}

	if parseErr != nil {
		slog.Warn("[cron] stream parse error", "job_id", job.ID, "error", parseErr)
	}

	if lastResult == nil {
		// No result event — process crashed.
		errMsg := truncate(stderrStr, maxErrLen)
		if errMsg == "" {
			errMsg = "claude process exited without result"
		}
		cs.finishExecution(job, execution, "failed", lastText, errMsg, 0, 0, 0, 0)
		return
	}

	var exitErr *exec.ExitError
	exitCode := 0
	if errors.As(waitErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if waitErr != nil {
		cs.finishExecution(job, execution, "failed", lastText,
			fmt.Sprintf("wait for claude: %v", waitErr),
			lastResult.CostUSD, int(lastResult.InputTokens), int(lastResult.OutputTokens), int(lastResult.DurationMs))
		return
	}

	if exitCode != 0 || lastResult.IsError {
		errMsg := truncate(stderrStr, maxErrLen)
		if errMsg == "" {
			errMsg = "claude process exited with error"
		}
		cs.finishExecution(job, execution, "failed", lastText, errMsg,
			lastResult.CostUSD, int(lastResult.InputTokens), int(lastResult.OutputTokens), int(lastResult.DurationMs))
		return
	}

	// Success.
	cs.finishExecution(job, execution, "success", lastText, "",
		lastResult.CostUSD, int(lastResult.InputTokens), int(lastResult.OutputTokens), int(lastResult.DurationMs))

	slog.Info(fmt.Sprintf("[cron] job %d completed: success (%.4f USD, %dms)",
		job.ID, lastResult.CostUSD, lastResult.DurationMs))
}

// finishExecution updates the CronExecution and CronJob records with the final
// status and results.
func (cs *CronScheduler) finishExecution(
	job *models.CronJob, execution *models.CronExecution,
	status, output, errMsg string,
	costUSD float64, inputTokens, outputTokens, durationMs int,
) {
	now := time.Now()
	updates := map[string]interface{}{
		"status":      status,
		"finished_at": now,
		"cost_usd":    costUSD,
		"duration_ms": durationMs,
	}
	if output != "" {
		updates["output"] = output
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	if inputTokens > 0 {
		updates["input_tokens"] = inputTokens
	}
	if outputTokens > 0 {
		updates["output_tokens"] = outputTokens
	}

	if err := cs.db.Model(execution).Updates(updates).Error; err != nil {
		slog.Error("[cron] failed to update execution", "job_id", job.ID, "error", err)
	}

	// Update denormalized fields on the job.
	cs.db.Model(job).Updates(map[string]interface{}{
		"last_run_at": now,
		"last_status": status,
	})
}

// PurgeCronExecutions deletes cron execution records older than the given
// retention period. Returns the number of deleted rows.
func PurgeCronExecutions(db *gorm.DB, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	result := db.Where("started_at < ?", cutoff).Delete(&models.CronExecution{})
	return result.RowsAffected, result.Error
}
