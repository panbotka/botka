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

	"botka/internal/box"
	"botka/internal/claude"
	"botka/internal/config"
	"botka/internal/models"
)

const (
	tickInterval   = 5 * time.Second
	bufferCapacity = 1 << 20 // 1 MB
	// Grace period before a DB-only running task is considered orphaned.
	// Covers the window between claimTask (DB commit) and launchTask (executor map insert).
	orphanGracePeriod = 30 * time.Second
)

// Force-run outcome sentinels, mapped to HTTP status codes by the handler.
var (
	// ErrRunnerStopped is returned when force-run is attempted while the runner
	// is hard-stopped.
	ErrRunnerStopped = errors.New("runner is stopped")
	// ErrTaskNotQueued is returned when the target task is not in the queued
	// state.
	ErrTaskNotQueued = errors.New("task is not queued")
	// ErrWorkersBusy is returned when all worker slots are occupied.
	ErrWorkersBusy = errors.New("all workers are busy")
	// ErrProjectBusy is returned when the pre-flight check finds the task's
	// project already has a running task (an in-memory executor is present).
	ErrProjectBusy = errors.New("another task on this project is already running")
	// ErrLaunchRace is returned when the task could not be claimed or
	// launched because of a race between the pre-flight check and the actual
	// claim/launch step: the task stopped being eligible (its status changed
	// underneath us, or a concurrent transaction claimed it first) or a
	// worker/project slot was taken in the gap between the pre-flight check
	// and launchTask. Distinct from ErrProjectBusy, which means the
	// pre-flight itself already found the project busy in memory — this one
	// means the pre-flight passed but reality changed before the claim or
	// launch completed. The caller should treat it as transient and may
	// retry.
	ErrLaunchRace = errors.New("could not launch the task right now; try again")
)

// activeTask tracks a currently executing task.
type activeTask struct {
	task      *models.Task
	execution *models.TaskExecution
	cancel    context.CancelFunc
}

// ActiveTaskInfo provides a read-only summary of a running task for the API.
//
// Orphaned is true when the task is marked 'running' in the database but is
// not tracked by any in-memory executor. This happens after a process restart
// if the previous instance's Claude Code subprocess outlived the Botka
// process, or if RestoreState has not yet requeued the task. The dashboard
// renders these distinctly so operators can see that the runner's in-memory
// view disagrees with the database.
type ActiveTaskInfo struct {
	TaskID      uuid.UUID `json:"task_id"`
	TaskTitle   string    `json:"task_title"`
	ProjectName string    `json:"project_name"`
	StartedAt   time.Time `json:"started_at"`
	Orphaned    bool      `json:"orphaned,omitempty"`
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
	// PausedUntil is when the rate-limit gate clears. Null when the gate is
	// not active. ISO8601 in JSON.
	PausedUntil *time.Time `json:"paused_until"`
	// PauseReason is a human-readable string describing why the gate is set.
	// Null when the gate is not active.
	PauseReason *string `json:"pause_reason"`
	// PauseSource identifies what tripped the gate ("rate_limit" today;
	// future-proof for additional sources). Null when not paused.
	PauseSource *string `json:"pause_source"`
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
	TaskEvents     *TaskEventHub
	pingFn         func() error              // overridable for testing; nil uses defaultPing
	activityFn     func() (time.Time, error) // overridable for testing; nil queries the database
	resetsAtFn     func() time.Time          // overridable for testing; nil reads from usageMon
	pushNotifier   PushNotifier              // optional; nil disables push triggers
	rateLimitGate  *RateLimitGate
	// launchFn, when non-nil, replaces launchTask. Test-only seam (like pingFn /
	// activityFn / resetsAtFn) so force-run tests can assert launch behavior
	// without spawning a real Claude subprocess.
	launchFn func(task *models.Task, execution *models.TaskExecution) bool
}

