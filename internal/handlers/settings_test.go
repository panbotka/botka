package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"botka/internal/models"
)

func TestSettings_Get(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Seed a max_workers setting.
	db.Save(&models.Setting{Key: "max_workers", Value: "2"})

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodGet, "/api/v1/settings", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			MaxWorkers int `json:"max_workers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.MaxWorkers != 2 {
		t.Errorf("expected max_workers=2, got %d", resp.Data.MaxWorkers)
	}
}

func TestSettings_UpdateMaxWorkers(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodPut, "/api/v1/settings", `{"max_workers": 5}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			MaxWorkers int `json:"max_workers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.MaxWorkers != 5 {
		t.Errorf("expected max_workers=5, got %d", resp.Data.MaxWorkers)
	}
}

func TestSettings_UpdateMaxWorkers_InvalidLow(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodPut, "/api/v1/settings", `{"max_workers": 0}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSettings_UpdateMaxWorkers_InvalidHigh(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodPut, "/api/v1/settings", `{"max_workers": 11}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSettings_SetOnChange(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	var calledKey, calledValue string
	h.SetOnChange(func(key, value string) {
		calledKey = key
		calledValue = value
	})

	w := doRequest(router, http.MethodPut, "/api/v1/settings", `{"max_workers": 3}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if calledKey != "max_workers" {
		t.Errorf("expected onChange key=max_workers, got %q", calledKey)
	}
	if calledValue != "3" {
		t.Errorf("expected onChange value=3, got %q", calledValue)
	}
}

func TestSettings_UpdateInvalidBody(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodPut, "/api/v1/settings", `{invalid json}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSettings_PurgeTaskOutputs(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusDone)

	// Create executions with raw_output set.
	output1 := "some output"
	output2 := "more output"
	exec1 := models.TaskExecution{TaskID: task.ID, Attempt: 1, StartedAt: task.CreatedAt, RawOutput: &output1}
	exec2 := models.TaskExecution{TaskID: task.ID, Attempt: 2, StartedAt: task.CreatedAt, RawOutput: &output2}
	exec3 := models.TaskExecution{TaskID: task.ID, Attempt: 3, StartedAt: task.CreatedAt, RawOutput: nil}
	db.Create(&exec1)
	db.Create(&exec2)
	db.Create(&exec3)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodDelete, "/api/v1/settings/task-outputs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Purged int64 `json:"purged"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Purged != 2 {
		t.Errorf("expected purged=2, got %d", resp.Data.Purged)
	}

	// Verify raw_output is now NULL for all executions.
	var count int64
	db.Model(&models.TaskExecution{}).Where("raw_output IS NOT NULL").Count(&count)
	if count != 0 {
		t.Errorf("expected 0 rows with raw_output, got %d", count)
	}
}

func TestSettings_PurgeTaskOutputs_NoneToDelete(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodDelete, "/api/v1/settings/task-outputs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Purged int64 `json:"purged"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Purged != 0 {
		t.Errorf("expected purged=0, got %d", resp.Data.Purged)
	}
}
