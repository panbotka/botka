// Package runner implements the task scheduler and batch Claude Code executor.
//
// This is one of two Claude Code spawn paths (the other is internal/claude for chat).
// The task executor uses process groups (Setpgid) for reliable timeout/kill, has retry
// logic with backoff for API errors, runs optional verification commands, and creates
// PRs on feature branches. Each execution is standalone with no session continuity,
// because tasks are independent units of work.
//
// The scheduler loop polls every 5 seconds, picks the highest-priority queued task
// (excluding projects with a running task), and launches it in a goroutine. It enforces
// one task per project to prevent git conflicts.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"botka/internal/config"
	"botka/internal/models"
)

const (
	tickInterval   = 5 * time.Second
	bufferCapacity = 1 << 20 // 1 MB
)

// activeTask tracks a currently executing task.
type activeTask struct {
	task      *models.Task
	execution *models.TaskExecution
	cancel    context.CancelFunc
}

// ActiveTaskInfo provides a read-only summary of a running task for the API.
type ActiveTaskInfo struct {
	TaskID      uuid.UUID `json:"task_id"`
	TaskTitle   string    `json:"task_title"`
	ProjectName string    `json:"project_name"`
	StartedAt   time.Time `json:"started_at"`
}

// Status reports the current state of the scheduler.
type Status struct {
	State          models.RunnerStateType `json:"state"`
	ActiveTasks    []ActiveTaskInfo       `json:"active_tasks"`
	MaxWorkers     int                    `json:"max_workers"`
	Draining       bool                   `json:"draining"`
	Usage          *UsageInfo             `json:"usage,omitempty"`
	TaskLimit      int                    `json:"task_limit"`
	CompletedCount int                    `json:"completed_count"`
}

// Runner manages the scheduling loop and parallel task execution.
// All public methods are safe for concurrent use.
type Runner struct {
	db             *gorm.DB
	config         *config.Config
	mu             sync.RWMutex
	state          models.RunnerStateType
	taskLimit      int // 0 = unlimited
	completedCount int
	maxWorkers     int
	executors      map[uuid.UUID]*activeTask // key: project ID
	buffers        map[uuid.UUID]*Buffer     // key: task ID
	usageMon       *UsageMonitor
	executor       *Executor
	stopCh         chan struct{}
	wg             sync.WaitGroup
	retryNotBefore map[uuid.UUID]time.Time // key: task ID
}

// NewRunner creates a new Runner instance and loads persisted state from the database.
func NewRunner(db *gorm.DB, cfg *config.Config, usageMon *UsageMonitor) (*Runner, error) {
	exec, err := NewExecutor(cfg.ClaudePath)
	if err != nil {
		return nil, err
	}
	r := &Runner{
		db:             db,
		config:         cfg,
		state:          models.StatePaused,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		usageMon:       usageMon,
		executor:       exec,
		retryNotBefore: make(map[uuid.UUID]time.Time),
	}
	r.state = r.loadState()
	r.maxWorkers = cfg.MaxWorkers // default from env
	r.loadMaxWorkersFromDB()
	return r, nil
}

// loadMaxWorkersFromDB reads the max_workers setting from the app_settings table.
func (r *Runner) loadMaxWorkersFromDB() {
	if r.db == nil {
		return
	}
	var value string
	err := r.db.Table("app_settings").Where("key = ?", "max_workers").Pluck("value", &value).Error
	if err != nil || value == "" {
		return
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return
	}
	r.maxWorkers = n
}

// SetMaxWorkers updates the maximum number of concurrent task workers.
// Thread-safe: acquires the mutex before updating.
func (r *Runner) SetMaxWorkers(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxWorkers = n
	slog.Info("max workers updated", "max_workers", n)
}

// RestoreState checks the persisted state and starts the scheduler loop
// if it was previously running. Call this once after NewRunner on server startup.
func (r *Runner) RestoreState() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == models.StateRunning {
		r.startLocked()
	}
}

// Start begins the scheduler loop in unlimited mode. Idempotent: no-op if already running.
// On first start, requeues any tasks orphaned in "running" status from a previous instance.
func (r *Runner) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.taskLimit = 0
	r.completedCount = 0
	r.startLocked()
}

// StartN begins the scheduler loop and auto-stops after n tasks complete.
func (r *Runner) StartN(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.taskLimit = n
	r.completedCount = 0
	r.startLocked()
}

// startLocked sets the runner to running and starts the loop. Must be called with r.mu held.
func (r *Runner) startLocked() {
	r.state = models.StateRunning
	r.persistState()
	if r.stopCh == nil {
		r.recoverOrphanedTasks()
		r.stopCh = make(chan struct{})
		r.wg.Add(1)
		go r.loop(r.stopCh)
	}
}