// NewRunner creates a new Runner instance and loads persisted state from the database.
// boxWaker may be nil, in which case remote-path projects fail fast when executed.
func NewRunner(db *gorm.DB, cfg *config.Config, usageMon *UsageMonitor, boxWaker *box.Waker) (*Runner, error) {
	sshTarget := ""
	if boxWaker != nil {
		sshTarget = boxWaker.SSHTarget()
	}
	exec, err := NewExecutor(cfg.ClaudePath, boxWaker, sshTarget, cfg.TaskTimeout)
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
		TaskEvents:     NewTaskEventHub(),
		rateLimitGate:  NewRateLimitGate(db),
	}
	r.executor.onPhase = r.recordPhase
	r.state = r.loadState()
	r.maxWorkers = cfg.MaxWorkers // default from env
	r.loadMaxWorkersFromDB()
	return r, nil
}

// recordPhase persists the executor phase a running task has just entered and
// broadcasts it on the task event stream, so an open detail page updates without
// refetching on a timer.
//
// The write is guarded on status = 'running': a task that was killed or finalized
// concurrently keeps its cleared phase rather than resurrecting a stale one.
// Recording is best-effort and never fails a task — a database error is logged
// and execution continues.
func (r *Runner) recordPhase(task *models.Task, phase models.RunPhase) {
	if r.db == nil || !phase.IsValid() {
		return
	}
	res := r.db.Model(&models.Task{}).
		Where("id = ? AND status = ?", task.ID, models.TaskStatusRunning).
		Update("run_phase", phase)
	if res.Error != nil {
		slog.Warn("failed to record task run phase",
			"task_id", task.ID, "phase", phase, "error", res.Error)
		return
	}
	if res.RowsAffected == 0 {
		return
	}

	p := phase
	task.RunPhase = &p
	if r.TaskEvents != nil {
		r.TaskEvents.Publish(TaskEvent{
			TaskID:    task.ID,
			Status:    models.TaskStatusRunning,
			ProjectID: task.ProjectID,
			RunPhase:  &p,
		})
	}
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
	if err != nil || n < 0 {
		return
	}
	r.maxWorkers = n
}

// SetPushNotifier installs (or replaces) the push notifier used to fire Web
// Push notifications on terminal task transitions. Pass nil to disable.
// Wired at startup; not thread-safe with itself but safe to call before the
// scheduler loop starts.
func (r *Runner) SetPushNotifier(n PushNotifier) {
	r.pushNotifier = n
}

// SetMaxWorkers updates the maximum number of concurrent task workers.
// Thread-safe: acquires the mutex before updating.
func (r *Runner) SetMaxWorkers(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxWorkers = n
	slog.Info("max workers updated", "max_workers", n)
}

