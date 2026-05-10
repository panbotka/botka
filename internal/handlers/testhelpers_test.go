package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
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

func init() {
	gin.SetMode(gin.TestMode)
}

// setupTestDB connects to the botka_test database once per test run.
// It auto-migrates all models and returns the shared DB connection.
// Tests are skipped if the database is unavailable.
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
			// Ensure unaccent extension is available for diacritic-insensitive search.
			sharedDB.Exec("CREATE EXTENSION IF NOT EXISTS unaccent")
			// Drop all tables and recreate to avoid migration conflicts
			sharedDB.Exec("DROP TABLE IF EXISTS push_subscriptions, task_schedules, cron_executions, cron_jobs, thread_mcp_servers, project_mcp_servers, mcp_servers, thread_access, webauthn_credentials, sessions, users, thread_sources, signal_bridges, thread_tags, task_tag_assignments, task_tags, task_notes, branch_selections, attachments, messages, task_executions, tasks, threads, thread_folders, projects, personas, tags, memories, runner_state, fork_points CASCADE")
			dbErr = sharedDB.AutoMigrate(
				&models.Project{},
				&models.TaskSchedule{},
				&models.Task{},
				&models.TaskExecution{},
				&models.TaskTag{},
				&models.TaskTagAssignment{},
				&models.TaskNote{},
				&models.ThreadFolder{},
				&models.Thread{},
				&models.Message{},
				&models.Attachment{},
				&models.BranchSelection{},
				&models.Persona{},
				&models.Tag{},
				&models.Memory{},
				&models.User{},
				&models.Session{},
				&models.WebAuthnCredential{},
				&models.ThreadSource{},
				&models.SignalBridge{},
				&models.ThreadAccess{},
				&models.MCPServer{},
				&models.ThreadMCPServer{},
				&models.ProjectMCPServer{},
				&models.CronJob{},
				&models.CronExecution{},
				&models.PushSubscription{},
			)
			if dbErr == nil {
				// Create thread_tags join table
				sharedDB.Exec(`CREATE TABLE IF NOT EXISTS thread_tags (
					thread_id BIGINT NOT NULL,
					tag_id BIGINT NOT NULL,
					PRIMARY KEY (thread_id, tag_id)
				)`)
				// Case-insensitive uniqueness on task tag names (matches migration 029).
				sharedDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_task_tags_name_lower
					ON task_tags (LOWER(name))`)
				// task_tag_assignments mirrors migration 029: ON DELETE CASCADE
				// from both sides. AutoMigrate alone does not emit the FK
				// constraint, so we recreate the table here. The table is
				// otherwise empty (just-migrated) so dropping is safe.
				sharedDB.Exec(`DROP TABLE IF EXISTS task_tag_assignments`)
				sharedDB.Exec(`CREATE TABLE task_tag_assignments (
					task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
					tag_id BIGINT NOT NULL REFERENCES task_tags(id) ON DELETE CASCADE,
					PRIMARY KEY (task_id, tag_id)
				)`)
				sharedDB.Exec(`CREATE INDEX IF NOT EXISTS idx_task_tag_assignments_tag_id
					ON task_tag_assignments(tag_id)`)
				// Mirror migration 031: ON DELETE CASCADE on task_notes so
				// deleting a task removes its notes. AutoMigrate doesn't emit
				// the FK constraint, so we add it here.
				sharedDB.Exec(`ALTER TABLE task_notes
					DROP CONSTRAINT IF EXISTS fk_task_notes_task`)
				sharedDB.Exec(`ALTER TABLE task_notes
					ADD CONSTRAINT fk_task_notes_task
					FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE`)
				// Create runner_state manually (GORM struggles with default:1 PK)
				sharedDB.Exec(`CREATE TABLE IF NOT EXISTS runner_state (
					id INTEGER PRIMARY KEY DEFAULT 1,
					state TEXT NOT NULL DEFAULT 'stopped',
					completed_count INTEGER NOT NULL DEFAULT 0,
					task_limit INTEGER,
					updated_at TIMESTAMPTZ
				)`)
				// Create app_settings for server-side configuration.
				sharedDB.Exec(`CREATE TABLE IF NOT EXISTS app_settings (
					key VARCHAR(100) PRIMARY KEY,
					value TEXT NOT NULL,
					updated_at TIMESTAMPTZ
				)`)
				// One running task per project constraint.
				sharedDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_one_running_per_project
					ON tasks (project_id) WHERE status = 'running'`)
				// Mirror migration 030: full-text search column, GIN index, and
				// auto-update trigger. The trigger keeps search_vector in sync
				// with title/spec/failure_summary on every insert and update.
				sharedDB.Exec(`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS search_vector tsvector`)
				sharedDB.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_search
					ON tasks USING GIN (search_vector)`)
				sharedDB.Exec(`DROP TRIGGER IF EXISTS tasks_search_vector_update ON tasks`)
				sharedDB.Exec(`CREATE TRIGGER tasks_search_vector_update
					BEFORE INSERT OR UPDATE OF title, spec, failure_summary ON tasks
					FOR EACH ROW EXECUTE FUNCTION
					tsvector_update_trigger(search_vector, 'pg_catalog.simple', title, spec, failure_summary)`)
				// Mirror migration 034: messages full-text search column with
				// 'simple' config (mixed-language content) and GIN index.
				// AutoMigrate does not emit GENERATED ALWAYS columns, so we
				// add it manually.
				sharedDB.Exec(`ALTER TABLE messages DROP COLUMN IF EXISTS search_vector`)
				sharedDB.Exec(`ALTER TABLE messages ADD COLUMN search_vector tsvector
					GENERATED ALWAYS AS (to_tsvector('pg_catalog.simple', content)) STORED`)
				sharedDB.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_search
					ON messages USING GIN (search_vector)`)
				// Partial unique index on branch_selections — non-deleted rows only.
				sharedDB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_branch_thread_fork
					ON branch_selections (thread_id, fork_message_id)
					WHERE deleted_at IS NULL`)
			}
		}
	})
	if dbErr != nil {
		t.Skipf("test database unavailable: %v", dbErr)
	}
	return sharedDB
}

// cleanTables truncates all tables in FK-safe order.
func cleanTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	db.Exec("TRUNCATE TABLE push_subscriptions, task_schedules, cron_executions, cron_jobs, thread_mcp_servers, project_mcp_servers, mcp_servers, thread_access, webauthn_credentials, sessions, users, thread_sources, signal_bridges, thread_tags, task_tag_assignments, task_tags, task_notes, branch_selections, attachments, messages, task_executions, tasks, threads, thread_folders, projects, personas, tags, memories, runner_state, app_settings CASCADE")
}

// createTestProject creates and returns a test project.
func createTestProject(t *testing.T, db *gorm.DB) models.Project {
	t.Helper()
	p := models.Project{
		Name:           "test-project",
		Path:           "/tmp/test-project-" + uuid.New().String()[:8],
		BranchStrategy: "main",
		Active:         true,
	}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("create test project: %v", err)
	}
	return p
}

// createTestThread creates and returns a test thread.
func createTestThread(t *testing.T, db *gorm.DB) models.Thread {
	t.Helper()
	model := "sonnet"
	th := models.Thread{
		Title: "test thread",
		Model: &model,
	}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create test thread: %v", err)
	}
	return th
}

// createTestTask creates and returns a test task for the given project.
func createTestTask(t *testing.T, db *gorm.DB, projectID uuid.UUID, status models.TaskStatus) models.Task {
	t.Helper()
	task := models.Task{
		Title:     "test task",
		Spec:      "test spec",
		ProjectID: projectID,
		Status:    status,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create test task: %v", err)
	}
	return task
}

// doRequest performs an HTTP request against the given router and returns the recorder.
func doRequest(router *gin.Engine, method, path string, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
