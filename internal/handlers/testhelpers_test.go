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
			// Drop all tables and recreate to avoid migration conflicts
			sharedDB.Exec("DROP TABLE IF EXISTS thread_tags, branch_selections, attachments, messages, task_executions, tasks, threads, projects, personas, tags, memories, runner_state, fork_points CASCADE")
			dbErr = sharedDB.AutoMigrate(
				&models.Project{},
				&models.Task{},
				&models.TaskExecution{},
				&models.Thread{},
				&models.Message{},
				&models.Attachment{},
				&models.BranchSelection{},
				&models.Persona{},
				&models.Tag{},
				&models.Memory{},
			)
			if dbErr == nil {
				// Create thread_tags join table
				sharedDB.Exec(`CREATE TABLE IF NOT EXISTS thread_tags (
					thread_id BIGINT NOT NULL,
					tag_id BIGINT NOT NULL,
					PRIMARY KEY (thread_id, tag_id)
				)`)
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
	db.Exec("TRUNCATE TABLE thread_tags, branch_selections, attachments, messages, task_executions, tasks, threads, projects, personas, tags, memories, runner_state, app_settings CASCADE")
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
