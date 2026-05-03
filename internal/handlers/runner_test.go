package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