// recoverOrphanedTasks requeues any tasks stuck in "running" status from a previous instance.
// Must be called before the scheduler loop starts.
func (r *Runner) recoverOrphanedTasks() {
	if r.db == nil {
		return
	}
	result := r.db.Model(&models.Task{}).
		Where("status = ?", models.TaskStatusRunning).
		Updates(map[string]interface{}{
			"status":         models.TaskStatusQueued,
			"failure_reason": "recovered: process restarted while task was running",
		})
	if result.Error != nil {
		slog.Error("failed to recover orphaned tasks", "error", result.Error)
		return
	}
	if result.RowsAffected > 0 {
		slog.Info("recovered orphaned tasks", "count", result.RowsAffected)
	}
}

// Shutdown stops the scheduler loop and waits for all running executors to finish.
// Used for graceful server shutdown — does not change persisted state.
func (r *Runner) Shutdown() {
	r.mu.Lock()
	if r.stopCh != nil {
		close(r.stopCh)
		r.stopCh = nil
	}
	r.mu.Unlock()

	r.wg.Wait()
}

// Pause stops picking new tasks but lets running tasks complete.
// State is persisted and survives restarts.
func (r *Runner) Pause() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = models.StatePaused
	r.persistState()
}

// HardStop immediately kills all running tasks and stops the scheduler.
// State is persisted and survives restarts.
func (r *Runner) HardStop() {
	r.mu.Lock()
	r.state = models.StateStopped
	r.taskLimit = 0
	r.completedCount = 0
	r.persistState()
	// Cancel all running task contexts (sends SIGTERM to claude processes).
	for _, at := range r.executors {
		slog.Info("killing task", "task_id", at.task.ID)
		at.cancel()
	}
	r.mu.Unlock()
}

// Resume resumes picking tasks. Alias for Start.
func (r *Runner) Resume() {
	r.Start()
}

// GetStatus returns the current state of the runner for the API.
func (r *Runner) GetStatus() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tasks := make([]ActiveTaskInfo, 0, len(r.executors))
	for _, at := range r.executors {
		tasks = append(tasks, ActiveTaskInfo{
			TaskID:      at.task.ID,
			TaskTitle:   at.task.Title,
			ProjectName: at.task.Project.Name,
			StartedAt:   at.execution.StartedAt,
		})
	}

	usage := r.usageMon.CurrentUsage()
	return Status{
		State:          r.state,
		ActiveTasks:    tasks,
		MaxWorkers:     r.maxWorkers,
		Draining:       len(r.executors) > r.maxWorkers,
		Usage:          &usage,
		TaskLimit:      r.taskLimit,
		CompletedCount: r.completedCount,
	}
}

// RefreshUsage triggers an immediate usage poll and returns the updated info.
func (r *Runner) RefreshUsage() UsageInfo {
	r.usageMon.Poll()
	return r.usageMon.CurrentUsage()
}

// GetBuffer returns the output buffer for a running task, or nil if not found.
func (r *Runner) GetBuffer(taskID uuid.UUID) *Buffer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.buffers[taskID]
}

func (r *Runner) loop(stopCh <-chan struct{}) {
	defer r.wg.Done()

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	slog.Info("scheduler loop started")

	for {
		select {
		case <-stopCh:
			slog.Info("scheduler loop stopped")
			return
		case <-ticker.C:
			r.tick()
		}
	}
}

func (r *Runner) tick() {
	eligible, activeProjectIDs, blockedTaskIDs := r.collectTickState()
	if !eligible {
		return
	}

	if limited, reason := r.usageMon.IsRateLimited(); limited {
		slog.Info("rate limited, waiting", "reason", reason, "resets_at", r.usageMon.ResetsAt())
		return
	}

	task, execution, err := r.pickNextTask(activeProjectIDs, blockedTaskIDs)
	if err != nil {
		slog.Error("scheduler: failed to pick task", "error", err)
		return
	}
	if task == nil {
		return
	}

	r.launchTask(task, execution)
}

// collectTickState gathers state needed for a scheduler tick under a single lock.
func (r *Runner) collectTickState() (eligible bool, activeProjectIDs, blockedTaskIDs []uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != models.StateRunning || len(r.executors) >= r.maxWorkers {
		return false, nil, nil
	}

	// Auto-stop: task limit reached.
	if r.taskLimit > 0 && r.completedCount >= r.taskLimit {
		if len(r.executors) == 0 {
			slog.Info("task limit reached, stopping runner",
				"limit", r.taskLimit, "completed", r.completedCount)
			r.state = models.StateStopped
			r.persistState()
		}
		return false, nil, nil
	}

	activeProjectIDs = make([]uuid.UUID, 0, len(r.executors))
	for projectID := range r.executors {
		activeProjectIDs = append(activeProjectIDs, projectID)
	}

	now := time.Now()
	for taskID, notBefore := range r.retryNotBefore {
		if now.Before(notBefore) {
			blockedTaskIDs = append(blockedTaskIDs, taskID)
		} else {
			delete(r.retryNotBefore, taskID)
		}
	}

	if len(activeProjectIDs) > 0 {
		slog.Debug("scheduler: excluding active projects", "project_ids", activeProjectIDs)
	}

	return true, activeProjectIDs, blockedTaskIDs
}

