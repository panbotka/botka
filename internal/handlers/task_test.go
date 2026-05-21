package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/runner"
)

func taskRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewTaskHandler(db, runner.NewTaskEventHub(), nil, "")
	v1 := r.Group("/api/v1")
	RegisterTaskRoutes(v1, h)
	return r
}

// ---------------------------------------------------------------------------
// Pure helper function tests
// ---------------------------------------------------------------------------

func TestValidateCreateRequest_TitleRequired(t *testing.T) {
	req := &createTaskRequest{ProjectID: uuid.New()}
	if err := validateCreateRequest(req); err == nil || err.Error() != "title is required" {
		t.Fatalf("expected 'title is required', got %v", err)
	}
}

func TestValidateCreateRequest_ProjectIDRequired(t *testing.T) {
	req := &createTaskRequest{Title: "foo"}
	if err := validateCreateRequest(req); err == nil || err.Error() != "project_id is required" {
		t.Fatalf("expected 'project_id is required', got %v", err)
	}
}

func TestValidateCreateRequest_DefaultStatus(t *testing.T) {
	req := &createTaskRequest{Title: "foo", ProjectID: uuid.New()}
	if err := validateCreateRequest(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Status != models.TaskStatusQueued {
		t.Errorf("expected default status queued, got %s", req.Status)
	}
}

func TestValidateCreateRequest_InvalidStatus(t *testing.T) {
	req := &createTaskRequest{Title: "foo", ProjectID: uuid.New(), Status: models.TaskStatusRunning}
	if err := validateCreateRequest(req); err == nil || err.Error() != "status must be pending or queued" {
		t.Fatalf("expected 'status must be pending or queued', got %v", err)
	}
}

func TestValidateUpdate_RunningBlocksNonStatusEdits(t *testing.T) {
	task := models.Task{Status: models.TaskStatusRunning}
	title := "new title"
	req := updateTaskRequest{Title: &title}
	want := "cannot edit title, spec, or priority of a running task"
	if msg := validateUpdate(task, req); msg != want {
		t.Fatalf("expected %q, got %q", want, msg)
	}
}

func TestValidateUpdate_RunningAllowsStatusOnly(t *testing.T) {
	task := models.Task{Status: models.TaskStatusRunning}
	s := models.TaskStatusFailed
	req := updateTaskRequest{Status: &s}
	if msg := validateUpdate(task, req); msg != "" {
		t.Fatalf("expected running→failed allowed, got %q", msg)
	}
}

func TestValidateUpdate_AllStatusTransitionsAllowed(t *testing.T) {
	cases := []struct {
		from, to models.TaskStatus
	}{
		{models.TaskStatusDone, models.TaskStatusQueued},
		{models.TaskStatusFailed, models.TaskStatusDone},
		{models.TaskStatusQueued, models.TaskStatusRunning},
		{models.TaskStatusPending, models.TaskStatusRunning},
		{models.TaskStatusCancelled, models.TaskStatusRunning},
	}
	for _, tc := range cases {
		task := models.Task{Status: tc.from}
		s := tc.to
		req := updateTaskRequest{Status: &s}
		if msg := validateUpdate(task, req); msg != "" {
			t.Errorf("%s→%s: expected allowed, got %q", tc.from, tc.to, msg)
		}
	}
}

func TestBuildTaskUpdates_NilFieldsOmitted(t *testing.T) {
	req := updateTaskRequest{}
	updates := buildTaskUpdates(req, models.Task{Status: models.TaskStatusPending})
	if len(updates) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(updates))
	}
}

func TestBuildTaskUpdates_NonNilIncluded(t *testing.T) {
	title := "new"
	spec := "spec"
	pri := 5
	s := models.TaskStatusQueued
	req := updateTaskRequest{Title: &title, Spec: &spec, Priority: &pri, Status: &s}
	updates := buildTaskUpdates(req, models.Task{Status: models.TaskStatusPending})
	if len(updates) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(updates))
	}
	if updates["title"] != "new" {
		t.Errorf("title mismatch: %v", updates["title"])
	}
	if updates["spec"] != "spec" {
		t.Errorf("spec mismatch: %v", updates["spec"])
	}
	if updates["priority"] != 5 {
		t.Errorf("priority mismatch: %v", updates["priority"])
	}
	if updates["status"] != models.TaskStatusQueued {
		t.Errorf("status mismatch: %v", updates["status"])
	}
}

