package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

// taskStatsRouter wires the /stats/tasks endpoint against the test database.
func taskStatsRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	r := gin.New()
	h := NewTaskStatsHandler(db)
	v1 := r.Group("/api/v1")
	RegisterTaskStatsRoutes(v1, h)
	return r
}

// taskStatsResp mirrors the response envelope for decoding.
type taskStatsResp struct {
	Data struct {
		From    string `json:"from"`
		To      string `json:"to"`
		GroupBy string `json:"group_by"`
		Buckets []struct {
			Day                 *string `json:"day"`
			Project             *string `json:"project"`
			ProjectID           *string `json:"project_id"`
			Model               *string `json:"model"`
			TaskCount           int64   `json:"task_count"`
			InputTokens         int64   `json:"input_tokens"`
			OutputTokens        int64   `json:"output_tokens"`
			CacheReadTokens     int64   `json:"cache_read_tokens"`
			CacheCreationTokens int64   `json:"cache_creation_tokens"`
			CostUSD             float64 `json:"cost_usd"`
		} `json:"buckets"`
	} `json:"data"`
}

func TestTaskStats_Empty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskStatsRouter(t, db)
	w := doRequest(r, http.MethodGet, "/api/v1/stats/tasks", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp taskStatsResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.GroupBy != "day" {
		t.Errorf("group_by = %q, want %q", resp.Data.GroupBy, "day")
	}
	if len(resp.Data.Buckets) != 0 {
		t.Errorf("expected empty buckets, got %d", len(resp.Data.Buckets))
	}
}

func TestTaskStats_InvalidGroupBy(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskStatsRouter(t, db)
	w := doRequest(r, http.MethodGet, "/api/v1/stats/tasks?group_by=user", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid group_by, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskStats_InvalidDate(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskStatsRouter(t, db)
	w := doRequest(r, http.MethodGet, "/api/v1/stats/tasks?from=not-a-date", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid from date, got %d: %s", w.Code, w.Body.String())
	}
}

// TestTaskStats_GroupByProject seeds two completed tasks across two projects
// and verifies the per-project rollup totals match.
func TestTaskStats_GroupByProject(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	projA := createTestProject(t, db)
	projB := createTestProject(t, db)

	now := time.Now().UTC()
	mustCreateCompletedTask(t, db, projA.ID, now, taskUsage{
		input: 1000, output: 500, cacheRead: 200, cacheCreation: 100, cost: 0.05, model: "sonnet",
	})
	mustCreateCompletedTask(t, db, projA.ID, now, taskUsage{
		input: 200, output: 100, cost: 0.02, model: "sonnet",
	})
	mustCreateCompletedTask(t, db, projB.ID, now, taskUsage{
		input: 50, output: 25, cost: 0.01, model: "haiku",
	})

	r := taskStatsRouter(t, db)
	w := doRequest(r, http.MethodGet, "/api/v1/stats/tasks?group_by=project", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp taskStatsResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.GroupBy != "project" {
		t.Errorf("group_by = %q, want %q", resp.Data.GroupBy, "project")
	}
	if len(resp.Data.Buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(resp.Data.Buckets))
	}

	// Buckets are sorted by cost_usd DESC, so projA (cost=0.07) comes first.
	first := resp.Data.Buckets[0]
	if first.ProjectID == nil || *first.ProjectID != projA.ID.String() {
		t.Errorf("first bucket project_id = %v, want %s", first.ProjectID, projA.ID)
	}
	if first.TaskCount != 2 {
		t.Errorf("projA task_count = %d, want 2", first.TaskCount)
	}
	if first.InputTokens != 1200 {
		t.Errorf("projA input_tokens = %d, want 1200", first.InputTokens)
	}
	if first.OutputTokens != 600 {
		t.Errorf("projA output_tokens = %d, want 600", first.OutputTokens)
	}
	if diff := first.CostUSD - 0.07; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("projA cost_usd = %f, want ~0.07", first.CostUSD)
	}
}

// TestTaskStats_GroupByModel verifies the per-model rollup, including the
// COALESCE(model, 'unknown') fallback for tasks whose model wasn't captured.
func TestTaskStats_GroupByModel(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	now := time.Now().UTC()

	mustCreateCompletedTask(t, db, proj.ID, now, taskUsage{
		input: 1000, output: 500, cost: 0.10, model: "opus",
	})
	mustCreateCompletedTask(t, db, proj.ID, now, taskUsage{
		input: 200, output: 100, cost: 0.01, model: "haiku",
	})
	mustCreateCompletedTask(t, db, proj.ID, now, taskUsage{
		input: 50, output: 25, cost: 0.005, // no model — should land in 'unknown'
	})

	r := taskStatsRouter(t, db)
	w := doRequest(r, http.MethodGet, "/api/v1/stats/tasks?group_by=model", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp taskStatsResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data.Buckets) != 3 {
		t.Fatalf("expected 3 model buckets, got %d", len(resp.Data.Buckets))
	}

	models := make(map[string]int64)
	for _, b := range resp.Data.Buckets {
		if b.Model == nil {
			t.Fatalf("bucket missing model field")
		}
		models[*b.Model] = b.TaskCount
	}
	if models["opus"] != 1 || models["haiku"] != 1 || models["unknown"] != 1 {
		t.Errorf("model counts = %v, expected one each of opus/haiku/unknown", models)
	}
}

// TestTaskStats_GroupByDayProject exercises the composite shape used by the
// stacked-bar chart on the frontend.
func TestTaskStats_GroupByDayProject(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	now := time.Now().UTC()

	mustCreateCompletedTask(t, db, proj.ID, now, taskUsage{
		input: 100, output: 50, cost: 0.01, model: "sonnet",
	})

	r := taskStatsRouter(t, db)
	w := doRequest(r, http.MethodGet, "/api/v1/stats/tasks?group_by=day,project", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp taskStatsResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.GroupBy != "day,project" {
		t.Errorf("group_by = %q, want day,project", resp.Data.GroupBy)
	}
	if len(resp.Data.Buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(resp.Data.Buckets))
	}
	b := resp.Data.Buckets[0]
	if b.Day == nil || b.Project == nil {
		t.Fatalf("bucket should populate both day and project, got day=%v project=%v", b.Day, b.Project)
	}
}

// taskUsage bundles the parameters for a seeded completed task.
type taskUsage struct {
	input, output, cacheRead, cacheCreation int64
	cost                                    float64
	model                                   string
}

// mustCreateCompletedTask seeds a task in `done` state with the given token
// counts and completed_at timestamp. The task is later picked up by
// /stats/tasks queries that filter on completed_at.
func mustCreateCompletedTask(
	t *testing.T, db *gorm.DB, projectID uuid.UUID, completedAt time.Time, u taskUsage,
) {
	t.Helper()
	task := models.Task{
		Title:               "stats fixture",
		Spec:                "fixture",
		Status:              models.TaskStatusDone,
		ProjectID:           projectID,
		InputTokens:         &u.input,
		OutputTokens:        &u.output,
		CacheReadTokens:     &u.cacheRead,
		CacheCreationTokens: &u.cacheCreation,
		CostUSD:             &u.cost,
		CompletedAt:         &completedAt,
	}
	if u.model != "" {
		m := u.model
		task.Model = &m
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create completed task: %v", err)
	}
}
