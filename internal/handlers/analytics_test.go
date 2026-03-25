package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"botka/internal/models"
)

func analyticsRouterWithDB(t *testing.T) *gin.Engine {
	db := setupTestDB(t)
	r := gin.New()
	h := NewAnalyticsHandler(db)
	v1 := r.Group("/api/v1")
	RegisterAnalyticsRoutes(v1, h)
	return r
}

func TestCostAnalytics_Empty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := analyticsRouterWithDB(t)
	w := doRequest(r, http.MethodGet, "/api/v1/analytics/cost", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			TotalCostUSD float64 `json:"total_cost_usd"`
			ByDate       []struct {
				Date    string  `json:"date"`
				CostUSD float64 `json:"cost_usd"`
			} `json:"by_date"`
			ByThread  []interface{} `json:"by_thread"`
			ByProject []interface{} `json:"by_project"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.TotalCostUSD != 0 {
		t.Errorf("expected total_cost_usd 0, got %f", resp.Data.TotalCostUSD)
	}
	if len(resp.Data.ByThread) != 0 {
		t.Errorf("expected empty by_thread, got %d", len(resp.Data.ByThread))
	}
	if len(resp.Data.ByProject) != 0 {
		t.Errorf("expected empty by_project, got %d", len(resp.Data.ByProject))
	}
}

func TestCostAnalytics_WithData(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Create a project and thread
	proj := createTestProject(t, db)
	projID := proj.ID
	th := createTestThread(t, db)

	// Assign project to thread
	db.Model(&th).Update("project_id", projID)

	// Create messages with costs
	cost1 := 0.05
	cost2 := 0.10
	prompt1 := 100
	comp1 := 50
	prompt2 := 200
	comp2 := 100
	msgs := []models.Message{
		{ThreadID: th.ID, Role: "assistant", Content: "hello", CostUSD: &cost1, PromptTokens: &prompt1, CompletionTokens: &comp1, CreatedAt: time.Now()},
		{ThreadID: th.ID, Role: "assistant", Content: "world", CostUSD: &cost2, PromptTokens: &prompt2, CompletionTokens: &comp2, CreatedAt: time.Now()},
	}
	for i := range msgs {
		if err := db.Create(&msgs[i]).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	// Create a task with execution cost
	task := createTestTask(t, db, projID, models.TaskStatusDone)
	execCost := 0.25
	exec := models.TaskExecution{
		ID:        uuid.New(),
		TaskID:    task.ID,
		Attempt:   1,
		StartedAt: time.Now(),
		CostUSD:   &execCost,
	}
	if err := db.Create(&exec).Error; err != nil {
		t.Fatalf("create execution: %v", err)
	}

	r := analyticsRouterWithDB(t)
	w := doRequest(r, http.MethodGet, "/api/v1/analytics/cost?days=7", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			TotalCostUSD float64 `json:"total_cost_usd"`
			ByDate       []struct {
				Date         string  `json:"date"`
				CostUSD      float64 `json:"cost_usd"`
				InputTokens  int64   `json:"input_tokens"`
				OutputTokens int64   `json:"output_tokens"`
			} `json:"by_date"`
			ByThread []struct {
				ThreadID int64   `json:"thread_id"`
				Title    string  `json:"title"`
				CostUSD  float64 `json:"cost_usd"`
			} `json:"by_thread"`
			ByProject []struct {
				ProjectName string  `json:"project_name"`
				CostUSD     float64 `json:"cost_usd"`
			} `json:"by_project"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Total: 0.05 + 0.10 + 0.25 = 0.40
	expectedTotal := 0.40
	if diff := resp.Data.TotalCostUSD - expectedTotal; diff > 0.001 || diff < -0.001 {
		t.Errorf("expected total_cost_usd ~%.2f, got %.6f", expectedTotal, resp.Data.TotalCostUSD)
	}

	// by_date should have entries
	if len(resp.Data.ByDate) == 0 {
		t.Error("expected non-empty by_date")
	}

	// by_thread should have the test thread
	if len(resp.Data.ByThread) != 1 {
		t.Errorf("expected 1 thread in by_thread, got %d", len(resp.Data.ByThread))
	} else {
		if resp.Data.ByThread[0].ThreadID != th.ID {
			t.Errorf("expected thread_id %d, got %d", th.ID, resp.Data.ByThread[0].ThreadID)
		}
		threadCost := 0.15
		if diff := resp.Data.ByThread[0].CostUSD - threadCost; diff > 0.001 || diff < -0.001 {
			t.Errorf("expected thread cost ~%.2f, got %.6f", threadCost, resp.Data.ByThread[0].CostUSD)
		}
	}

	// by_project should have the test project
	if len(resp.Data.ByProject) != 1 {
		t.Errorf("expected 1 project in by_project, got %d", len(resp.Data.ByProject))
	} else {
		// Project cost = exec (0.25) + chat (0.15) = 0.40
		projCost := 0.40
		if diff := resp.Data.ByProject[0].CostUSD - projCost; diff > 0.001 || diff < -0.001 {
			t.Errorf("expected project cost ~%.2f, got %.6f", projCost, resp.Data.ByProject[0].CostUSD)
		}
	}
}

func TestCostAnalytics_DefaultDays(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := analyticsRouterWithDB(t)

	// No days param = default 30
	w := doRequest(r, http.MethodGet, "/api/v1/analytics/cost", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ByDate []struct{ Date string } `json:"by_date"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Should have ~31 date entries (30 days back + today)
	if len(resp.Data.ByDate) < 30 {
		t.Errorf("expected at least 30 date entries for default 30 days, got %d", len(resp.Data.ByDate))
	}
}

func TestCostAnalytics_InvalidDays(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := analyticsRouterWithDB(t)

	// Invalid days should default to 30
	w := doRequest(r, http.MethodGet, "/api/v1/analytics/cost?days=abc", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