func (r *Runner) pickNextTask(
	activeProjectIDs, blockedTaskIDs []uuid.UUID,
) (*models.Task, *models.TaskExecution, error) {
	tx := r.db.Begin()
	if tx.Error != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", tx.Error)
	}
	defer tx.Rollback() //nolint:errcheck // safe no-op after commit

	var task models.Task
	err := r.buildPickQuery(tx, activeProjectIDs, blockedTaskIDs).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("query task: %w", err)
	}

	slog.Info("scheduler: picked task",
		"task_id", task.ID, "project_id", task.ProjectID, "title", task.Title)

	t, exec, err := r.claimTask(tx, &task)
	if err != nil {
		if isUniqueViolation(err) {
			slog.Info("scheduler: another process already claimed a task for this project, skipping",
				"task_id", task.ID, "project_id", task.ProjectID)
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return t, exec, nil
}

// buildPickQuery constructs the GORM query for finding the next eligible task.
// Uses SELECT FOR UPDATE SKIP LOCKED for safe concurrent access without blocking.
// Excludes projects that already have a running task (one task per project) to
// prevent git conflicts when multiple tasks modify the same repository.
// Uses both in-memory exclusion (activeProjectIDs) and a database-level subquery
// to guarantee one-task-per-project even if in-memory state is inconsistent.
func (r *Runner) buildPickQuery(
	tx *gorm.DB, activeProjectIDs, blockedTaskIDs []uuid.UUID,
) *gorm.DB {
	query := tx.
		Where("status = ?", models.TaskStatusQueued).
		Where("NOT EXISTS (SELECT 1 FROM tasks t2 WHERE t2.project_id = tasks.project_id AND t2.status = ?)", models.TaskStatusRunning).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Order("priority DESC, created_at ASC")

	if len(activeProjectIDs) > 0 {
		query = query.Where("project_id NOT IN ?", activeProjectIDs)
	}
	if len(blockedTaskIDs) > 0 {
		query = query.Where("id NOT IN ?", blockedTaskIDs)
	}

	return query
}

// claimTask transitions a task to running, loads its project, creates an execution record.
func (r *Runner) claimTask(
	tx *gorm.DB, task *models.Task,
) (*models.Task, *models.TaskExecution, error) {
	if err := tx.First(&task.Project, task.ProjectID).Error; err != nil {
		return nil, nil, fmt.Errorf("load project: %w", err)
	}

	updates := map[string]interface{}{"status": models.TaskStatusRunning}
	if task.StartedAt == nil {
		now := time.Now()
		updates["started_at"] = now
		task.StartedAt = &now
	}
	if err := tx.Model(task).Updates(updates).Error; err != nil {
		return nil, nil, fmt.Errorf("update task status: %w", err)
	}
	task.Status = models.TaskStatusRunning

	execution := models.TaskExecution{
		TaskID:    task.ID,
		Attempt:   task.RetryCount + 1,
		StartedAt: time.Now(),
	}
	if err := tx.Create(&execution).Error; err != nil {
		return nil, nil, fmt.Errorf("create execution: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, nil, fmt.Errorf("commit: %w", err)
	}

	return task, &execution, nil
}

func (r *Runner) launchTask(task *models.Task, execution *models.TaskExecution) {
	r.mu.Lock()
	if existing, ok := r.executors[task.ProjectID]; ok {
		r.mu.Unlock()
		slog.Error("scheduler: refusing to launch second task for same project",
			"project_id", task.ProjectID,
			"existing_task", existing.task.ID,
			"new_task", task.ID)
		// Requeue the task so it can be picked up later.
		r.db.Model(task).Update("status", models.TaskStatusQueued)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	buf := NewBuffer(bufferCapacity)

	r.executors[task.ProjectID] = &activeTask{
		task:      task,
		execution: execution,
		cancel:    cancel,
	}
	r.buffers[task.ID] = buf
	r.mu.Unlock()

	slog.Info("launching task",
		"task_id", task.ID, "project", task.Project.Name, "title", task.Title)

	r.wg.Add(1)
	go r.executeTask(ctx, task, execution, buf)
}

func (r *Runner) executeTask(
	ctx context.Context, task *models.Task, exec *models.TaskExecution, buf *Buffer,
) {
	defer r.wg.Done()
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("executor panic", "task_id", task.ID, "panic", rec)
			r.finishTask(task, exec, buf, &ExecutionResult{
				Status:       models.TaskStatusFailed,
				ErrorMessage: fmt.Sprintf("executor panic: %v", rec),
			})
		}
	}()

	result, err := r.executor.Execute(ctx, task, &task.Project, buf)
	if err != nil {
		slog.Error("executor error", "task_id", task.ID, "error", err)
		result = &ExecutionResult{
			Status:       models.TaskStatusFailed,
			ErrorMessage: err.Error(),
		}
	}

	r.finishTask(task, exec, buf, result)
}

func (r *Runner) finishTask(
	task *models.Task, exec *models.TaskExecution, buf *Buffer, result *ExecutionResult,
) {
	rawOutput := string(buf.ReadAll())
	r.updateExecution(exec, result, rawOutput)
	r.applyResult(task, result)

	buf.Close()
	r.mu.Lock()
	delete(r.executors, task.ProjectID)
	delete(r.buffers, task.ID)
	r.mu.Unlock()
}

func (r *Runner) updateExecution(exec *models.TaskExecution, result *ExecutionResult, rawOutput string) {
	now := time.Now()
	r.db.Model(exec).Updates(map[string]interface{}{
		"finished_at":   now,
		"cost_usd":      result.CostUSD,
		"duration_ms":   result.DurationMs,
		"summary":       nilString(result.Summary),
		"error_message": nilString(result.ErrorMessage),
		"raw_output":    nilString(rawOutput),
	})
}

func (r *Runner) applyResult(task *models.Task, result *ExecutionResult) {
	switch {
	case result.RetryAfter > 0:
		r.requeueTask(task, result.ErrorMessage)
		r.mu.Lock()
		r.retryNotBefore[task.ID] = time.Now().Add(result.RetryAfter)
		r.mu.Unlock()
		slog.Info("task retry scheduled",
			"task_id", task.ID, "retry_after", result.RetryAfter)

	case result.ShouldRetry:
		r.requeueTask(task, result.ErrorMessage)
		slog.Info("task queued for retry", "task_id", task.ID)

	default:
		r.finalizeTask(task, result)
	}
}

func (r *Runner) requeueTask(task *models.Task, errMsg string) {
	r.db.Model(task).Updates(map[string]interface{}{
		"status":         models.TaskStatusQueued,
		"retry_count":    gorm.Expr("retry_count + 1"),
		"failure_reason": errMsg,
	})
}

func (r *Runner) finalizeTask(task *models.Task, result *ExecutionResult) {
	now := time.Now()
	updates := map[string]interface{}{
		"status":       result.Status,
		"completed_at": now,
	}
	if result.ErrorMessage != "" {
		updates["failure_reason"] = result.ErrorMessage
	}
	r.db.Model(task).Updates(updates)
	slog.Info("task finished", "task_id", task.ID, "status", result.Status)

	r.mu.Lock()
	if r.taskLimit > 0 {
		r.completedCount++
		r.persistState()
		slog.Info("task limit progress",
			"completed", r.completedCount, "limit", r.taskLimit)
	}
	r.mu.Unlock()
}

func nilString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isUniqueViolation reports whether err is a PostgreSQL unique_violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// persistState writes the runner state to the database. Called with r.mu held.
func (r *Runner) persistState() {
	if r.db == nil {
		return
	}
	if err := r.db.Exec(
		`UPDATE runner_state SET state = ?, task_limit = ?, completed_count = ?, updated_at = NOW() WHERE id = 1`,
		string(r.state), r.taskLimit, r.completedCount,
	).Error; err != nil {
		slog.Error("failed to persist runner state", "state", r.state, "error", err)
	}
}

// loadState reads the persisted runner state from the database and populates
// taskLimit and completedCount on the runner.
func (r *Runner) loadState() models.RunnerStateType {
	if r.db == nil {
		return models.StatePaused
	}
	var row struct {
		State          string
		TaskLimit      int
		CompletedCount int
	}
	if err := r.db.Raw(
		"SELECT state, task_limit, completed_count FROM runner_state WHERE id = 1",
	).Scan(&row).Error; err != nil {
		slog.Warn("failed to load runner state, defaulting to paused", "error", err)
		return models.StatePaused
	}
	r.taskLimit = row.TaskLimit
	r.completedCount = row.CompletedCount
	switch models.RunnerStateType(row.State) {
	case models.StateRunning, models.StatePaused, models.StateStopped:
		return models.RunnerStateType(row.State)
	default:
		return models.StatePaused
	}
}
