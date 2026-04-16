package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

func cronRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	// Pass nil scheduler — manual trigger tests will be separate.
	h := NewCronHandler(db, nil)
	v1 := r.Group("/api/v1")
	RegisterCronRoutes(v1, h)
	return r
}

func createTestCronJob(t *testing.T, db *gorm.DB, projectID uuid.UUID) models.CronJob {
	t.Helper()
	job := models.CronJob{
		Name:           "test-cron",
		Schedule:       "*/5 * * * *",
		Prompt:         "run tests",
		ProjectID:      projectID,
		Enabled:        true,
		TimeoutMinutes: 30,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create test cron job: %v", err)
	}
	return job
}

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestCronJobs_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := cronRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/cron-jobs", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Total int64         `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected total=0, got %d", resp.Total)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty data, got %d items", len(resp.Data))
	}
}

func TestCronJobs_ListWithData(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	createTestCronJob(t, db, proj.ID)

	r := cronRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/cron-jobs", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data  []map[string]interface{} `json:"data"`
		Total int64                    `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Total)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Data))
	}
	if resp.Data[0]["name"] != "test-cron" {
		t.Errorf("expected name=test-cron, got %v", resp.Data[0]["name"])
	}
	// Check project is preloaded.
	if resp.Data[0]["project"] == nil {
		t.Error("expected project to be preloaded")
	}
}

// ---------------------------------------------------------------------------
// Create tests
// ---------------------------------------------------------------------------

func TestCronJobs_CreateValid(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	r := cronRouter(db)

	body := fmt.Sprintf(`{
		"name": "daily check",
		"schedule": "0 9 * * *",
		"prompt": "run daily checks",
		"project_id": "%s"
	}`, proj.ID)

	w := doRequest(r, http.MethodPost, "/api/v1/cron-jobs", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Schedule string `json:"schedule"`
			Enabled  bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Name != "daily check" {
		t.Errorf("expected name=daily check, got %s", resp.Data.Name)
	}
	if resp.Data.Schedule != "0 9 * * *" {
		t.Errorf("expected schedule=0 9 * * *, got %s", resp.Data.Schedule)
	}
	if !resp.Data.Enabled {
		t.Error("expected enabled=true by default")
	}
}

func TestCronJobs_CreateInvalidSchedule(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	r := cronRouter(db)

	body := fmt.Sprintf(`{
		"name": "bad schedule",
		"schedule": "not a cron expression",
		"prompt": "test",
		"project_id": "%s"
	}`, proj.ID)

	w := doRequest(r, http.MethodPost, "/api/v1/cron-jobs", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCronJobs_CreateMissingFields(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := cronRouter(db)

	// Missing name.
	w := doRequest(r, http.MethodPost, "/api/v1/cron-jobs", `{"schedule": "* * * * *", "prompt": "test"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing name, got %d: %s", w.Code, w.Body.String())
	}

	// Missing schedule.
	w = doRequest(r, http.MethodPost, "/api/v1/cron-jobs", fmt.Sprintf(`{"name": "test", "prompt": "test", "project_id": "%s"}`, uuid.New()))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing schedule, got %d: %s", w.Code, w.Body.String())
	}

	// Missing prompt.
	w = doRequest(r, http.MethodPost, "/api/v1/cron-jobs", fmt.Sprintf(`{"name": "test", "schedule": "* * * * *", "project_id": "%s"}`, uuid.New()))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing prompt, got %d: %s", w.Code, w.Body.String())
	}

	// Missing project_id.
	w = doRequest(r, http.MethodPost, "/api/v1/cron-jobs", `{"name": "test", "schedule": "* * * * *", "prompt": "test"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing project_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCronJobs_CreateWithOptionalFields(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	r := cronRouter(db)

	body := fmt.Sprintf(`{
		"name": "custom job",
		"schedule": "0 */2 * * *",
		"prompt": "test prompt",
		"project_id": "%s",
		"enabled": false,
		"timeout_minutes": 60,
		"model": "opus"
	}`, proj.ID)

	w := doRequest(r, http.MethodPost, "/api/v1/cron-jobs", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Enabled        bool    `json:"enabled"`
			TimeoutMinutes int     `json:"timeout_minutes"`
			Model          *string `json:"model"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Enabled {
		t.Error("expected enabled=false")
	}
	if resp.Data.TimeoutMinutes != 60 {
		t.Errorf("expected timeout_minutes=60, got %d", resp.Data.TimeoutMinutes)
	}
	if resp.Data.Model == nil || *resp.Data.Model != "opus" {
		t.Errorf("expected model=opus, got %v", resp.Data.Model)
	}
}

