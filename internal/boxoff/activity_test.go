package boxoff

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"botka/internal/claude"
	"botka/internal/models"
	"botka/internal/runner"
)

// setupTestDB connects to botka_test and makes sure the two tables the
// Box-chat lookup joins exist. Tests skip when the database is unavailable, so
// `make test` passes on a machine without it.
//
// Nothing here truncates a table. botka_test is shared with the handlers
// package, whose tests run concurrently with these, so this helper only ever
// touches rows it created itself — a DELETE FROM threads here would fail a
// handler test three packages away.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_TEST_URL")
	if dsn == "" {
		dsn = "postgres://botka:botka@localhost:5432/botka_test?sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("botka_test unavailable: %v", err)
	}

	if err := db.AutoMigrate(&models.Project{}, &models.Persona{}, &models.Thread{}); err != nil {
		t.Skipf("botka_test schema unavailable: %v", err)
	}

	return db
}

// createThreadOnProject inserts a project and a thread pointing at it, and
// registers their removal. Both are addressed by primary key so concurrent
// tests in other packages are unaffected.
func createThreadOnProject(t *testing.T, db *gorm.DB, name, path, title string) models.Thread {
	t.Helper()

	project := models.Project{ID: uuid.New(), Name: name, Path: path}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project %s: %v", name, err)
	}

	thread := models.Thread{Title: title, ProjectID: &project.ID}
	if err := db.Create(&thread).Error; err != nil {
		t.Fatalf("create thread %s: %v", title, err)
	}

	t.Cleanup(func() {
		db.Delete(&models.Thread{}, thread.ID)
		db.Delete(&models.Project{}, "id = ?", project.ID)
	})

	return thread
}

type fakeTaskStatus struct{ status runner.Status }

func (f fakeTaskStatus) GetStatus() runner.Status { return f.status }

type fakeChatRegistry struct{ procs []claude.ProcessInfo }

func (f fakeChatRegistry) List() []claude.ProcessInfo { return f.procs }

func TestAppActivity_RunningTasks(t *testing.T) {
	act := NewAppActivity(fakeTaskStatus{status: runner.Status{
		ActiveTasks: []runner.ActiveTaskInfo{
			{TaskID: uuid.New(), TaskTitle: "fix the parser", ProjectName: "botka", StartedAt: time.Now()},
		},
	}}, fakeChatRegistry{}, nil)

	got := act.RunningTasks()
	if len(got) != 1 {
		t.Fatalf("RunningTasks() = %v, want one entry", got)
	}
	if got[0] == "" {
		t.Error("task label is empty; it is shown to the user as a blocker reason")
	}
}

func TestAppActivity_NoTasks(t *testing.T) {
	act := NewAppActivity(fakeTaskStatus{}, fakeChatRegistry{}, nil)

	if got := act.RunningTasks(); len(got) != 0 {
		t.Fatalf("RunningTasks() = %v, want empty", got)
	}
}

func TestAppActivity_BoxChatThreads_NoProcessesSkipsTheQuery(t *testing.T) {
	// db is nil on purpose: with an empty registry there is nothing to look up,
	// so touching the database at all would panic here.
	act := NewAppActivity(fakeTaskStatus{}, fakeChatRegistry{}, nil)

	got, err := act.BoxChatThreads(context.Background())
	if err != nil {
		t.Fatalf("BoxChatThreads() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("BoxChatThreads() = %v, want empty", got)
	}
}

func TestAppActivity_BoxChatThreads_WithoutDatabaseIsAnError(t *testing.T) {
	// A running chat whose location cannot be resolved must never read as
	// "no chats on box" — unknown is a blocker, not a licence to shut down.
	act := NewAppActivity(fakeTaskStatus{}, fakeChatRegistry{procs: []claude.ProcessInfo{
		{ThreadID: 1, ThreadTitle: "a chat"},
	}}, nil)

	if _, err := act.BoxChatThreads(context.Background()); err == nil {
		t.Fatal("BoxChatThreads() = nil error with no database, want a failure")
	}
}

func TestAppActivity_BoxChatThreads_OnlyBoxProjects(t *testing.T) {
	db := setupTestDB(t)

	suffix := uuid.NewString()
	localThread := createThreadOnProject(t, db, "local-"+suffix, "/home/pi/projects/local-"+suffix, "local chat")
	remoteThread := createThreadOnProject(t, db, "render-"+suffix, "box:/home/box/projects/render-"+suffix, "render chat")

	act := NewAppActivity(fakeTaskStatus{}, fakeChatRegistry{procs: []claude.ProcessInfo{
		{ThreadID: localThread.ID, ThreadTitle: "local chat"},
		{ThreadID: remoteThread.ID, ThreadTitle: "render chat"},
	}}, db)

	got, err := act.BoxChatThreads(context.Background())
	if err != nil {
		t.Fatalf("BoxChatThreads() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("BoxChatThreads() = %v, want only the box: thread", got)
	}
	if got[0] != "render chat" {
		t.Errorf("BoxChatThreads() = %v, want the remote thread's title", got)
	}
}
