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
			TotalCostUSD      float64 `json:"total_cost_usd"`
			TotalInputTokens  int64   `json:"total_input_tokens"`
			TotalOutputTokens int64   `json:"total_output_tokens"`
			ByDate            []struct {
				Date    string                            `json:"date"`
				CostUSD float64                           `json:"cost_usd"`
				ByModel map[string]map[string]interface{} `json:"by_model"`
			} `json:"by_date"`
			ByModel   []interface{} `json:"by_model"`
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
	if resp.Data.TotalInputTokens != 0 {
		t.Errorf("expected total_input_tokens 0, got %d", resp.Data.TotalInputTokens)
	}
	if resp.Data.TotalOutputTokens != 0 {
		t.Errorf("expected total_output_tokens 0, got %d", resp.Data.TotalOutputTokens)
	}
	if len(resp.Data.ByModel) != 0 {
		t.Errorf("expected empty by_model, got %d", len(resp.Data.ByModel))
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

	// Assign project and model to thread
	db.Model(&th).Update("project_id", projID)
	db.Model(&th).Update("model", "sonnet")

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
			TotalCostUSD      float64 `json:"total_cost_usd"`
			TotalInputTokens  int64   `json:"total_input_tokens"`
			TotalOutputTokens int64   `json:"total_output_tokens"`
			ByDate            []struct {
				Date         string  `json:"date"`
				CostUSD      float64 `json:"cost_usd"`
				InputTokens  int64   `json:"input_tokens"`
				OutputTokens int64   `json:"output_tokens"`
				ByModel      map[string]struct {
					Input  int64 `json:"input"`
					Output int64 `json:"output"`
				} `json:"by_model"`
			} `json:"by_date"`
			ByModel []struct {
				Model        string  `json:"model"`
				InputTokens  int64   `json:"input_tokens"`
				OutputTokens int64   `json:"output_tokens"`
				CostUSD      float64 `json:"cost_usd"`
				MessageCount int64   `json:"message_count"`
			} `json:"by_model"`
			ByThread []struct {
				ThreadID     int64   `json:"thread_id"`
				Title        string  `json:"title"`
				CostUSD      float64 `json:"cost_usd"`
				InputTokens  int64   `json:"input_tokens"`
				OutputTokens int64   `json:"output_tokens"`
			} `json:"by_thread"`
			ByProject []struct {
				ProjectName  string  `json:"project_name"`
				CostUSD      float64 `json:"cost_usd"`
				InputTokens  int64   `json:"input_tokens"`
				OutputTokens int64   `json:"output_tokens"`
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

	// Token totals: 100+200=300 input, 50+100=150 output
	if resp.Data.TotalInputTokens != 300 {
		t.Errorf("expected total_input_tokens 300, got %d", resp.Data.TotalInputTokens)
	}
	if resp.Data.TotalOutputTokens != 150 {
		t.Errorf("expected total_output_tokens 150, got %d", resp.Data.TotalOutputTokens)
	}

	// by_model should have "sonnet"
	if len(resp.Data.ByModel) != 1 {
		t.Errorf("expected 1 model in by_model, got %d", len(resp.Data.ByModel))
	} else {
		m := resp.Data.ByModel[0]
		if m.Model != "sonnet" {
			t.Errorf("expected model 'sonnet', got %q", m.Model)
		}
		if m.InputTokens != 300 {
			t.Errorf("expected input_tokens 300, got %d", m.InputTokens)
		}
		if m.OutputTokens != 150 {
			t.Errorf("expected output_tokens 150, got %d", m.OutputTokens)
		}
		if m.MessageCount != 2 {
			t.Errorf("expected message_count 2, got %d", m.MessageCount)
		}
	}

	// by_date should have entries
	if len(resp.Data.ByDate) == 0 {
		t.Error("expected non-empty by_date")
	}

	// Find today's entry and check by_model breakdown
	today := time.Now().Format("2006-01-02")
	var foundToday bool
	for _, d := range resp.Data.ByDate {
		if d.Date == today {
			foundToday = true
			if bm, ok := d.ByModel["sonnet"]; ok {
				if bm.Input != 300 {
					t.Errorf("expected date by_model sonnet input 300, got %d", bm.Input)
				}
				if bm.Output != 150 {
					t.Errorf("expected date by_model sonnet output 150, got %d", bm.Output)
				}
			} else {
				t.Error("expected by_model to contain 'sonnet' for today")
			}
			break
		}
	}
	if !foundToday {
		t.Error("expected to find today's date in by_date")
	}

	// by_thread should have the test thread with tokens
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
		if resp.Data.ByThread[0].InputTokens != 300 {
			t.Errorf("expected thread input_tokens 300, got %d", resp.Data.ByThread[0].InputTokens)
		}
		if resp.Data.ByThread[0].OutputTokens != 150 {
			t.Errorf("expected thread output_tokens 150, got %d", resp.Data.ByThread[0].OutputTokens)
		}
	}

	// by_project should have the test project with tokens
	if len(resp.Data.ByProject) != 1 {
		t.Errorf("expected 1 project in by_project, got %d", len(resp.Data.ByProject))
	} else {
		// Project cost = exec (0.25) + chat (0.15) = 0.40
		projCost := 0.40
		if diff := resp.Data.ByProject[0].CostUSD - projCost; diff > 0.001 || diff < -0.001 {
			t.Errorf("expected project cost ~%.2f, got %.6f", projCost, resp.Data.ByProject[0].CostUSD)
		}
		// Tokens come only from messages (task executions don't have tokens)
		if resp.Data.ByProject[0].InputTokens != 300 {
			t.Errorf("expected project input_tokens 300, got %d", resp.Data.ByProject[0].InputTokens)
		}
		if resp.Data.ByProject[0].OutputTokens != 150 {
			t.Errorf("expected project output_tokens 150, got %d", resp.Data.ByProject[0].OutputTokens)
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

func TestCostAnalytics_MultipleModels(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Create two threads with different models
	th1 := createTestThread(t, db)
	db.Model(&th1).Update("model", "opus")

	th2 := createTestThread(t, db)
	db.Model(&th2).Update("model", "haiku")

	cost1 := 0.50
	prompt1 := 1000
	comp1 := 500
	cost2 := 0.01
	prompt2 := 50
	comp2 := 10

	msgs := []models.Message{
		{ThreadID: th1.ID, Role: "assistant", Content: "opus msg", CostUSD: &cost1, PromptTokens: &prompt1, CompletionTokens: &comp1, CreatedAt: time.Now()},
		{ThreadID: th2.ID, Role: "assistant", Content: "haiku msg", CostUSD: &cost2, PromptTokens: &prompt2, CompletionTokens: &comp2, CreatedAt: time.Now()},
	}
	for i := range msgs {
		if err := db.Create(&msgs[i]).Error; err != nil {
			t.Fatalf("create message: %v", err)
		}
	}

	r := analyticsRouterWithDB(t)
	w := doRequest(r, http.MethodGet, "/api/v1/analytics/cost?days=7", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			TotalInputTokens  int64 `json:"total_input_tokens"`
			TotalOutputTokens int64 `json:"total_output_tokens"`
			ByModel           []struct {
				Model        string `json:"model"`
				InputTokens  int64  `json:"input_tokens"`
				OutputTokens int64  `json:"output_tokens"`
				MessageCount int64  `json:"message_count"`
			} `json:"by_model"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Totals: 1000+50=1050 input, 500+10=510 output
	if resp.Data.TotalInputTokens != 1050 {
		t.Errorf("expected total_input_tokens 1050, got %d", resp.Data.TotalInputTokens)
	}
	if resp.Data.TotalOutputTokens != 510 {
		t.Errorf("expected total_output_tokens 510, got %d", resp.Data.TotalOutputTokens)
	}

	// Should have 2 models
	if len(resp.Data.ByModel) != 2 {
		t.Fatalf("expected 2 models in by_model, got %d", len(resp.Data.ByModel))
	}

	modelMap := make(map[string]struct {
		InputTokens  int64
		OutputTokens int64
		MessageCount int64
	})
	for _, m := range resp.Data.ByModel {
		modelMap[m.Model] = struct {
			InputTokens  int64
			OutputTokens int64
			MessageCount int64
		}{m.InputTokens, m.OutputTokens, m.MessageCount}
	}

	if opus, ok := modelMap["opus"]; !ok {
		t.Error("expected 'opus' in by_model")
	} else {
		if opus.InputTokens != 1000 {
			t.Errorf("expected opus input_tokens 1000, got %d", opus.InputTokens)
		}
		if opus.OutputTokens != 500 {
			t.Errorf("expected opus output_tokens 500, got %d", opus.OutputTokens)
		}
	}

	if haiku, ok := modelMap["haiku"]; !ok {
		t.Error("expected 'haiku' in by_model")
	} else {
		if haiku.InputTokens != 50 {
			t.Errorf("expected haiku input_tokens 50, got %d", haiku.InputTokens)
		}
		if haiku.OutputTokens != 10 {
			t.Errorf("expected haiku output_tokens 10, got %d", haiku.OutputTokens)
		}
	}
}