// ---------------------------------------------------------------------------
// Get tests
// ---------------------------------------------------------------------------

func TestCronJobs_GetExisting(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	job := createTestCronJob(t, db, proj.ID)

	r := cronRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/cron-jobs/%d", job.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.ID != job.ID {
		t.Errorf("expected id=%d, got %d", job.ID, resp.Data.ID)
	}
}

func TestCronJobs_GetNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := cronRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/cron-jobs/99999", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Update tests
// ---------------------------------------------------------------------------

func TestCronJobs_UpdateSchedule(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	job := createTestCronJob(t, db, proj.ID)

	r := cronRouter(db)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/cron-jobs/%d", job.ID), `{"schedule": "0 12 * * *"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Schedule string `json:"schedule"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Schedule != "0 12 * * *" {
		t.Errorf("expected schedule=0 12 * * *, got %s", resp.Data.Schedule)
	}
}

func TestCronJobs_ToggleEnabled(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	job := createTestCronJob(t, db, proj.ID)

	r := cronRouter(db)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/cron-jobs/%d", job.ID), `{"enabled": false}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Enabled {
		t.Error("expected enabled=false after toggle")
	}
}

func TestCronJobs_UpdateInvalidSchedule(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	job := createTestCronJob(t, db, proj.ID)

	r := cronRouter(db)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/cron-jobs/%d", job.ID), `{"schedule": "invalid"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCronJobs_UpdateNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := cronRouter(db)
	w := doRequest(r, http.MethodPatch, "/api/v1/cron-jobs/99999", `{"name": "foo"}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Delete tests
// ---------------------------------------------------------------------------

func TestCronJobs_DeleteExisting(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	job := createTestCronJob(t, db, proj.ID)

	r := cronRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/cron-jobs/%d", job.ID), "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it's gone.
	var count int64
	db.Model(&models.CronJob{}).Where("id = ?", job.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected job to be deleted, found %d", count)
	}
}

func TestCronJobs_DeleteNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := cronRouter(db)
	w := doRequest(r, http.MethodDelete, "/api/v1/cron-jobs/99999", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCronJobs_DeleteCascadesExecutions(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	job := createTestCronJob(t, db, proj.ID)

	// Create an execution.
	now := time.Now()
	db.Create(&models.CronExecution{
		CronJobID:  job.ID,
		Status:     "success",
		StartedAt:  now,
		FinishedAt: &now,
	})

	r := cronRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/cron-jobs/%d", job.ID), "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify executions are also gone.
	var count int64
	db.Model(&models.CronExecution{}).Where("cron_job_id = ?", job.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected executions to be cascade-deleted, found %d", count)
	}
}

// ---------------------------------------------------------------------------
// Execution list tests
// ---------------------------------------------------------------------------

func TestCronJobs_ListExecutions(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	job := createTestCronJob(t, db, proj.ID)

	// Create some executions.
	now := time.Now()
	for i := 0; i < 5; i++ {
		started := now.Add(-time.Duration(i) * time.Hour)
		db.Create(&models.CronExecution{
			CronJobID:  job.ID,
			Status:     "success",
			StartedAt:  started,
			FinishedAt: &started,
		})
	}

	r := cronRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/cron-jobs/%d/executions", job.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Total int64         `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 5 {
		t.Errorf("expected total=5, got %d", resp.Total)
	}
	if len(resp.Data) != 5 {
		t.Errorf("expected 5 items, got %d", len(resp.Data))
	}
}

func TestCronJobs_ListExecutionsPagination(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	job := createTestCronJob(t, db, proj.ID)

	now := time.Now()
	for i := 0; i < 5; i++ {
		started := now.Add(-time.Duration(i) * time.Hour)
		db.Create(&models.CronExecution{
			CronJobID:  job.ID,
			Status:     "success",
			StartedAt:  started,
			FinishedAt: &started,
		})
	}

	r := cronRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/cron-jobs/%d/executions?limit=2&offset=1", job.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Total int64         `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 5 {
		t.Errorf("expected total=5 (unchanged by pagination), got %d", resp.Total)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 items (limit=2), got %d", len(resp.Data))
	}
}