// RestoreState recovers orphaned tasks from a previous crashed process and
// starts the scheduler loop if it was previously running.
// Call this once after NewRunner on server startup.
func (r *Runner) RestoreState() {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Always recover orphaned tasks on startup, regardless of runner state.
	// Without this, tasks stuck in "running" from a crashed process are never
	// cleaned up when the runner state is "stopped" or "paused", causing the
	// dashboard to show running tasks while the runner shows as stopped.
	r.recoverOrphanedTasks()
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
		if r.config != nil && r.config.KeepaliveEnabled {
			r.wg.Add(1)
			go r.keepaliveLoop(r.stopCh)
		}
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
			"run_phase":      nil,
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

// KillTask terminates a single running task by its task ID.
// It cancels the task's context, which sends SIGTERM to the process group.
// The existing finalization flow handles git revert and status updates.
func (r *Runner) KillTask(taskID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, at := range r.executors {
		if at.task.ID == taskID {
			slog.Info("killing task", "task_id", taskID)
			at.cancel()
			return nil
		}
	}
	return fmt.Errorf("task %s is not currently running", taskID)
}

// Resume resumes picking tasks. Alias for Start.
func (r *Runner) Resume() {
	r.Start()
}

// GetStatus returns the current state of the runner for the API.
//
// The active task list is the union of the in-memory executors and any tasks
// marked 'running' in the database that are not tracked in memory. The latter
// are returned with Orphaned=true so the dashboard never shows "Stopped 0/N
// active" while the database still has running tasks (e.g. because a prior
// process exited before its Claude Code subprocess finished).
func (r *Runner) GetStatus() Status {
	r.mu.RLock()
	tasks := make([]ActiveTaskInfo, 0, len(r.executors))
	inMem := make(map[uuid.UUID]struct{}, len(r.executors))
	for _, at := range r.executors {
		tasks = append(tasks, ActiveTaskInfo{
			TaskID:      at.task.ID,
			TaskTitle:   at.task.Title,
			ProjectName: at.task.Project.Name,
			StartedAt:   at.execution.StartedAt,
		})
		inMem[at.task.ID] = struct{}{}
	}
	inMemCount := len(r.executors)
	state := r.state
	maxWorkers := r.maxWorkers
	taskLimit := r.taskLimit
	completedCount := r.completedCount
	r.mu.RUnlock()

	tasks = append(tasks, r.loadOrphanedRunningTasks(inMem)...)

	usage := r.usageMon.CurrentUsage()
	status := Status{
		State:          state,
		ActiveTasks:    tasks,
		MaxWorkers:     maxWorkers,
		Draining:       inMemCount > maxWorkers,
		Usage:          &usage,
		TaskLimit:      taskLimit,
		CompletedCount: completedCount,
	}
	if r.rateLimitGate != nil {
		if active, until, reason, source, _ := r.rateLimitGate.Snapshot(); active {
			t := until
			s := string(source)
			rs := reason
			status.PausedUntil = &t
			status.PauseReason = &rs
			status.PauseSource = &s
		}
	}
	return status
}

// RateLimitGate returns the runner's rate-limit gate so handlers and other
// callers can read state or trigger a manual clear. May return nil in tests
// that construct a Runner without one.
func (r *Runner) RateLimitGate() *RateLimitGate {
	return r.rateLimitGate
}

// SetRateLimitGate replaces the runner's gate. Intended for tests; production
// code uses the gate created in NewRunner.
func (r *Runner) SetRateLimitGate(g *RateLimitGate) {
	r.rateLimitGate = g
}

// NewRunnerForTest builds a minimally-wired runner for HTTP-level tests in
// other packages. It deliberately avoids constructing an Executor — tests that
// only exercise GetStatus / pause endpoints don't need one. The returned
// runner is in the StatePaused state; the caller can flip state directly via
// Pause / Resume / HardStop if needed.
func NewRunnerForTest(db *gorm.DB, usageMon *UsageMonitor, gate *RateLimitGate) *Runner {
	return &Runner{
		db:             db,
		state:          models.StatePaused,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		usageMon:       usageMon,
		retryNotBefore: make(map[uuid.UUID]time.Time),
		TaskEvents:     NewTaskEventHub(),
		rateLimitGate:  gate,
		maxWorkers:     2,
	}
}

// loadOrphanedRunningTasks queries the database for tasks with status 'running'
// whose IDs are not in inMem and returns them as ActiveTaskInfo entries with
// Orphaned=true. Tasks that transitioned to running within orphanGracePeriod
// are excluded to avoid false positives during the window between claimTask
// (DB commit) and launchTask (executor map insertion). Errors are logged but
// do not fail the status response.
func (r *Runner) loadOrphanedRunningTasks(inMem map[uuid.UUID]struct{}) []ActiveTaskInfo {
	if r.db == nil {
		return nil
	}
	var dbRunning []models.Task
	if err := r.db.Preload("Project").
		Where("status = ?", models.TaskStatusRunning).
		Find(&dbRunning).Error; err != nil {
		slog.Warn("failed to query running tasks for status", "error", err)
		return nil
	}
	now := time.Now()
	orphans := make([]ActiveTaskInfo, 0)
	for i := range dbRunning {
		t := &dbRunning[i]
		if _, ok := inMem[t.ID]; ok {
			continue
		}
		started := t.UpdatedAt
		if t.StartedAt != nil {
			started = *t.StartedAt
		}
		if now.Sub(started) < orphanGracePeriod {
			continue
		}
		orphans = append(orphans, ActiveTaskInfo{
			TaskID:      t.ID,
			TaskTitle:   t.Title,
			ProjectName: t.Project.Name,
			StartedAt:   started,
			Orphaned:    true,
		})
	}
	return orphans
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

	if r.rateLimitGate != nil {
		if active, until, reason, _, _ := r.rateLimitGate.Snapshot(); active {
			slog.Info("scheduler: rate-limit gate active, skipping tick",
				"paused_until", until, "reason", reason)
			return
		}
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

// pickTaskByID claims one specific queued task by ID, bypassing the scheduler's
// gate checks and priority ordering. It still enforces one-running-per-project
// via the NOT EXISTS subquery and FOR UPDATE SKIP LOCKED, with the DB unique
// index as the final backstop. Returns (nil, nil, nil) when the task is no
// longer eligible (status changed, or the project already has a running task).
func (r *Runner) pickTaskByID(
	taskID uuid.UUID,
) (*models.Task, *models.TaskExecution, error) {
	tx := r.db.Begin()
	if tx.Error != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", tx.Error)
	}
	defer tx.Rollback() //nolint:errcheck // safe no-op after commit

	var task models.Task
	err := tx.
		Where("id = ? AND status = ?", taskID, models.TaskStatusQueued).
		Where("NOT EXISTS (SELECT 1 FROM tasks t2 WHERE t2.project_id = tasks.project_id AND t2.status = ?)", models.TaskStatusRunning).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("query task: %w", err)
	}

	t, exec, err := r.claimTask(tx, &task)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return t, exec, nil
}

// ForceRunTask launches a specific queued task immediately, bypassing both
// rate-limit gates (the UsageMonitor 5h/7d threshold and the RateLimitGate
// cooldown). It backs the "Spustit teď" escape hatch for tasks stuck behind an
// exhausted limit. It still honors the hard invariants: the runner must not be
// hard-stopped, a worker slot must be free, and the task's project must have no
// other running task. A forced task runs through the normal executeTask path, so
// if it fails on a genuine rate-limit error the gate re-arms itself.
func (r *Runner) ForceRunTask(taskID uuid.UUID) (*models.Task, error) {
	var task models.Task
	if err := r.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, err // gorm.ErrRecordNotFound maps to 404 in the handler
	}
	if task.Status != models.TaskStatusQueued {
		return nil, ErrTaskNotQueued
	}

	// Pre-flight capacity check under the lock (mirrors collectTickState +
	// launchTask). The lock is released before the claim transaction; the DB
	// unique index and launchTask's re-check are the final authority, exactly as
	// in the normal tick path.
	r.mu.Lock()
	if r.state == models.StateStopped {
		r.mu.Unlock()
		return nil, ErrRunnerStopped
	}
	if len(r.executors) >= r.maxWorkers {
		r.mu.Unlock()
		return nil, ErrWorkersBusy
	}
	if _, busy := r.executors[task.ProjectID]; busy {
		r.mu.Unlock()
		return nil, ErrProjectBusy
	}
	r.mu.Unlock()

	claimed, execution, err := r.pickTaskByID(taskID)
	if err != nil {
		return nil, fmt.Errorf("claim task: %w", err)
	}
	if claimed == nil {
		// Lost the race: status changed or the project became busy between the
		// pre-flight check and the claim.
		return nil, ErrLaunchRace
	}

	// Snapshot the claimed task (status = running, started_at advanced) before
	// handing the pointer to doLaunch. In production doLaunch reaches
	// `go r.executeTask(...)`, whose goroutine mutates exported, json-tagged
	// fields of *claimed (BaseCommitSHA, HeadCommitSHA, RunPhase, Status,
	// FailureReason, UpdatedAt) concurrently with this function returning. The
	// caller JSON-marshals whatever ForceRunTask returns on the HTTP goroutine,
	// so returning the live pointer would be an unsynchronized read/write race.
	// A shallow copy is safe here because the executor only ever assigns new
	// pointers to those fields — it never mutates through the pointers already
	// on this snapshot.
	snapshot := *claimed

	if !r.doLaunch(claimed, execution) {
		// launchTask requeued it (a worker slot or the project was taken in the
		// gap between the pre-flight check and the launch).
		return nil, ErrLaunchRace
	}

	slog.Info("force-run: launched task past rate-limit gates",
		"task_id", claimed.ID, "project_id", claimed.ProjectID)
	return &snapshot, nil
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

	// started_at is advanced on every claim, not just the first, so a retried
	// task's displayed duration (completed_at - started_at) is the current
	// attempt's work rather than the work plus a superseded first attempt.
	// Per-attempt start times are preserved separately in task_executions below.
	now := time.Now()
	updates := map[string]interface{}{
		"status":     models.TaskStatusRunning,
		"started_at": now,
	}
	task.StartedAt = &now
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

	r.TaskEvents.Publish(TaskEvent{
		TaskID:    task.ID,
		Status:    models.TaskStatusRunning,
		ProjectID: task.ProjectID,
	})

	return task, &execution, nil
}

