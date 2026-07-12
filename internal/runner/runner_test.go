package runner

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"botka/internal/models"
)

var (
	testDBOnce sync.Once
	sharedDB   *gorm.DB
	dbErr      error
)

// setupTestDB connects to the botka_test database once per test run.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	testDBOnce.Do(func() {
		dsn := os.Getenv("DATABASE_TEST_URL")
		if dsn == "" {
			dsn = "postgres://botka:botka@localhost:5432/botka_test?sslmode=disable"
		}
		sharedDB, dbErr = gorm.Open(postgres.Open(dsn), &gorm.Config{
			SkipDefaultTransaction: true,
			Logger:                 logger.Default.LogMode(logger.Silent),
		})
		if dbErr == nil {
			// task_schedules is dropped alongside the others because another test
			// binary (handlers) can leave orphan rows in it that reference projects
			// this DROP removes; without dropping it, GORM's re-validation of
			// fk_task_schedules_project below fails and every runner DB test skips.
			// schedule_test.go re-creates it via its own bootstrap when needed.
			sharedDB.Exec("DROP TABLE IF EXISTS task_executions, tasks, task_schedules, projects, runner_state CASCADE")
			dbErr = sharedDB.AutoMigrate(
				&models.Project{},
				&models.Task{},
				&models.TaskExecution{},
			)
			if dbErr == nil {
				sharedDB.Exec(`CREATE TABLE IF NOT EXISTS runner_state (
					id INTEGER PRIMARY KEY DEFAULT 1,
					state TEXT NOT NULL DEFAULT 'stopped',
					completed_count INTEGER NOT NULL DEFAULT 0,
					task_limit INTEGER,
					updated_at TIMESTAMPTZ
				)`)
				sharedDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_one_running_per_project
					ON tasks (project_id) WHERE status = 'running'`)
			}
		}
	})
	if dbErr != nil {
		t.Skipf("test database unavailable: %v", dbErr)
	}
	return sharedDB
}

func cleanTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	db.Exec("TRUNCATE TABLE task_executions, tasks, projects, runner_state CASCADE")
}

func createProject(t *testing.T, db *gorm.DB, name string) models.Project {
	t.Helper()
	p := models.Project{
		Name:           name,
		Path:           "/tmp/" + name + "-" + uuid.New().String()[:8],
		BranchStrategy: "main",
		Active:         true,
	}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p
}

func createTask(t *testing.T, db *gorm.DB, projectID uuid.UUID, title string, status models.TaskStatus) models.Task {
	t.Helper()
	task := models.Task{
		Title:     title,
		Spec:      "test spec",
		ProjectID: projectID,
		Status:    status,
		Priority:  5,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task
}

func TestBuildPickQuery_ExcludesActiveProjects(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	projA := createProject(t, db, "project-a")
	projB := createProject(t, db, "project-b")
	createTask(t, db, projA.ID, "task-a-queued", models.TaskStatusQueued)
	taskB := createTask(t, db, projB.ID, "task-b-queued", models.TaskStatusQueued)

	r := &Runner{db: db}

	// Exclude projA — should only find task from projB.
	tx := db.Begin()
	defer tx.Rollback() //nolint:errcheck

	var task models.Task
	err := r.buildPickQuery(tx, []uuid.UUID{projA.ID}, nil).First(&task).Error
	if err != nil {
		t.Fatalf("expected to find a task, got error: %v", err)
	}
	if task.ID != taskB.ID {
		t.Errorf("expected task %v (project-b), got %v", taskB.ID, task.ID)
	}
	if task.ProjectID != projB.ID {
		t.Errorf("expected project_id %v, got %v", projB.ID, task.ProjectID)
	}
}

func TestBuildPickQuery_ExcludesAllActiveProjects(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	projA := createProject(t, db, "project-a")
	projB := createProject(t, db, "project-b")
	createTask(t, db, projA.ID, "task-a", models.TaskStatusQueued)
	createTask(t, db, projB.ID, "task-b", models.TaskStatusQueued)

	r := &Runner{db: db}

	// Exclude both projects — should find nothing.
	tx := db.Begin()
	defer tx.Rollback() //nolint:errcheck

	var task models.Task
	err := r.buildPickQuery(tx, []uuid.UUID{projA.ID, projB.ID}, nil).First(&task).Error
	if err == nil {
		t.Fatalf("expected no task, got task %v for project %v", task.ID, task.ProjectID)
	}
}

func TestBuildPickQuery_ExcludesProjectsWithRunningTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	projA := createProject(t, db, "project-a")
	projB := createProject(t, db, "project-b")

	// Project A has a running task AND a queued task.
	createTask(t, db, projA.ID, "task-a-running", models.TaskStatusRunning)
	createTask(t, db, projA.ID, "task-a-queued", models.TaskStatusQueued)
	// Project B has only a queued task.
	taskB := createTask(t, db, projB.ID, "task-b-queued", models.TaskStatusQueued)

	r := &Runner{db: db}

	// Even without passing activeProjectIDs, the DB subquery should exclude project A.
	tx := db.Begin()
	defer tx.Rollback() //nolint:errcheck

	var task models.Task
	err := r.buildPickQuery(tx, nil, nil).First(&task).Error
	if err != nil {
		t.Fatalf("expected to find a task, got error: %v", err)
	}
	if task.ID != taskB.ID {
		t.Errorf("expected task %v (project-b), got %v", taskB.ID, task.ID)
	}
}

func TestBuildPickQuery_DBLevelBlocksAllSameProject(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-only")

	// Project has a running task and a queued task.
	createTask(t, db, proj.ID, "running-task", models.TaskStatusRunning)
	createTask(t, db, proj.ID, "queued-task", models.TaskStatusQueued)

	r := &Runner{db: db}

	// No in-memory exclusions — DB subquery must block it.
	tx := db.Begin()
	defer tx.Rollback() //nolint:errcheck

	var task models.Task
	err := r.buildPickQuery(tx, nil, nil).First(&task).Error
	if err == nil {
		t.Fatalf("expected no task (project has running task), got task %v", task.ID)
	}
}

func TestLaunchTask_RefusesDuplicateProject(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-dup")
	task1 := createTask(t, db, proj.ID, "first-task", models.TaskStatusRunning)
	// Create as queued — the unique index prevents two running tasks per project.
	// This test validates the in-memory guard in launchTask, not the DB constraint.
	task2 := createTask(t, db, proj.ID, "second-task", models.TaskStatusQueued)

	r := &Runner{
		db:        db,
		executors: make(map[uuid.UUID]*activeTask),
		buffers:   make(map[uuid.UUID]*Buffer),
	}

	// Simulate first task already running.
	r.executors[proj.ID] = &activeTask{
		task:      &task1,
		execution: &models.TaskExecution{TaskID: task1.ID},
	}

	// Try to launch second task for the same project.
	if r.launchTask(&task2, &models.TaskExecution{TaskID: task2.ID}) {
		t.Error("expected launchTask to return false when project is busy")
	}

	// The executor should still reference the first task.
	r.mu.RLock()
	at, ok := r.executors[proj.ID]
	r.mu.RUnlock()

	if !ok {
		t.Fatal("expected executor for project to still exist")
	}
	if at.task.ID != task1.ID {
		t.Errorf("expected executor to still reference task %v, got %v", task1.ID, at.task.ID)
	}

	// The second task should be requeued.
	var reloaded models.Task
	if err := db.First(&reloaded, task2.ID).Error; err != nil {
		t.Fatalf("reload task2: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected task2 status %q, got %q", models.TaskStatusQueued, reloaded.Status)
	}
}

func TestLaunchTask_RefusesWhenMaxWorkersReached(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	projA := createProject(t, db, "project-a")
	projB := createProject(t, db, "project-b")
	projC := createProject(t, db, "project-c")
	taskA := createTask(t, db, projA.ID, "task-a", models.TaskStatusRunning)
	taskB := createTask(t, db, projB.ID, "task-b", models.TaskStatusRunning)
	taskC := createTask(t, db, projC.ID, "task-c", models.TaskStatusQueued)

	r := &Runner{
		db:         db,
		maxWorkers: 2,
		executors:  make(map[uuid.UUID]*activeTask),
		buffers:    make(map[uuid.UUID]*Buffer),
	}

	// Simulate two tasks already running (at max_workers limit).
	r.executors[projA.ID] = &activeTask{
		task:      &taskA,
		execution: &models.TaskExecution{TaskID: taskA.ID},
	}
	r.executors[projB.ID] = &activeTask{
		task:      &taskB,
		execution: &models.TaskExecution{TaskID: taskB.ID},
	}

	// Try to launch a third task on a different project.
	if r.launchTask(&taskC, &models.TaskExecution{TaskID: taskC.ID}) {
		t.Error("expected launchTask to return false when workers are full")
	}

	// Should still have only 2 executors.
	r.mu.RLock()
	count := len(r.executors)
	_, hasC := r.executors[projC.ID]
	r.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 executors, got %d", count)
	}
	if hasC {
		t.Error("executor for project-c should not exist")
	}

	// Task C should be requeued.
	var reloaded models.Task
	if err := db.First(&reloaded, taskC.ID).Error; err != nil {
		t.Fatalf("reload taskC: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected taskC status %q, got %q", models.TaskStatusQueued, reloaded.Status)
	}
}

// TestDoLaunch_NilLaunchFnUsesRealLaunchTask exercises the branch of doLaunch
// that every other force-run test bypasses by installing launchFn: with
// launchFn left nil (as it always is in production), doLaunch must delegate
// to the real launchTask. It proves delegation by giving the task under test
// a "running" DB row before the call: only a real launchTask call — hitting
// the duplicate-project guard, which calls unclaimTask — can flip that row
// back to "queued". If doLaunch instead short-circuited to `return false`
// without ever calling launchTask, the row would stay "running" and the
// final assertion below would fail. maxWorkers is set to 1 so the guard that
// fires is unambiguously the duplicate-project one, not a max-workers
// coincidence.
func TestDoLaunch_NilLaunchFnUsesRealLaunchTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "dolaunch-real")
	// This row's own status is irrelevant to the guard under test; it is
	// created non-"running" only because idx_one_running_per_project forbids
	// two running rows for the same project, and the task under test below
	// occupies that slot for the duration of this test.
	running := createTask(t, db, proj.ID, "already-running", models.TaskStatusDone)
	task := createTask(t, db, proj.ID, "queued", models.TaskStatusRunning)

	r := &Runner{
		db:         db,
		maxWorkers: 1,
		executors:  make(map[uuid.UUID]*activeTask),
		buffers:    make(map[uuid.UUID]*Buffer),
	}
	r.executors[proj.ID] = &activeTask{
		task:      &running,
		execution: &models.TaskExecution{TaskID: running.ID},
	}
	// launchFn intentionally left nil.

	if r.doLaunch(&task, &models.TaskExecution{TaskID: task.ID}) {
		t.Fatal("expected doLaunch to return false: project already has a running executor")
	}

	// The pre-existing executor must be untouched — a real launchTask call
	// (rather than a stub) is what enforces this.
	r.mu.RLock()
	at, ok := r.executors[proj.ID]
	r.mu.RUnlock()
	if !ok || at.task.ID != running.ID {
		t.Error("expected executor for project to still reference the original running task")
	}

	// Only a real launchTask call reaches the duplicate-project guard and
	// calls unclaimTask, which is what can flip this row from "running" back
	// to "queued". A doLaunch that never delegates to launchTask would leave
	// it "running".
	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected task back to queued, got %s", reloaded.Status)
	}
}

// TestLaunchTask_RefusesWhenHardStopped covers the TOCTOU window between a
// caller's pre-flight state check and launchTask's own claim: if HardStop
// flips state to StateStopped after the pre-flight passed but before
// launchTask acquires the lock, launchTask must still refuse rather than
// start an executor with a fresh context.Background() that HardStop never
// had a chance to cancel.
//
// The task row is created "running" (mirroring claimTask's write in
// production, which always precedes a launchTask call) and maxWorkers is set
// to 1 with an empty executors map, so the max-workers guard (len(executors)
// = 0 >= maxWorkers = 1 is false) cannot fire and produce the same
// running -> queued unclaim as a side effect. Only the hard-stop guard can
// explain the DB write asserted below.
func TestLaunchTask_RefusesWhenHardStopped(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-stopped")
	task := createTask(t, db, proj.ID, "task", models.TaskStatusRunning)

	r := &Runner{
		db:         db,
		state:      models.StateStopped,
		maxWorkers: 1,
		executors:  make(map[uuid.UUID]*activeTask),
		buffers:    make(map[uuid.UUID]*Buffer),
	}

	if r.launchTask(&task, &models.TaskExecution{TaskID: task.ID}) {
		t.Error("expected launchTask to return false when runner is hard-stopped")
	}

	r.mu.RLock()
	_, hasExecutor := r.executors[proj.ID]
	r.mu.RUnlock()
	if hasExecutor {
		t.Error("expected no executor to be registered for a hard-stopped runner")
	}

	// Only the hard-stop guard's unclaimTask call can flip this row from
	// "running" back to "queued" — with maxWorkers: 1 and an empty executors
	// map, the max-workers guard cannot fire here.
	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected task back to queued, got %s", reloaded.Status)
	}
}

func TestUniqueIndex_PreventsSecondRunningTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-unique")
	createTask(t, db, proj.ID, "first-running", models.TaskStatusRunning)

	// Attempting to create a second running task for the same project must fail.
	second := models.Task{
		Title:     "second-running",
		Spec:      "test spec",
		ProjectID: proj.ID,
		Status:    models.TaskStatusRunning,
		Priority:  5,
	}
	err := db.Create(&second).Error
	if err == nil {
		t.Fatal("expected unique violation error, got nil")
	}
	if !isUniqueViolation(err) {
		t.Fatalf("expected unique violation, got: %v", err)
	}
}

func TestPickNextTask_UniqueViolationSkips(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-race")
	// Simulate another process already claimed a running task.
	createTask(t, db, proj.ID, "already-running", models.TaskStatusRunning)
	// A queued task exists for the same project.
	createTask(t, db, proj.ID, "wants-to-run", models.TaskStatusQueued)

	r := &Runner{
		db:             db,
		executors:      make(map[uuid.UUID]*activeTask),
		retryNotBefore: make(map[uuid.UUID]time.Time),
	}

	// The NOT EXISTS subquery should filter it out, so pickNextTask returns nil.
	task, exec, err := r.pickNextTask(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task != nil {
		t.Errorf("expected no task (project already has running task), got task %v", task.ID)
	}
	if exec != nil {
		t.Errorf("expected no execution, got %v", exec.ID)
	}
}

func TestKillTask_CancelsRunningTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-kill")
	task := createTask(t, db, proj.ID, "task-to-kill", models.TaskStatusRunning)

	cancelled := false
	r := &Runner{
		db:        db,
		executors: make(map[uuid.UUID]*activeTask),
		buffers:   make(map[uuid.UUID]*Buffer),
	}
	r.executors[proj.ID] = &activeTask{
		task:      &task,
		execution: &models.TaskExecution{TaskID: task.ID},
		cancel:    func() { cancelled = true },
	}

	err := r.KillTask(task.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cancelled {
		t.Error("expected cancel to be called")
	}
}

func TestKillTask_NotRunning(t *testing.T) {
	r := &Runner{
		executors: make(map[uuid.UUID]*activeTask),
	}

	err := r.KillTask(uuid.New())
	if err == nil {
		t.Fatal("expected error for non-running task")
	}
	if !strings.Contains(err.Error(), "not currently running") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRestoreState_RecoversOrphanedTasksWhenStopped(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Simulate a previous process that crashed: runner_state is "stopped"
	// but some tasks are stuck in "running" status (orphaned).
	db.Exec(`INSERT INTO runner_state (id, state, completed_count, task_limit, updated_at)
		VALUES (1, 'stopped', 0, 0, NOW())`)

	proj := createProject(t, db, "project-orphan")
	orphan := createTask(t, db, proj.ID, "orphaned-running-task", models.TaskStatusRunning)
	queued := createTask(t, db, proj.ID, "queued-task", models.TaskStatusQueued)

	usageMon := &UsageMonitor{done: make(chan struct{})}
	r := &Runner{
		db:             db,
		state:          models.StateStopped,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		usageMon:       usageMon,
		retryNotBefore: make(map[uuid.UUID]time.Time),
	}
	r.state = r.loadState()

	// RestoreState should recover orphaned tasks even when state is "stopped".
	r.RestoreState()

	// The orphaned task should be requeued.
	var reloaded models.Task
	if err := db.First(&reloaded, orphan.ID).Error; err != nil {
		t.Fatalf("reload orphaned task: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected orphaned task status %q, got %q", models.TaskStatusQueued, reloaded.Status)
	}

	// The already-queued task should be unaffected.
	var qReloaded models.Task
	if err := db.First(&qReloaded, queued.ID).Error; err != nil {
		t.Fatalf("reload queued task: %v", err)
	}
	if qReloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected queued task status %q, got %q", models.TaskStatusQueued, qReloaded.Status)
	}

	// Runner state should remain stopped (scheduler loop not started).
	if r.state != models.StateStopped {
		t.Errorf("expected runner state %q, got %q", models.StateStopped, r.state)
	}
}

func TestRestoreState_RecoversOrphanedTasksWhenPaused(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Runner state is "paused" with orphaned running tasks.
	db.Exec(`INSERT INTO runner_state (id, state, completed_count, task_limit, updated_at)
		VALUES (1, 'paused', 0, 0, NOW())`)

	proj := createProject(t, db, "project-paused")
	orphan := createTask(t, db, proj.ID, "orphaned-task", models.TaskStatusRunning)

	usageMon := &UsageMonitor{done: make(chan struct{})}
	r := &Runner{
		db:             db,
		state:          models.StatePaused,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		usageMon:       usageMon,
		retryNotBefore: make(map[uuid.UUID]time.Time),
	}
	r.state = r.loadState()

	r.RestoreState()

	var reloaded models.Task
	if err := db.First(&reloaded, orphan.ID).Error; err != nil {
		t.Fatalf("reload orphaned task: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected orphaned task status %q, got %q", models.TaskStatusQueued, reloaded.Status)
	}

	// Runner state should remain paused.
	if r.state != models.StatePaused {
		t.Errorf("expected runner state %q, got %q", models.StatePaused, r.state)
	}
}

func TestGetStatus_IncludesOrphanedRunningTasks(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Simulate a previous process that left a task in running state but whose
	// in-memory executors map is empty (e.g. Botka restarted while the Claude
	// subprocess was still alive, or RestoreState has not yet been called).
	// started_at must be older than orphanGracePeriod to be detected.
	proj := createProject(t, db, "project-orphan-status")
	orphan := createTask(t, db, proj.ID, "orphan-task", models.TaskStatusRunning)
	oldStart := time.Now().Add(-2 * time.Minute)
	db.Model(&orphan).Update("started_at", oldStart)

	r := &Runner{
		db:         db,
		state:      models.StateStopped,
		maxWorkers: 2,
		executors:  make(map[uuid.UUID]*activeTask),
		buffers:    make(map[uuid.UUID]*Buffer),
		usageMon:   &UsageMonitor{done: make(chan struct{})},
	}

	status := r.GetStatus()

	if len(status.ActiveTasks) != 1 {
		t.Fatalf("expected 1 active task (the orphan), got %d", len(status.ActiveTasks))
	}
	got := status.ActiveTasks[0]
	if got.TaskID != orphan.ID {
		t.Errorf("expected orphan task ID %v, got %v", orphan.ID, got.TaskID)
	}
	if !got.Orphaned {
		t.Error("expected orphaned=true for DB-only running task")
	}
	if got.ProjectName != proj.Name {
		t.Errorf("expected project name %q, got %q", proj.Name, got.ProjectName)
	}
	if status.State != models.StateStopped {
		t.Errorf("expected state stopped, got %q", status.State)
	}
	if status.Draining {
		t.Error("orphaned tasks should not set Draining")
	}
}

func TestGetStatus_TrackedTasksNotMarkedOrphaned(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-tracked")
	task := createTask(t, db, proj.ID, "tracked-task", models.TaskStatusRunning)
	task.Project = proj

	r := &Runner{
		db:         db,
		state:      models.StateRunning,
		maxWorkers: 2,
		executors:  make(map[uuid.UUID]*activeTask),
		buffers:    make(map[uuid.UUID]*Buffer),
		usageMon:   &UsageMonitor{done: make(chan struct{})},
	}
	startedAt := time.Now().Add(-30 * time.Second)
	r.executors[proj.ID] = &activeTask{
		task:      &task,
		execution: &models.TaskExecution{TaskID: task.ID, StartedAt: startedAt},
	}

	status := r.GetStatus()

	if len(status.ActiveTasks) != 1 {
		t.Fatalf("expected exactly 1 active task, got %d", len(status.ActiveTasks))
	}
	got := status.ActiveTasks[0]
	if got.TaskID != task.ID {
		t.Errorf("expected task ID %v, got %v", task.ID, got.TaskID)
	}
	if got.Orphaned {
		t.Error("expected orphaned=false for in-memory tracked task")
	}
	if !got.StartedAt.Equal(startedAt) {
		t.Errorf("expected started_at to come from execution (%v), got %v", startedAt, got.StartedAt)
	}
}

func TestGetStatus_MixedTrackedAndOrphaned(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	projTracked := createProject(t, db, "project-mixed-tracked")
	projOrphan := createProject(t, db, "project-mixed-orphan")

	tracked := createTask(t, db, projTracked.ID, "tracked", models.TaskStatusRunning)
	tracked.Project = projTracked
	orphan := createTask(t, db, projOrphan.ID, "orphan", models.TaskStatusRunning)
	oldStart := time.Now().Add(-2 * time.Minute)
	db.Model(&orphan).Update("started_at", oldStart)

	r := &Runner{
		db:         db,
		state:      models.StateRunning,
		maxWorkers: 2,
		executors:  make(map[uuid.UUID]*activeTask),
		buffers:    make(map[uuid.UUID]*Buffer),
		usageMon:   &UsageMonitor{done: make(chan struct{})},
	}
	r.executors[projTracked.ID] = &activeTask{
		task:      &tracked,
		execution: &models.TaskExecution{TaskID: tracked.ID, StartedAt: time.Now()},
	}

	status := r.GetStatus()

	if len(status.ActiveTasks) != 2 {
		t.Fatalf("expected 2 active tasks (1 tracked + 1 orphaned), got %d", len(status.ActiveTasks))
	}

	var trackedInfo, orphanInfo *ActiveTaskInfo
	for i := range status.ActiveTasks {
		switch status.ActiveTasks[i].TaskID {
		case tracked.ID:
			trackedInfo = &status.ActiveTasks[i]
		case orphan.ID:
			orphanInfo = &status.ActiveTasks[i]
		}
	}

	if trackedInfo == nil {
		t.Fatal("tracked task missing from active_tasks")
	}
	if trackedInfo.Orphaned {
		t.Error("tracked task should not be flagged orphaned")
	}
	if orphanInfo == nil {
		t.Fatal("orphan task missing from active_tasks")
	}
	if !orphanInfo.Orphaned {
		t.Error("db-only task should be flagged orphaned")
	}
}

func TestGetStatus_NoRunningTasksReturnsEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-empty")
	// A queued task exists but no running tasks.
	createTask(t, db, proj.ID, "queued-only", models.TaskStatusQueued)

	r := &Runner{
		db:         db,
		state:      models.StatePaused,
		maxWorkers: 2,
		executors:  make(map[uuid.UUID]*activeTask),
		buffers:    make(map[uuid.UUID]*Buffer),
		usageMon:   &UsageMonitor{done: make(chan struct{})},
	}

	status := r.GetStatus()

	if len(status.ActiveTasks) != 0 {
		t.Errorf("expected no active tasks, got %d", len(status.ActiveTasks))
	}
}

func TestGetStatus_GracePeriodHidesRecentlyStartedTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// A task that just transitioned to running (within grace period) should
	// NOT appear as orphaned even though it has no in-memory executor yet.
	// This covers the race between claimTask (DB commit) and launchTask.
	proj := createProject(t, db, "project-grace")
	recentStart := time.Now().Add(-5 * time.Second)
	task := createTask(t, db, proj.ID, "just-claimed", models.TaskStatusRunning)
	db.Model(&task).Update("started_at", recentStart)

	r := &Runner{
		db:         db,
		state:      models.StateRunning,
		maxWorkers: 2,
		executors:  make(map[uuid.UUID]*activeTask),
		buffers:    make(map[uuid.UUID]*Buffer),
		usageMon:   &UsageMonitor{done: make(chan struct{})},
	}

	status := r.GetStatus()

	if len(status.ActiveTasks) != 0 {
		t.Fatalf("expected 0 active tasks (grace period should hide recently started task), got %d", len(status.ActiveTasks))
	}
}

func TestGetStatus_GracePeriodExpiresForTrulyOrphanedTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// A task that has been running long past the grace period with no executor
	// is truly orphaned and should be reported.
	proj := createProject(t, db, "project-expired-grace")
	oldStart := time.Now().Add(-5 * time.Minute)
	task := createTask(t, db, proj.ID, "stuck-task", models.TaskStatusRunning)
	db.Model(&task).Update("started_at", oldStart)

	r := &Runner{
		db:         db,
		state:      models.StateStopped,
		maxWorkers: 2,
		executors:  make(map[uuid.UUID]*activeTask),
		buffers:    make(map[uuid.UUID]*Buffer),
		usageMon:   &UsageMonitor{done: make(chan struct{})},
	}

	status := r.GetStatus()

	if len(status.ActiveTasks) != 1 {
		t.Fatalf("expected 1 active task (orphaned after grace period), got %d", len(status.ActiveTasks))
	}
	if !status.ActiveTasks[0].Orphaned {
		t.Error("expected orphaned=true for task past grace period with no executor")
	}
	if status.ActiveTasks[0].TaskID != task.ID {
		t.Errorf("expected task ID %v, got %v", task.ID, status.ActiveTasks[0].TaskID)
	}
}

func TestKillTask_IdempotentAfterCompletion(t *testing.T) {
	// After a task finishes, its executor is removed. A second kill should return an error.
	r := &Runner{
		executors: make(map[uuid.UUID]*activeTask),
	}

	taskID := uuid.New()
	err := r.KillTask(taskID)
	if err == nil {
		t.Fatal("expected error for non-running task")
	}

	// Call again — should still return error (idempotent).
	err = r.KillTask(taskID)
	if err == nil {
		t.Fatal("expected error on second kill attempt")
	}
}

// newForceRunner builds a StateRunning runner whose gates report "ready" so
// ForceRunTask tests exercise only the force logic, not the gate bypass path.
//
// launchFn is pre-installed as a fail-fast hook: if a regression ever breaks
// the pre-flight refusal checks, a test that expects ForceRunTask to bail out
// early would otherwise fall through to the real launchTask on a Runner whose
// executor/config fields are nil, panicking in a background goroutine after
// the test has already finished (a flaky truncation race, not a clean
// failure). Tests that legitimately expect a launch must override r.launchFn
// themselves after construction.
func newForceRunner(t *testing.T, db *gorm.DB) *Runner {
	t.Helper()
	usage := NewUsageMonitor("", 0.99, 0.99)
	usage.lastPollOK = true
	r := &Runner{
		db:             db,
		state:          models.StateRunning,
		maxWorkers:     2,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		retryNotBefore: make(map[uuid.UUID]time.Time),
		usageMon:       usage,
		rateLimitGate:  NewRateLimitGate(nil),
		TaskEvents:     NewTaskEventHub(),
	}
	r.launchFn = func(*models.Task, *models.TaskExecution) bool {
		t.Fatal("launch must not be called")
		return false
	}
	return r
}

func TestForceRunTask_LaunchesDespiteActiveGates(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-force")
	task := createTask(t, db, proj.ID, "stuck-task", models.TaskStatusQueued)

	// Both gates would block a normal tick: usage never polled OK (rate limited),
	// and the rate-limit gate is paused.
	usage := NewUsageMonitor("", 0.99, 0.99)
	usage.lastPollOK = false // IsRateLimited() == true
	gate := NewRateLimitGate(nil)
	gate.PauseUntil(time.Now().Add(2*time.Hour), "test pause", uuid.New())

	var launched *models.Task
	r := &Runner{
		db:             db,
		state:          models.StateRunning,
		maxWorkers:     2,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		retryNotBefore: make(map[uuid.UUID]time.Time),
		usageMon:       usage,
		rateLimitGate:  gate,
		TaskEvents:     NewTaskEventHub(),
		launchFn: func(tk *models.Task, _ *models.TaskExecution) bool {
			launched = tk
			return true
		},
	}

	got, err := r.ForceRunTask(task.ID)
	if err != nil {
		t.Fatalf("ForceRunTask: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("expected returned task %v, got %v", task.ID, got)
	}
	// The returned snapshot must be taken after the claim, not before: it
	// should already carry the claimed running status and advanced
	// started_at, not the pre-claim "queued"/nil-started_at struct.
	if got.Status != models.TaskStatusRunning {
		t.Errorf("expected returned task status %q, got %q", models.TaskStatusRunning, got.Status)
	}
	if got.StartedAt == nil {
		t.Error("expected returned task to have StartedAt set")
	}
	if launched == nil || launched.ID != task.ID {
		t.Fatalf("expected launch hook called with task %v, got %v", task.ID, launched)
	}
	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusRunning {
		t.Errorf("expected task running, got %s", reloaded.Status)
	}
}

func TestForceRunTask_RefusesNonQueued(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createProject(t, db, "force-nq")
	task := createTask(t, db, proj.ID, "done-task", models.TaskStatusDone)

	called := false
	r := newForceRunner(t, db)
	r.launchFn = func(*models.Task, *models.TaskExecution) bool { called = true; return true }

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrTaskNotQueued) {
		t.Fatalf("want ErrTaskNotQueued, got %v", err)
	}
	if called {
		t.Error("launch hook must not be called for a non-queued task")
	}
}

func TestForceRunTask_RefusesWhenStopped(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createProject(t, db, "force-stopped")
	task := createTask(t, db, proj.ID, "q", models.TaskStatusQueued)

	r := newForceRunner(t, db)
	r.state = models.StateStopped

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrRunnerStopped) {
		t.Fatalf("want ErrRunnerStopped, got %v", err)
	}
	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("task must stay queued, got %s", reloaded.Status)
	}
}

func TestForceRunTask_RefusesWhenWorkersBusy(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	projA := createProject(t, db, "force-a")
	projB := createProject(t, db, "force-b")
	taskA := createTask(t, db, projA.ID, "a", models.TaskStatusRunning)
	task := createTask(t, db, projB.ID, "b", models.TaskStatusQueued)

	r := newForceRunner(t, db)
	r.maxWorkers = 1
	r.executors[projA.ID] = &activeTask{task: &taskA, execution: &models.TaskExecution{TaskID: taskA.ID}}

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrWorkersBusy) {
		t.Fatalf("want ErrWorkersBusy, got %v", err)
	}
}

func TestForceRunTask_RefusesWhenProjectBusy(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createProject(t, db, "force-proj-busy")
	running := createTask(t, db, proj.ID, "running", models.TaskStatusRunning)
	task := createTask(t, db, proj.ID, "queued", models.TaskStatusQueued)

	r := newForceRunner(t, db)
	r.executors[proj.ID] = &activeTask{task: &running, execution: &models.TaskExecution{TaskID: running.ID}}

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrProjectBusy) {
		t.Fatalf("want ErrProjectBusy, got %v", err)
	}
}

// TestForceRunTask_RefusesWhenProjectBusyInDBOnly exercises the DB-level
// project-busy guard in pickTaskByID's NOT EXISTS subquery in isolation.
// r.executors is left empty on purpose, so the in-memory pre-flight check in
// ForceRunTask passes and the only thing standing between this force-run and
// a second running task on the project is the database. Every other
// ForceRunTask test short-circuits at the in-memory pre-flight, so without
// this test the NOT EXISTS clause could be deleted from pickTaskByID and
// nothing would fail — yet it's what stops a force-run from starting a second
// task on a project whose running task exists only in the DB (another
// process, or an orphan between claimTask and launchTask).
//
// pickTaskByID returning nil here is indistinguishable from a status change
// or a concurrent claim losing the race, so ForceRunTask reports it as
// ErrLaunchRace rather than ErrProjectBusy — that sentinel is reserved for
// the in-memory pre-flight finding the project busy (see
// TestForceRunTask_RefusesWhenProjectBusy).
func TestForceRunTask_RefusesWhenProjectBusyInDBOnly(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createProject(t, db, "force-proj-busy-db-only")
	createTask(t, db, proj.ID, "running", models.TaskStatusRunning)
	task := createTask(t, db, proj.ID, "queued", models.TaskStatusQueued)

	// executors intentionally left empty (default from newForceRunner) — the
	// running task above is known only to the database.
	r := newForceRunner(t, db)

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrLaunchRace) {
		t.Fatalf("want ErrLaunchRace, got %v", err)
	}

	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected task to remain queued, got %s", reloaded.Status)
	}
}

// TestForceRunTask_DoLaunchFalseIsTreatedAsFailure guards against ForceRunTask
// being "simplified" to ignore doLaunch's return value. A false return means
// launchTask (or the test's stand-in) has already unclaimed the task back to
// queued; if ForceRunTask stopped checking that return value, it would report
// success while the task silently sat back in queued and never ran. The
// error is ErrLaunchRace: doLaunch returning false means a worker or project
// slot was taken between the pre-flight check and the launch.
func TestForceRunTask_DoLaunchFalseIsTreatedAsFailure(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createProject(t, db, "force-launch-false")
	task := createTask(t, db, proj.ID, "queued", models.TaskStatusQueued)

	r := newForceRunner(t, db)
	r.launchFn = func(tk *models.Task, _ *models.TaskExecution) bool {
		// Mirror launchTask's real contract: a false return means the caller
		// has already unclaimed the task back to queued.
		r.unclaimTask(tk)
		return false
	}

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrLaunchRace) {
		t.Fatalf("want ErrLaunchRace, got %v", err)
	}

	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected task back to queued, got %s", reloaded.Status)
	}
}