func TestBuildTaskUpdates_StartedAtSetOnRunningWhenNull(t *testing.T) {
	s := models.TaskStatusRunning
	req := updateTaskRequest{Status: &s}
	updates := buildTaskUpdates(req, models.Task{Status: models.TaskStatusQueued})
	if _, ok := updates["started_at"]; !ok {
		t.Errorf("expected started_at to be set, got %v", updates)
	}
}

func TestBuildTaskUpdates_StartedAtUntouchedWhenAlreadySet(t *testing.T) {
	s := models.TaskStatusRunning
	req := updateTaskRequest{Status: &s}
	prior := time.Now().Add(-time.Hour)
	updates := buildTaskUpdates(req, models.Task{
		Status:    models.TaskStatusFailed,
		StartedAt: &prior,
	})
	if _, ok := updates["started_at"]; ok {
		t.Errorf("expected started_at to be untouched, got %v", updates)
	}
}

func TestBuildTaskUpdates_CompletedAtSetOnTerminal(t *testing.T) {
	for _, target := range []models.TaskStatus{
		models.TaskStatusDone,
		models.TaskStatusFailed,
		models.TaskStatusNeedsReview,
		models.TaskStatusCancelled,
	} {
		s := target
		req := updateTaskRequest{Status: &s}
		updates := buildTaskUpdates(req, models.Task{Status: models.TaskStatusRunning})
		if _, ok := updates["completed_at"]; !ok {
			t.Errorf("%s: expected completed_at to be set, got %v", target, updates)
		}
	}
}

func TestBuildTaskUpdates_NoTimestampOnNonStatusTargets(t *testing.T) {
	for _, target := range []models.TaskStatus{
		models.TaskStatusPending,
		models.TaskStatusQueued,
		models.TaskStatusDeleted,
	} {
		s := target
		req := updateTaskRequest{Status: &s}
		updates := buildTaskUpdates(req, models.Task{Status: models.TaskStatusFailed})
		if _, ok := updates["started_at"]; ok {
			t.Errorf("%s: unexpected started_at: %v", target, updates)
		}
		if _, ok := updates["completed_at"]; ok {
			t.Errorf("%s: unexpected completed_at: %v", target, updates)
		}
	}
}

func TestParsePagination_Defaults(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	limit, offset := parsePagination(c)
	if limit != 50 {
		t.Errorf("expected default limit 50, got %d", limit)
	}
	if offset != 0 {
		t.Errorf("expected default offset 0, got %d", offset)
	}
}

func TestParsePagination_CustomValues(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?limit=10&offset=5", nil)
	limit, offset := parsePagination(c)
	if limit != 10 {
		t.Errorf("expected limit 10, got %d", limit)
	}
	if offset != 5 {
		t.Errorf("expected offset 5, got %d", offset)
	}
}

func TestValidateBatchStatusRequest_EmptyIDs(t *testing.T) {
	req := batchStatusRequest{IDs: []uuid.UUID{}, Status: models.TaskStatusQueued}
	if msg := validateBatchStatusRequest(req); msg != "ids must not be empty" {
		t.Fatalf("expected 'ids must not be empty', got %q", msg)
	}
}

func TestValidateBatchStatusRequest_DuplicateIDs(t *testing.T) {
	id := uuid.New()
	req := batchStatusRequest{IDs: []uuid.UUID{id, id}, Status: models.TaskStatusQueued}
	expected := fmt.Sprintf("duplicate id: %s", id)
	if msg := validateBatchStatusRequest(req); msg != expected {
		t.Fatalf("expected %q, got %q", expected, msg)
	}
}

func TestValidateBatchStatusRequest_InvalidStatus(t *testing.T) {
	req := batchStatusRequest{IDs: []uuid.UUID{uuid.New()}, Status: "bogus"}
	if msg := validateBatchStatusRequest(req); msg != "invalid status: bogus" {
		t.Fatalf("expected invalid status error, got %q", msg)
	}
}

func TestFindMissingID_AllFound(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	tasks := []models.Task{{ID: id1}, {ID: id2}}
	if missing := findMissingID([]uuid.UUID{id1, id2}, tasks); missing != nil {
		t.Fatalf("expected nil, got %s", missing)
	}
}

func TestFindMissingID_OneMissing(t *testing.T) {
	id1, id2 := uuid.New(), uuid.New()
	tasks := []models.Task{{ID: id1}}
	missing := findMissingID([]uuid.UUID{id1, id2}, tasks)
	if missing == nil || *missing != id2 {
		t.Fatalf("expected %s missing, got %v", id2, missing)
	}
}

func TestValidateBatchTransitions_ValidTransition(t *testing.T) {
	tasks := []models.Task{{ID: uuid.New(), Status: models.TaskStatusPending}}
	invalid := validateBatchTransitions(tasks, models.TaskStatusQueued)
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid, got %d", len(invalid))
	}
}

