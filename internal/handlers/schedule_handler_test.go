package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"botka/internal/models"
	"botka/internal/runner"
)

func setupScheduleRouter(t *testing.T) (*gin.Engine, *gin.RouterGroup) {
	t.Helper()
	r := gin.New()
	g := r.Group("/api/v1")
	return r, g
}

func TestCreateSchedule_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	body := `{
		"project_id":"` + proj.ID.String() + `",
		"title":"Daily digest",
		"spec":"Summarize PRs",
		"cron_expression":"0 9 * * *",
		"priority":2,
		"enabled":true
	}`
	w := doRequest(r, "POST", "/api/v1/schedules", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data models.TaskSchedule `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Title != "Daily digest" || resp.Data.Priority != 2 || !resp.Data.Enabled {
		t.Errorf("unexpected response: %+v", resp.Data)
	}
	if resp.Data.NextRunAt == nil || !resp.Data.NextRunAt.After(time.Now().Add(-time.Minute)) {
		t.Errorf("next_run_at not set: %+v", resp.Data.NextRunAt)
	}
}

func TestCreateSchedule_InvalidCron(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	body := `{"project_id":"` + proj.ID.String() + `","title":"x","cron_expression":"not a cron"}`
	w := doRequest(r, "POST", "/api/v1/schedules", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestCreateSchedule_MissingTitle(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	body := `{"project_id":"` + proj.ID.String() + `","cron_expression":"* * * * *"}`
	w := doRequest(r, "POST", "/api/v1/schedules", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateSchedule_UnknownProject(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	body := `{"project_id":"` + uuid.New().String() + `","title":"x","cron_expression":"* * * * *"}`
	w := doRequest(r, "POST", "/api/v1/schedules", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListSchedules(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	now := time.Now().Add(time.Hour)

	for _, title := range []string{"alpha", "beta"} {
		s := models.TaskSchedule{
			ProjectID: proj.ID, Title: title,
			CronExpression: "* * * * *", NextRunAt: &now, Enabled: true,
		}
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("create schedule: %v", err)
		}
	}

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	w := doRequest(r, "GET", "/api/v1/schedules", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data  []models.TaskSchedule `json:"data"`
		Total int64                 `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 || len(resp.Data) != 2 {
		t.Errorf("got total=%d data=%d", resp.Total, len(resp.Data))
	}
	if resp.Data[0].Title != "alpha" {
		t.Errorf("expected alphabetical order, got %q first", resp.Data[0].Title)
	}
}

func TestUpdateSchedule_RecomputesNextRun(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t0 := time.Now().Add(time.Hour)
	sched := models.TaskSchedule{
		ProjectID: proj.ID, Title: "x",
		CronExpression: "0 9 * * *", NextRunAt: &t0, Enabled: true,
	}
	if err := db.Create(&sched).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	body := `{"cron_expression":"*/5 * * * *","title":"renamed"}`
	w := doRequest(r, "PUT", "/api/v1/schedules/"+strconv.FormatInt(sched.ID, 10), body)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var reloaded models.TaskSchedule
	if err := db.First(&reloaded, sched.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Title != "renamed" {
		t.Errorf("title not updated: %q", reloaded.Title)
	}
	if reloaded.CronExpression != "*/5 * * * *" {
		t.Errorf("cron not updated: %q", reloaded.CronExpression)
	}
	if reloaded.NextRunAt == nil || reloaded.NextRunAt.Equal(t0) {
		t.Errorf("next_run_at should have been recomputed")
	}
}

func TestUpdateSchedule_TogglesEnabled(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t0 := time.Now().Add(time.Hour)
	sched := models.TaskSchedule{
		ProjectID: proj.ID, Title: "x",
		CronExpression: "* * * * *", NextRunAt: &t0, Enabled: true,
	}
	if err := db.Create(&sched).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	w := doRequest(r, "PUT", "/api/v1/schedules/"+strconv.FormatInt(sched.ID, 10), `{"enabled":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var reloaded models.TaskSchedule
	db.First(&reloaded, sched.ID)
	if reloaded.Enabled {
		t.Error("expected enabled=false")
	}
}

func TestDeleteSchedule(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t0 := time.Now().Add(time.Hour)
	sched := models.TaskSchedule{
		ProjectID: proj.ID, Title: "x",
		CronExpression: "* * * * *", NextRunAt: &t0, Enabled: true,
	}
	if err := db.Create(&sched).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	w := doRequest(r, "DELETE", "/api/v1/schedules/"+strconv.FormatInt(sched.ID, 10), "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var count int64
	db.Model(&models.TaskSchedule{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 schedules, got %d", count)
	}
}

func TestRunNowSchedule_CreatesTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t0 := time.Now().Add(time.Hour)
	sched := models.TaskSchedule{
		ProjectID: proj.ID, Title: "x", Spec: "do thing",
		CronExpression: "* * * * *", NextRunAt: &t0, Enabled: false,
	}
	if err := db.Create(&sched).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	w := doRequest(r, "POST", "/api/v1/schedules/"+strconv.FormatInt(sched.ID, 10)+"/run-now", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.TaskID == "" {
		t.Fatal("expected a task id")
	}
	var task models.Task
	if err := db.First(&task, "id = ?", resp.Data.TaskID).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != models.TaskStatusPending {
		t.Errorf("status = %s", task.Status)
	}
	if task.ScheduleID == nil || *task.ScheduleID != sched.ID {
		t.Errorf("schedule_id not set: %v", task.ScheduleID)
	}
}

func TestGetSchedule_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r, g := setupScheduleRouter(t)
	h := NewScheduleHandler(db, runner.NewScheduleScheduler(db))
	RegisterScheduleRoutes(g, h)

	w := doRequest(r, "GET", "/api/v1/schedules/9999", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}
