package runner

import (
	"os"
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
			sharedDB.Exec("DROP TABLE IF EXISTS task_executions, tasks, projects, runner_state CASCADE")
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
	r.launchTask(&task2, &models.TaskExecution{TaskID: task2.ID})

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