func TestValidateBatchTransitions_InvalidTransition(t *testing.T) {
	tasks := []models.Task{{ID: uuid.New(), Status: models.TaskStatusDone}}
	invalid := validateBatchTransitions(tasks, models.TaskStatusQueued)
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid, got %d", len(invalid))
	}
	if invalid[0].CurrentStatus != models.TaskStatusDone {
		t.Errorf("expected current_status done, got %s", invalid[0].CurrentStatus)
	}
}

func TestValidateBatchTransitions_SameStatusSkipped(t *testing.T) {
	tasks := []models.Task{{ID: uuid.New(), Status: models.TaskStatusQueued}}
	invalid := validateBatchTransitions(tasks, models.TaskStatusQueued)
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid when same status, got %d", len(invalid))
	}
}

func TestValidateCreateRequest_TitleTooLong(t *testing.T) {
	req := &createTaskRequest{
		Title:     string(make([]byte, maxTitleLength+1)),
		ProjectID: uuid.New(),
	}
	err := validateCreateRequest(req)
	if err == nil {
		t.Fatal("expected error for title too long")
	}
	if err.Error() != fmt.Sprintf("title must be at most %d characters", maxTitleLength) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateCreateRequest_SpecTooLong(t *testing.T) {
	req := &createTaskRequest{
		Title:     "ok",
		Spec:      string(make([]byte, maxSpecLength+1)),
		ProjectID: uuid.New(),
	}
	err := validateCreateRequest(req)
	if err == nil {
		t.Fatal("expected error for spec too long")
	}
	if err.Error() != fmt.Sprintf("spec must be at most %d characters", maxSpecLength) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateUpdate_TitleTooLong(t *testing.T) {
	task := models.Task{Status: models.TaskStatusPending}
	longTitle := string(make([]byte, maxTitleLength+1))
	req := updateTaskRequest{Title: &longTitle}
	msg := validateUpdate(task, req)
	if msg == "" {
		t.Fatal("expected error for title too long")
	}
}

func TestValidateUpdate_InvalidStatusValue(t *testing.T) {
	task := models.Task{Status: models.TaskStatusPending}
	badStatus := models.TaskStatus("bogus")
	req := updateTaskRequest{Status: &badStatus}
	msg := validateUpdate(task, req)
	if msg != "invalid status value" {
		t.Errorf("expected 'invalid status value', got %q", msg)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (require test database)
// ---------------------------------------------------------------------------

func TestTaskCreate_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"title":"my task","spec":"do stuff","project_id":"%s"}`, proj.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	var task models.Task
	json.Unmarshal(resp["data"], &task)
	if task.Title != "my task" {
		t.Errorf("expected title 'my task', got %q", task.Title)
	}
	if task.Status != models.TaskStatusQueued {
		t.Errorf("expected default status queued, got %s", task.Status)
	}
}

func TestTaskCreate_MissingTitle(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"spec":"do stuff","project_id":"%s"}`, proj.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTaskCreate_InvalidProject(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"title":"t","project_id":"%s"}`, uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/tasks", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTaskGet_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	r := taskRouter(db)

	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%s", task.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	var got models.Task
	json.Unmarshal(resp["data"], &got)
	if got.ID != task.ID {
		t.Errorf("expected id %s, got %s", task.ID, got.ID)
	}
}

func TestTaskGet_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	r := taskRouter(db)

	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%s", uuid.New()), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestTaskGet_InvalidID(t *testing.T) {
	db := setupTestDB(t)
	r := taskRouter(db)

	w := doRequest(r, http.MethodGet, "/api/v1/tasks/not-a-uuid", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTaskList_Empty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	r := taskRouter(db)

	w := doRequest(r, http.MethodGet, "/api/v1/tasks", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	total := resp["total"].(float64)
	if total != 0 {
		t.Errorf("expected total 0, got %.0f", total)
	}
}

func TestTaskList_FilterByStatus(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	createTestTask(t, db, proj.ID, models.TaskStatusPending)
	createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	r := taskRouter(db)

	w := doRequest(r, http.MethodGet, "/api/v1/tasks?status=pending", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	total := resp["total"].(float64)
	if total != 1 {
		t.Errorf("expected 1 pending task, got %.0f", total)
	}
}

func TestTaskList_FilterByProjectID(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj1 := createTestProject(t, db)
	proj2 := createTestProject(t, db)
	createTestTask(t, db, proj1.ID, models.TaskStatusPending)
	createTestTask(t, db, proj2.ID, models.TaskStatusPending)
	r := taskRouter(db)

	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tasks?project_id=%s", proj1.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	total := resp["total"].(float64)
	if total != 1 {
		t.Errorf("expected 1 task for project, got %.0f", total)
	}
}

func TestTaskList_Pagination(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	for i := 0; i < 3; i++ {
		createTestTask(t, db, proj.ID, models.TaskStatusPending)
	}
	r := taskRouter(db)

	w := doRequest(r, http.MethodGet, "/api/v1/tasks?limit=2&offset=0", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	total := resp["total"].(float64)
	if len(data) != 2 {
		t.Errorf("expected 2 items in page, got %d", len(data))
	}
	if total != 3 {
		t.Errorf("expected total 3, got %.0f", total)
	}
}

func TestTaskList_FullTextSearch(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)

	// Distinct titles/specs so search picks each one out individually.
	t1 := models.Task{Title: "Add VAPID keys", Spec: "configure web push", ProjectID: proj.ID, Status: models.TaskStatusPending}
	t2 := models.Task{Title: "Fix migration typo", Spec: "schema_migrations check", ProjectID: proj.ID, Status: models.TaskStatusPending}
	t3 := models.Task{Title: "Refactor renderer", Spec: "no related keywords", ProjectID: proj.ID, Status: models.TaskStatusPending}
	for _, task := range []*models.Task{&t1, &t2, &t3} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create test task: %v", err)
		}
	}

	r := taskRouter(db)

	// Term that appears in t1's title only.
	w := doRequest(r, http.MethodGet, "/api/v1/tasks?q=VAPID", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if total := resp["total"].(float64); total != 1 {
		t.Errorf("expected 1 match for 'VAPID', got %.0f", total)
	}
	data := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("expected 1 result, got %d", len(data))
	}
	first := data[0].(map[string]interface{})
	if first["title"] != "Add VAPID keys" {
		t.Errorf("expected 'Add VAPID keys', got %q", first["title"])
	}

	// Term in t2's spec.
	w = doRequest(r, http.MethodGet, "/api/v1/tasks?q=schema_migrations", "")
	json.Unmarshal(w.Body.Bytes(), &resp)
	if total := resp["total"].(float64); total != 1 {
		t.Errorf("expected 1 match for 'schema_migrations', got %.0f", total)
	}

	// No match.
	w = doRequest(r, http.MethodGet, "/api/v1/tasks?q=nonexistent", "")
	json.Unmarshal(w.Body.Bytes(), &resp)
	if total := resp["total"].(float64); total != 0 {
		t.Errorf("expected 0 matches for 'nonexistent', got %.0f", total)
	}
}

func TestTaskList_SearchCombinesWithFilters(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)

	t1 := models.Task{Title: "VAPID setup", Spec: "x", ProjectID: proj.ID, Status: models.TaskStatusPending}
	t2 := models.Task{Title: "VAPID rotate", Spec: "x", ProjectID: proj.ID, Status: models.TaskStatusDone}
	for _, task := range []*models.Task{&t1, &t2} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create test task: %v", err)
		}
	}

	r := taskRouter(db)

	// q + status — both matches contain VAPID, but only one is pending.
	w := doRequest(r, http.MethodGet, "/api/v1/tasks?q=VAPID&status=pending", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if total := resp["total"].(float64); total != 1 {
		t.Errorf("expected 1 result for q+status, got %.0f", total)
	}
}

func TestTaskUpdate_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	r := taskRouter(db)

	body := `{"title":"updated","status":"queued"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/tasks/%s", task.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	var got models.Task
	json.Unmarshal(resp["data"], &got)
	if got.Title != "updated" {
		t.Errorf("expected title 'updated', got %q", got.Title)
	}
	if got.Status != models.TaskStatusQueued {
		t.Errorf("expected status queued, got %s", got.Status)
	}
}

func TestTaskUpdate_RunningRejectsTitleEdit(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusRunning)
	r := taskRouter(db)

	body := `{"title":"nope"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/tasks/%s", task.ID), body)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestTaskUpdate_RunningAllowsStatusChange(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusRunning)
	r := taskRouter(db)

	body := `{"status":"failed"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/tasks/%s", task.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	var got models.Task
	json.Unmarshal(resp["data"], &got)
	if got.Status != models.TaskStatusFailed {
		t.Errorf("expected status failed, got %s", got.Status)
	}
	if got.CompletedAt == nil {
		t.Errorf("expected completed_at to be set on running→failed")
	}
}

func TestTaskUpdate_DoneToPendingAllowed(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusDone)
	r := taskRouter(db)

	body := `{"status":"pending"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/tasks/%s", task.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskUpdate_QueuedToRunning_SetsStartedAt(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	r := taskRouter(db)

	body := `{"status":"running"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/tasks/%s", task.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	var got models.Task
	json.Unmarshal(resp["data"], &got)
	if got.Status != models.TaskStatusRunning {
		t.Errorf("expected status running, got %s", got.Status)
	}
	if got.StartedAt == nil {
		t.Errorf("expected started_at to be set on →running")
	}
}

func TestTaskUpdate_ConcurrentRunning_409(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	createTestTask(t, db, proj.ID, models.TaskStatusRunning)
	other := createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	r := taskRouter(db)

	body := `{"status":"running"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/tasks/%s", other.ID), body)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "already running") {
		t.Errorf("expected message about another task already running, got %s", w.Body.String())
	}
}

func TestTaskDelete_SoftDeletePending(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	r := taskRouter(db)

	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%s", task.ID), "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	var got models.Task
	db.First(&got, "id = ?", task.ID)
	if got.Status != models.TaskStatusDeleted {
		t.Errorf("expected status deleted, got %s", got.Status)
	}
}

func TestTaskDelete_SoftDeleteQueued(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	r := taskRouter(db)

	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%s", task.ID), "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	var got models.Task
	db.First(&got, "id = ?", task.ID)
	if got.Status != models.TaskStatusDeleted {
		t.Errorf("expected status deleted, got %s", got.Status)
	}
}

func TestTaskDelete_RunningConflict(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusRunning)
	r := taskRouter(db)

	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%s", task.ID), "")

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestTaskDelete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	r := taskRouter(db)

	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%s", uuid.New()), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestTaskRetry_FailedTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusFailed)
	r := taskRouter(db)

	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/retry", task.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	var got models.Task
	json.Unmarshal(resp["data"], &got)
	if got.Status != models.TaskStatusQueued {
		t.Errorf("expected status queued after retry, got %s", got.Status)
	}
}

func TestTaskRetry_NeedsReviewTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusNeedsReview)
	r := taskRouter(db)

	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/retry", task.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestTaskRetry_PendingTaskRejected(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	r := taskRouter(db)

	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/retry", task.ID), "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTaskBatchStatus_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t1 := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	t2 := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"ids":["%s","%s"],"status":"queued"}`, t1.ID, t2.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/batch-status", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Verify both updated
	var got1, got2 models.Task
	db.First(&got1, "id = ?", t1.ID)
	db.First(&got2, "id = ?", t2.ID)
	if got1.Status != models.TaskStatusQueued {
		t.Errorf("task1: expected queued, got %s", got1.Status)
	}
	if got2.Status != models.TaskStatusQueued {
		t.Errorf("task2: expected queued, got %s", got2.Status)
	}
}

func TestTaskBatchStatus_MissingTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t1 := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"ids":["%s","%s"],"status":"queued"}`, t1.ID, uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/batch-status", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskBatchStatus_InvalidTransition(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t1 := createTestTask(t, db, proj.ID, models.TaskStatusDone)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"ids":["%s"],"status":"queued"}`, t1.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/batch-status", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskReorder_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t1 := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	t2 := createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	r := taskRouter(db)

	body := fmt.Sprintf(`[{"id":"%s","priority":10},{"id":"%s","priority":20}]`, t1.ID, t2.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/reorder", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got1, got2 models.Task
	db.First(&got1, "id = ?", t1.ID)
	db.First(&got2, "id = ?", t2.ID)
	if got1.Priority != 10 {
		t.Errorf("task1: expected priority 10, got %d", got1.Priority)
	}
	if got2.Priority != 20 {
		t.Errorf("task2: expected priority 20, got %d", got2.Priority)
	}
}

func TestTaskReorder_EmptyList(t *testing.T) {
	db := setupTestDB(t)
	r := taskRouter(db)

	w := doRequest(r, http.MethodPost, "/api/v1/tasks/reorder", `[]`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTaskReorder_TaskNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	r := taskRouter(db)

	body := fmt.Sprintf(`[{"id":"%s","priority":1}]`, uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/reorder", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskList_InvalidProjectID(t *testing.T) {
	db := setupTestDB(t)
	r := taskRouter(db)

	w := doRequest(r, http.MethodGet, "/api/v1/tasks?project_id=not-a-uuid", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTaskCreate_PendingStatus(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"title":"t","project_id":"%s","status":"pending"}`, proj.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &resp)
	var task models.Task
	json.Unmarshal(resp["data"], &task)
	if task.Status != models.TaskStatusPending {
		t.Errorf("expected status pending, got %s", task.Status)
	}
}

func TestTaskDelete_SoftDeleteCancelled(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusCancelled)
	r := taskRouter(db)

	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%s", task.ID), "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	var got models.Task
	db.First(&got, "id = ?", task.ID)
	if got.Status != models.TaskStatusDeleted {
		t.Errorf("expected status deleted, got %s", got.Status)
	}
}

func TestTaskDelete_SoftDeleteFailed(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusFailed)
	r := taskRouter(db)

	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%s", task.ID), "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	var got models.Task
	db.First(&got, "id = ?", task.ID)
	if got.Status != models.TaskStatusDeleted {
		t.Errorf("expected status deleted, got %s", got.Status)
	}
}

func TestTask_Stats(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)

	// Create tasks with various statuses.
	createTestTask(t, db, proj.ID, models.TaskStatusPending)
	createTestTask(t, db, proj.ID, models.TaskStatusPending)
	createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	createTestTask(t, db, proj.ID, models.TaskStatusDone)
	createTestTask(t, db, proj.ID, models.TaskStatusDone)
	createTestTask(t, db, proj.ID, models.TaskStatusDone)
	createTestTask(t, db, proj.ID, models.TaskStatusFailed)

	r := taskRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/tasks/stats", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Total    int64 `json:"total"`
			ByStatus struct {
				Pending     int64 `json:"pending"`
				Queued      int64 `json:"queued"`
				Running     int64 `json:"running"`
				Done        int64 `json:"done"`
				Failed      int64 `json:"failed"`
				NeedsReview int64 `json:"needs_review"`
				Cancelled   int64 `json:"cancelled"`
			} `json:"by_status"`
			SuccessRate *float64         `json:"success_rate"`
			TopProject  *json.RawMessage `json:"top_project"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Data.Total != 7 {
		t.Errorf("expected total=7, got %d", resp.Data.Total)
	}
	if resp.Data.ByStatus.Pending != 2 {
		t.Errorf("expected pending=2, got %d", resp.Data.ByStatus.Pending)
	}
	if resp.Data.ByStatus.Queued != 1 {
		t.Errorf("expected queued=1, got %d", resp.Data.ByStatus.Queued)
	}
	if resp.Data.ByStatus.Done != 3 {
		t.Errorf("expected done=3, got %d", resp.Data.ByStatus.Done)
	}
	if resp.Data.ByStatus.Failed != 1 {
		t.Errorf("expected failed=1, got %d", resp.Data.ByStatus.Failed)
	}
	if resp.Data.ByStatus.Running != 0 {
		t.Errorf("expected running=0, got %d", resp.Data.ByStatus.Running)
	}

	// Success rate: 3 done / (3 done + 1 failed) = 0.75
	if resp.Data.SuccessRate == nil {
		t.Fatal("expected success_rate to be non-nil")
	}
	if *resp.Data.SuccessRate != 0.75 {
		t.Errorf("expected success_rate=0.75, got %f", *resp.Data.SuccessRate)
	}

	// Top project should be our test project.
	if resp.Data.TopProject == nil {
		t.Error("expected top_project to be non-nil")
	}
}

// ---------------------------------------------------------------------------
// /tasks/bulk endpoint tests
// ---------------------------------------------------------------------------

// bulkResp is the test-side decoder for the /tasks/bulk response.
type bulkResp struct {
	Data struct {
		Succeeded []uuid.UUID `json:"succeeded"`
		Failed    []struct {
			ID    uuid.UUID `json:"id"`
			Error string    `json:"error"`
		} `json:"failed"`
	} `json:"data"`
}

func decodeBulkResp(t *testing.T, body []byte) bulkResp {
	t.Helper()
	var resp bulkResp
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode bulk response: %v: %s", err, string(body))
	}
	return resp
}

// failureFor returns the per-task error message for id, or "" if not in failed.
func (r bulkResp) failureFor(id uuid.UUID) string {
	for _, f := range r.Data.Failed {
		if f.ID == id {
			return f.Error
		}
	}
	return ""
}

// succeededHas reports whether the response marks the id as succeeded.
func (r bulkResp) succeededHas(id uuid.UUID) bool {
	for _, s := range r.Data.Succeeded {
		if s == id {
			return true
		}
	}
	return false
}

func TestTaskBulk_EmptyIDs(t *testing.T) {
	db := setupTestDB(t)
	r := taskRouter(db)

	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", `{"task_ids":[],"operation":"delete"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskBulk_DuplicateID(t *testing.T) {
	db := setupTestDB(t)
	r := taskRouter(db)

	id := uuid.New()
	body := fmt.Sprintf(`{"task_ids":["%s","%s"],"operation":"delete"}`, id, id)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskBulk_OverCap(t *testing.T) {
	db := setupTestDB(t)
	r := taskRouter(db)

	ids := make([]string, bulkMaxIDs+1)
	for i := range ids {
		ids[i] = fmt.Sprintf(`"%s"`, uuid.New())
	}
	body := fmt.Sprintf(`{"task_ids":[%s],"operation":"delete"}`, strings.Join(ids, ","))
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskBulk_InvalidOperation(t *testing.T) {
	db := setupTestDB(t)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"task_ids":["%s"],"operation":"frobnicate"}`, uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskBulk_SetPriority(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t1 := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	t2 := createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"task_ids":["%s","%s"],"operation":"set_priority","payload":{"priority":42}}`,
		t1.ID, t2.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeBulkResp(t, w.Body.Bytes())
	if len(resp.Data.Succeeded) != 2 || len(resp.Data.Failed) != 0 {
		t.Fatalf("expected 2 succeeded and 0 failed, got %+v", resp.Data)
	}
	var got1, got2 models.Task
	db.First(&got1, "id = ?", t1.ID)
	db.First(&got2, "id = ?", t2.ID)
	if got1.Priority != 42 || got2.Priority != 42 {
		t.Errorf("expected priority 42, got %d / %d", got1.Priority, got2.Priority)
	}
}

func TestTaskBulk_SetPriority_MissingPayload(t *testing.T) {
	db := setupTestDB(t)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"task_ids":["%s"],"operation":"set_priority"}`, uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskBulk_Cancel_Mixed(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	pending := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	done := createTestTask(t, db, proj.ID, models.TaskStatusDone)
	missing := uuid.New()
	r := taskRouter(db)

	body := fmt.Sprintf(`{"task_ids":["%s","%s","%s"],"operation":"cancel"}`,
		pending.ID, done.ID, missing)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeBulkResp(t, w.Body.Bytes())
	if !resp.succeededHas(pending.ID) {
		t.Errorf("pending: expected succeeded, got failed=%q", resp.failureFor(pending.ID))
	}
	if msg := resp.failureFor(done.ID); msg == "" {
		t.Errorf("done: expected failure, got success")
	}
	if msg := resp.failureFor(missing); msg != "task not found" {
		t.Errorf("missing: expected 'task not found', got %q", msg)
	}
	var got models.Task
	db.First(&got, "id = ?", pending.ID)
	if got.Status != models.TaskStatusCancelled {
		t.Errorf("pending task: expected cancelled, got %s", got.Status)
	}
}

func TestTaskBulk_Requeue_FailedToQueued(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	failed := createTestTask(t, db, proj.ID, models.TaskStatusFailed)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"task_ids":["%s"],"operation":"requeue"}`, failed.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeBulkResp(t, w.Body.Bytes())
	if !resp.succeededHas(failed.ID) {
		t.Fatalf("expected succeeded, got %+v", resp.Data)
	}
	var got models.Task
	db.First(&got, "id = ?", failed.ID)
	if got.Status != models.TaskStatusQueued {
		t.Errorf("expected queued, got %s", got.Status)
	}
}

func TestTaskBulk_SetPending_QueuedToPending(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	queued := createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"task_ids":["%s"],"operation":"set_pending"}`, queued.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeBulkResp(t, w.Body.Bytes())
	if !resp.succeededHas(queued.ID) {
		t.Fatalf("expected succeeded, got %+v", resp.Data)
	}
	var got models.Task
	db.First(&got, "id = ?", queued.ID)
	if got.Status != models.TaskStatusPending {
		t.Errorf("expected pending, got %s", got.Status)
	}
}

func TestTaskBulk_Cancel_RunningSkipped(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	running := createTestTask(t, db, proj.ID, models.TaskStatusRunning)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"task_ids":["%s"],"operation":"cancel"}`, running.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeBulkResp(t, w.Body.Bytes())
	if msg := resp.failureFor(running.ID); msg != "cannot change status of a running task" {
		t.Errorf("unexpected failure message: %q", msg)
	}
	var got models.Task
	db.First(&got, "id = ?", running.ID)
	if got.Status != models.TaskStatusRunning {
		t.Errorf("running task should be unchanged, got %s", got.Status)
	}
}

func TestTaskBulk_Delete_RunningSkipped(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	pending := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	running := createTestTask(t, db, proj.ID, models.TaskStatusRunning)
	r := taskRouter(db)

	body := fmt.Sprintf(`{"task_ids":["%s","%s"],"operation":"delete"}`, pending.ID, running.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeBulkResp(t, w.Body.Bytes())
	if !resp.succeededHas(pending.ID) {
		t.Error("pending should be deleted successfully")
	}
	if msg := resp.failureFor(running.ID); msg == "" {
		t.Error("running should not be deleted")
	}
	var got1, got2 models.Task
	db.First(&got1, "id = ?", pending.ID)
	db.First(&got2, "id = ?", running.ID)
	if got1.Status != models.TaskStatusDeleted {
		t.Errorf("pending: expected deleted, got %s", got1.Status)
	}
	if got2.Status != models.TaskStatusRunning {
		t.Errorf("running: expected unchanged, got %s", got2.Status)
	}
}

func TestTaskBulk_AddTags_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	tag1 := models.TaskTag{Name: "bug", Color: "#FF0000"}
	tag2 := models.TaskTag{Name: "urgent", Color: "#00FF00"}
	if err := db.Create(&tag1).Error; err != nil {
		t.Fatalf("create tag1: %v", err)
	}
	if err := db.Create(&tag2).Error; err != nil {
		t.Fatalf("create tag2: %v", err)
	}
	r := taskRouter(db)

	body := fmt.Sprintf(
		`{"task_ids":["%s"],"operation":"add_tags","payload":{"tag_ids":[%d,%d]}}`,
		task.ID, tag1.ID, tag2.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeBulkResp(t, w.Body.Bytes())
	if !resp.succeededHas(task.ID) {
		t.Fatalf("expected succeeded, got %+v", resp.Data)
	}
	var assignments []models.TaskTagAssignment
	db.Where("task_id = ?", task.ID).Find(&assignments)
	if len(assignments) != 2 {
		t.Errorf("expected 2 assignments, got %d", len(assignments))
	}
}

func TestTaskBulk_AddTags_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	tag := models.TaskTag{Name: "bug", Color: "#FF0000"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := db.Create(&models.TaskTagAssignment{TaskID: task.ID, TagID: tag.ID}).Error; err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
	r := taskRouter(db)

	body := fmt.Sprintf(
		`{"task_ids":["%s"],"operation":"add_tags","payload":{"tag_ids":[%d]}}`,
		task.ID, tag.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeBulkResp(t, w.Body.Bytes())
	if !resp.succeededHas(task.ID) {
		t.Fatalf("expected succeeded, got %+v", resp.Data)
	}
	var count int64
	db.Model(&models.TaskTagAssignment{}).Where("task_id = ?", task.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 assignment after idempotent add, got %d", count)
	}
}

func TestTaskBulk_RemoveTags_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	tag1 := models.TaskTag{Name: "bug", Color: "#FF0000"}
	tag2 := models.TaskTag{Name: "urgent", Color: "#00FF00"}
	for _, tg := range []*models.TaskTag{&tag1, &tag2} {
		if err := db.Create(tg).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
		if err := db.Create(&models.TaskTagAssignment{TaskID: task.ID, TagID: tg.ID}).Error; err != nil {
			t.Fatalf("seed assignment: %v", err)
		}
	}
	r := taskRouter(db)

	body := fmt.Sprintf(
		`{"task_ids":["%s"],"operation":"remove_tags","payload":{"tag_ids":[%d]}}`,
		task.ID, tag1.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeBulkResp(t, w.Body.Bytes())
	if !resp.succeededHas(task.ID) {
		t.Fatalf("expected succeeded, got %+v", resp.Data)
	}
	var remaining []models.TaskTagAssignment
	db.Where("task_id = ?", task.ID).Find(&remaining)
	if len(remaining) != 1 || remaining[0].TagID != tag2.ID {
		t.Errorf("expected only tag2 remaining, got %+v", remaining)
	}
}

func TestTaskBulk_AddTags_UnknownTagRejected(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	r := taskRouter(db)

	body := fmt.Sprintf(
		`{"task_ids":["%s"],"operation":"add_tags","payload":{"tag_ids":[99999]}}`,
		task.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskBulk_AddTags_EmptyTagIDsRejected(t *testing.T) {
	db := setupTestDB(t)
	r := taskRouter(db)

	body := fmt.Sprintf(
		`{"task_ids":["%s"],"operation":"add_tags","payload":{"tag_ids":[]}}`,
		uuid.New())
	w := doRequest(r, http.MethodPost, "/api/v1/tasks/bulk", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