func (r *Runner) launchTask(task *models.Task, execution *models.TaskExecution) bool {
	r.mu.Lock()

	// Re-check the runner state under the lock: a caller (force-run's claim,
	// or tick's pickNextTask) can pass its pre-flight check, release r.mu for
	// the DB round-trip, and lose a race with HardStop, which sets state and
	// cancels only the executors present at that instant. Without this check
	// a task claimed just after HardStop would still be launched with a fresh
	// context.Background(), running unbounded on a runner the user just
	// stopped.
	if r.state == models.StateStopped {
		r.mu.Unlock()
		slog.Warn("scheduler: refusing to launch task, runner was hard-stopped",
			"task_id", task.ID, "project_id", task.ProjectID)
		r.unclaimTask(task)
		return false
	}

	if existing, ok := r.executors[task.ProjectID]; ok {
		r.mu.Unlock()
		slog.Error("scheduler: refusing to launch second task for same project",
			"project_id", task.ProjectID,
			"existing_task", existing.task.ID,
			"new_task", task.ID)
		r.unclaimTask(task)
		return false
	}

	if len(r.executors) >= r.maxWorkers {
		r.mu.Unlock()
		slog.Warn("scheduler: worker limit reached, requeuing task",
			"task_id", task.ID, "current_workers", len(r.executors), "max_workers", r.maxWorkers)
		r.unclaimTask(task)
		return false
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
	return true
}

// doLaunch launches a task, honoring a test override if one is installed. Used
// by ForceRunTask so tests can stub the launch; the scheduler's tick calls
// launchTask directly.
func (r *Runner) doLaunch(task *models.Task, execution *models.TaskExecution) bool {
	if r.launchFn != nil {
		return r.launchFn(task, execution)
	}
	return r.launchTask(task, execution)
}

// SetLaunchHookForTest installs a launch override. Intended for tests; production
// code leaves launchFn nil so doLaunch calls launchTask.
func (r *Runner) SetLaunchHookForTest(fn func(task *models.Task, execution *models.TaskExecution) bool) {
	r.launchFn = fn
}

// unclaimTask returns a freshly claimed task to the queue without counting it as
// an attempt. It also clears run_phase so the task never sits in "queued" carrying
// the phase of an execution that never started.
func (r *Runner) unclaimTask(task *models.Task) {
	r.db.Model(task).Updates(map[string]interface{}{
		"status":    models.TaskStatusQueued,
		"run_phase": nil,
	})
	task.RunPhase = nil
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

	// Resolve MCP servers for this project and generate config file.
	var mcpConfigPath string
	mcpServers, mcpErr := models.ResolveMCPServers(r.db, nil, &task.ProjectID)
	if mcpErr != nil {
		slog.Warn("failed to resolve MCP servers", "task_id", task.ID, "error", mcpErr)
	}
	if len(mcpServers) > 0 {
		mcpConfigPath, mcpErr = claude.GenerateMCPConfig(mcpServers, r.config.ClaudeContextDir, fmt.Sprintf("task-%s", task.ID))
		if mcpErr != nil {
			slog.Warn("failed to generate MCP config", "task_id", task.ID, "error", mcpErr)
		}
	}
	defer claude.RemoveMCPConfig(mcpConfigPath)

	// Capture git HEAD SHA before execution for potential revert and as the
	// task's base commit for diff display.
	headSHA := CaptureGitHEAD(&task.Project, r.executor.waker, r.executor.sshTarget)
	if headSHA != "" {
		exec.GitHeadSHA = &headSHA
		r.db.Model(exec).Update("git_head_sha", headSHA)
		if task.BaseCommitSHA == nil {
			task.BaseCommitSHA = &headSHA
			r.db.Model(task).Update("base_commit_sha", headSHA)
		}
	}

	result, err := r.executor.Execute(ctx, task, &task.Project, buf, mcpConfigPath)
	if err != nil {
		slog.Error("executor error", "task_id", task.ID, "error", err)
		result = &ExecutionResult{
			Status:       models.TaskStatusFailed,
			ErrorMessage: err.Error(),
		}
	}

	// If the task was killed by user, perform git revert.
	if result.ErrorMessage == "Killed by user" {
		GitRevert(&task.Project, r.executor.waker, r.executor.sshTarget, headSHA, task)
		// Set retry count to max so it won't auto-retry.
		r.db.Model(task).Update("retry_count", maxRetries)
		result.ShouldRetry = false
	} else {
		// Capture post-execution HEAD SHA so the diff endpoint can show the
		// changes the task produced. Skip on user kill since GitRevert resets
		// HEAD back to the base commit.
		if postSHA := CaptureGitHEAD(&task.Project, r.executor.waker, r.executor.sshTarget); postSHA != "" && postSHA != headSHA {
			task.HeadCommitSHA = &postSHA
			r.db.Model(task).Update("head_commit_sha", postSHA)
		}
	}

	r.finishTask(task, exec, buf, result)
}

func (r *Runner) finishTask(
	task *models.Task, exec *models.TaskExecution, buf *Buffer, result *ExecutionResult,
) {
	rawOutput := string(buf.ReadAll())
	r.updateExecution(exec, result, rawOutput)
	r.accumulateTaskUsage(task, result)
	r.maybeTripRateLimitGate(task, result, rawOutput)
	r.suppressRetryWhileRateLimited(result)

	// The failure summary is generated asynchronously, after the terminal status
	// write. Record `summarizing` here, at the last moment the task is still
	// `running`, so the phase never contradicts the status; finalizeTask clears it
	// on its way out.
	summarize := r.willSummarizeFailure(result)
	if summarize {
		r.recordPhase(task, models.RunPhaseSummarizing)
	}

	r.applyResult(task, result)

	if summarize {
		r.scheduleFailureSummary(task.ID, rawOutput)
	}

	buf.Close()
	r.mu.Lock()
	delete(r.executors, task.ProjectID)
	delete(r.buffers, task.ID)
	r.mu.Unlock()
}

// maybeTripRateLimitGate inspects the failure text from a completed task and
// — if Claude's own rate-limit signal is present — sets the global pause gate
// and forces the result into a no-retry shape so the task lands in `failed`
// rather than being requeued for an immediate retry storm.
//
// The detector is bypassed entirely when RATE_LIMIT_DETECTION_ENABLED=false.
func (r *Runner) maybeTripRateLimitGate(
	task *models.Task, result *ExecutionResult, rawOutput string,
) {
	if r.rateLimitGate == nil {
		return
	}
	if r.config != nil && !r.config.RateLimitDetectionEnabled {
		return
	}
	if result.Status != models.TaskStatusFailed {
		return
	}

	failureText := result.ErrorMessage + "\n" + result.Summary + "\n" + rawOutput
	hit, resetAt, ok := DetectRateLimit(failureText)
	if !hit {
		return
	}

	cooldown := 2 * time.Hour //nolint:mnd // sensible default; overridden by config
	if r.config != nil && r.config.ClaudeRateLimitCooldown > 0 {
		cooldown = r.config.ClaudeRateLimitCooldown
	}
	pauseUntil := resetAt
	if !ok || pauseUntil.IsZero() {
		pauseUntil = time.Now().Add(cooldown)
	}

	reason := fmt.Sprintf("Claude rate limit (task %s)", task.ID)
	r.rateLimitGate.PauseUntil(pauseUntil, reason, task.ID)
	until := r.rateLimitGate.PausedUntil()
	slog.Warn("runner paused due to Claude rate limit",
		"paused_until", until, "task_id", task.ID)

	// Don't retry into the wall: force the task to land in `failed` rather
	// than getting requeued for an immediate second attempt.
	result.ShouldRetry = false
	result.RetryAfter = 0
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

// accumulateTaskUsage adds the tokens and cost from a single execution to the
// task row's running totals. Tasks may be retried, in which case the totals
// reflect the sum across all attempts. Skips the write when no result event
// was parsed (all token counts zero), so a crash during startup doesn't clobber
// the prior nullable state with zeros. The model column always reflects the
// most recent attempt's model, since retries can change the active model.
func (r *Runner) accumulateTaskUsage(task *models.Task, result *ExecutionResult) {
	if result.InputTokens == 0 && result.OutputTokens == 0 &&
		result.CacheReadTokens == 0 && result.CacheCreationTokens == 0 {
		return
	}
	updates := map[string]interface{}{
		"input_tokens":          gorm.Expr("COALESCE(input_tokens, 0) + ?", result.InputTokens),
		"output_tokens":         gorm.Expr("COALESCE(output_tokens, 0) + ?", result.OutputTokens),
		"cache_read_tokens":     gorm.Expr("COALESCE(cache_read_tokens, 0) + ?", result.CacheReadTokens),
		"cache_creation_tokens": gorm.Expr("COALESCE(cache_creation_tokens, 0) + ?", result.CacheCreationTokens),
		"cost_usd":              gorm.Expr("COALESCE(cost_usd, 0) + ?", result.CostUSD),
	}
	if result.Model != "" {
		updates["model"] = result.Model
	}
	r.db.Model(task).Updates(updates)
}

// suppressRetryWhileRateLimited enforces the "don't retry into the wall" rule:
// while the rate-limit gate is active, a failed task is never requeued. It lands
// in `failed` without a retry_count increment and is picked up naturally after
// the gate clears (since the original task remains in `failed` it stays failed;
// the next queued task on the project gets its turn).
//
// Calling it twice is a no-op — finishTask needs the decision before it can tell
// whether a failure summary is coming, and applyResult reasserts it for callers
// that reach it directly.
func (r *Runner) suppressRetryWhileRateLimited(result *ExecutionResult) {
	if r.rateLimitGate != nil && r.rateLimitGate.IsActive() &&
		result.Status == models.TaskStatusFailed {
		result.ShouldRetry = false
		result.RetryAfter = 0
	}
}

// willSummarizeFailure reports whether finishTask will schedule a failure summary
// for this result. It mirrors scheduleFailureSummary's own guards so the
// `summarizing` phase is only recorded when a summary actually follows.
func (r *Runner) willSummarizeFailure(result *ExecutionResult) bool {
	if r.config == nil || !r.config.FailureSummaryEnabled {
		return false
	}
	return !result.ShouldRetry && result.RetryAfter == 0 &&
		result.Status == models.TaskStatusFailed
}

func (r *Runner) applyResult(task *models.Task, result *ExecutionResult) {
	r.suppressRetryWhileRateLimited(result)

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
	// Clean the working tree before the next attempt is eligible to run, so it
	// starts from the same state the first one did instead of inheriting a partial
	// commit or stray files. Done while the executor is still registered for this
	// project, so the scheduler cannot pick the task up mid-reset.
	r.resetTreeToBase(task)

	r.db.Model(task).Updates(map[string]interface{}{
		"status":          models.TaskStatusQueued,
		"run_phase":       nil,
		"retry_count":     gorm.Expr("retry_count + 1"),
		"failure_reason":  errMsg,
		"failure_summary": gorm.Expr("NULL"),
	})
	task.RunPhase = nil
	r.TaskEvents.Publish(TaskEvent{
		TaskID:    task.ID,
		Status:    models.TaskStatusQueued,
		ProjectID: task.ProjectID,
	})
}

// resetTreeToBase parks whatever the failed attempt produced onto a pushed
// wip/task-<id>-attempt-<n> branch and then hard-resets the project's working
// tree to the task's recorded base commit, so the retry starts from a pristine
// tree without the attempt being lost. The attempt number is the current
// retry_count + 1, read before requeueTask increments it. It is a no-op (with a
// warning) when base_commit_sha was never captured — resetting to a guessed
// commit could corrupt the tree — or when the runner has no executor wired (as
// in some unit tests).
func (r *Runner) resetTreeToBase(task *models.Task) {
	if task.BaseCommitSHA == nil || *task.BaseCommitSHA == "" {
		slog.Warn("skipping pre-retry tree reset: base commit not recorded", "task_id", task.ID)
		return
	}
	if r.executor == nil {
		return
	}
	attempt := task.RetryCount + 1
	ParkLeftoversAndReset(
		&task.Project, r.executor.waker, r.executor.sshTarget, *task.BaseCommitSHA, task, attempt)
}

// commitLeftovers is the terminal-path safety net: on a task's final outcome it
// commits and pushes any work the agent left uncommitted in the working tree,
// so a task never ends `done`/`failed` with dirty, un-saved changes (the usual
// cause: the execution timeout (TASK_TIMEOUT) killing the agent mid-`make check`,
// before it committed). On a user kill GitRevert has already cleaned the tree, so this is
// a no-op there; likewise for a clean success. It runs before the terminal
// status write and only when an executor is wired (skipped in unit tests).
func (r *Runner) commitLeftovers(task *models.Task) {
	if r.executor == nil {
		return
	}
	msg := fmt.Sprintf("botka: auto-commit leftover work from task %s (%s)", task.ID, task.Title)
	CommitLeftovers(&task.Project, r.executor.waker, r.executor.sshTarget, task, msg)
}

func (r *Runner) finalizeTask(task *models.Task, result *ExecutionResult) {
	r.commitLeftovers(task)

	now := time.Now()
	// run_phase is cleared unconditionally: it only describes a running task, and
	// this is the single write every terminal status funnels through — including
	// the one that lands after a kill or a mid-phase crash.
	updates := map[string]interface{}{
		"status":       result.Status,
		"completed_at": now,
		"run_phase":    nil,
	}
	switch {
	case isSuccessful(result.Status):
		// A finished task must not carry an error from a superseded attempt — e.g.
		// a timeout on attempt 1 that a later attempt recovered from. Clear it.
		updates["failure_reason"] = nil
	case result.ErrorMessage != "":
		updates["failure_reason"] = result.ErrorMessage
	}
	r.db.Model(task).Updates(updates)
	slog.Info("task finished", "task_id", task.ID, "status", result.Status)

	// Mirror the in-memory task with the freshly persisted fields so the push
	// payload uses the same status and failure reason that just hit the DB.
	task.Status = result.Status
	task.RunPhase = nil
	switch {
	case isSuccessful(result.Status):
		task.FailureReason = nil
	case result.ErrorMessage != "":
		em := result.ErrorMessage
		task.FailureReason = &em
	}
	r.notifyTaskTransition(task, result.Status)

	r.TaskEvents.Publish(TaskEvent{
		TaskID:    task.ID,
		Status:    result.Status,
		ProjectID: task.ProjectID,
	})

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