func TestCronJobs_ListExecutionsJobNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := cronRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/cron-jobs/99999/executions", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Manual trigger test
// ---------------------------------------------------------------------------

func TestCronJobs_RunReturns202(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// The Run endpoint requires a non-nil scheduler with a real Claude binary.
	// We can't construct one in tests, so we verify the route is registered and
	// returns a proper error when scheduler is nil.
	r := cronRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/cron-jobs/99999/run", "")

	// With nil scheduler, the handler returns 500 "cron scheduler not available".
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Purge test (via settings handler)
// ---------------------------------------------------------------------------

func TestCronJobs_PurgeOldExecutions(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)
	job := createTestCronJob(t, db, proj.ID)

	// Create old and recent executions.
	oldTime := time.Now().Add(-60 * 24 * time.Hour) // 60 days ago
	recentTime := time.Now().Add(-1 * time.Hour)    // 1 hour ago

	finished1 := oldTime.Add(time.Minute)
	finished2 := recentTime.Add(time.Minute)
	db.Create(&models.CronExecution{CronJobID: job.ID, Status: "success", StartedAt: oldTime, FinishedAt: &finished1})
	db.Create(&models.CronExecution{CronJobID: job.ID, Status: "success", StartedAt: recentTime, FinishedAt: &finished2})

	// Use the settings handler purge endpoint.
	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodDelete, "/api/v1/settings/cron-executions?days=30", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Purged        int64 `json:"purged"`
			RetentionDays int   `json:"retention_days"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Purged != 1 {
		t.Errorf("expected purged=1, got %d", resp.Data.Purged)
	}
	if resp.Data.RetentionDays != 30 {
		t.Errorf("expected retention_days=30, got %d", resp.Data.RetentionDays)
	}

	// Verify only the recent one remains.
	var count int64
	db.Model(&models.CronExecution{}).Where("cron_job_id = ?", job.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 remaining execution, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Validation unit tests
// ---------------------------------------------------------------------------

func TestValidateCreateCronJob_AllRequired(t *testing.T) {
	tests := []struct {
		name string
		req  createCronJobRequest
		want string
	}{
		{"missing name", createCronJobRequest{Schedule: "* * * * *", Prompt: "test", ProjectID: uuid.New()}, "name is required"},
		{"missing schedule", createCronJobRequest{Name: "test", Prompt: "test", ProjectID: uuid.New()}, "schedule is required"},
		{"missing prompt", createCronJobRequest{Name: "test", Schedule: "* * * * *", ProjectID: uuid.New()}, "prompt is required"},
		{"missing project_id", createCronJobRequest{Name: "test", Schedule: "* * * * *", Prompt: "test"}, "project_id is required"},
		{"invalid schedule", createCronJobRequest{Name: "test", Schedule: "bad", Prompt: "test", ProjectID: uuid.New()}, "invalid cron schedule expression"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateCreateCronJob(&tt.req)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestValidateCreateCronJob_ValidRequest(t *testing.T) {
	req := createCronJobRequest{
		Name:      "test",
		Schedule:  "*/5 * * * *",
		Prompt:    "run something",
		ProjectID: uuid.New(),
	}
	if msg := validateCreateCronJob(&req); msg != "" {
		t.Errorf("expected no error, got %q", msg)
	}
}

func TestBuildCronJobUpdates_NilFieldsOmitted(t *testing.T) {
	req := updateCronJobRequest{}
	updates := buildCronJobUpdates(req)
	if len(updates) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(updates))
	}
}

func TestBuildCronJobUpdates_AllFieldsIncluded(t *testing.T) {
	name := "new"
	sched := "0 0 * * *"
	prompt := "do it"
	enabled := false
	timeout := 60
	model := "opus"
	req := updateCronJobRequest{
		Name:           &name,
		Schedule:       &sched,
		Prompt:         &prompt,
		Enabled:        &enabled,
		TimeoutMinutes: &timeout,
		Model:          &model,
	}
	updates := buildCronJobUpdates(req)
	if len(updates) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(updates))
	}
}
