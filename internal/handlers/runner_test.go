package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"botka/internal/models"
	"botka/internal/runner"
)

func TestKillTask_InvalidID(t *testing.T) {
	router := gin.New()
	h := &RunnerHandler{} // nil runner — we only test parameter parsing
	router.POST("/api/v1/tasks/:id/kill", h.KillTask)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/not-a-uuid/kill", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// runnerStatusEnvelope mirrors the API response shape:
// { "data": runner.Status }.
type runnerStatusEnvelope struct {
	Data struct {
		PausedUntil *time.Time `json:"paused_until"`
		PauseReason *string    `json:"pause_reason"`
		PauseSource *string    `json:"pause_source"`
	} `json:"data"`
}

func TestRunnerStatus_NoPause(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := newTestRunner(t, db, usage, runner.NewRateLimitGate(nil))
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodGet, "/api/v1/runner/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var env runnerStatusEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.Data.PausedUntil != nil {
		t.Errorf("expected paused_until null, got %v", env.Data.PausedUntil)
	}
	if env.Data.PauseReason != nil {
		t.Errorf("expected pause_reason null, got %v", env.Data.PauseReason)
	}
	if env.Data.PauseSource != nil {
		t.Errorf("expected pause_source null, got %v", env.Data.PauseSource)
	}
}

func TestRunnerStatus_WhenPaused(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	gate := runner.NewRateLimitGate(nil)
	gate.PauseUntil(time.Now().Add(2*time.Hour), "Claude rate limit (task abc)", uuid.Nil)

	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := newTestRunner(t, db, usage, gate)
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodGet, "/api/v1/runner/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var env runnerStatusEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.Data.PausedUntil == nil {
		t.Fatal("expected paused_until non-null when gate is active")
	}
	if env.Data.PauseReason == nil || *env.Data.PauseReason == "" {
		t.Error("expected pause_reason to be populated")
	}
	if env.Data.PauseSource == nil || *env.Data.PauseSource != "rate_limit" {
		t.Errorf("expected pause_source rate_limit, got %v", env.Data.PauseSource)
	}
}

func TestClearRateLimit_ResetsGate(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	gate := runner.NewRateLimitGate(nil)
	gate.PauseUntil(time.Now().Add(1*time.Hour), "test", uuid.Nil)
	if !gate.IsActive() {
		t.Fatal("precondition failed: gate must be active before clear")
	}

	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := newTestRunner(t, db, usage, gate)
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodPost, "/api/v1/runner/clear-rate-limit", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if gate.IsActive() {
		t.Error("expected gate to be cleared after POST /runner/clear-rate-limit")
	}
}

func TestForceRun_InvalidID(t *testing.T) {
	router := gin.New()
	h := &RunnerHandler{} // nil runner — only parameter parsing is exercised
	router.POST("/api/v1/tasks/:id/force-run", h.ForceRun)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/not-a-uuid/force-run", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForceRun_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := runner.NewRunnerForTest(db, usage, runner.NewRateLimitGate(nil))
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/"+uuid.New().String()+"/force-run", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForceRun_NotQueued(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusDone)
	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := runner.NewRunnerForTest(db, usage, runner.NewRateLimitGate(nil))
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/"+task.ID.String()+"/force-run", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForceRun_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := runner.NewRunnerForTest(db, usage, runner.NewRateLimitGate(nil))
	r.SetLaunchHookForTest(func(*models.Task, *models.TaskExecution) bool { return true })
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/"+task.ID.String()+"/force-run", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusRunning {
		t.Errorf("expected running, got %s", reloaded.Status)
	}
}

// TestForceRun_LaunchRace covers the (c) cause of the ErrLaunchRace sentinel:
// the pre-flight capacity check passes but the launch itself loses the race
// (a worker or project slot was taken in the gap). This must map to 409 with
// the launch-race message, distinct from the "project busy" message that
// covers only the in-memory pre-flight finding the project busy up front.
func TestForceRun_LaunchRace(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := runner.NewRunnerForTest(db, usage, runner.NewRateLimitGate(nil))
	r.SetLaunchHookForTest(func(*models.Task, *models.TaskExecution) bool { return false })
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/"+task.ID.String()+"/force-run", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error != "could not launch the task right now; try again" {
		t.Errorf("unexpected error message: %q", body.Error)
	}
}

// newTestRunner builds a runner that's wired enough for HTTP-level status
// tests. It deliberately avoids the full NewRunner constructor (which requires
// a real claude binary on PATH) by using the runner package's testing helper.
func newTestRunner(t *testing.T, _ any, usage *runner.UsageMonitor, gate *runner.RateLimitGate) *runner.Runner {
	t.Helper()
	return runner.NewRunnerForTest(nil, usage, gate)
}

func newRunnerTestRouter(r *runner.Runner) *gin.Engine {
	router := gin.New()
	rg := router.Group("/api/v1")
	h := NewRunnerHandler(r)
	RegisterRunnerRoutes(rg, h)
	return router
}
