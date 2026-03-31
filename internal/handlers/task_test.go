package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/runner"
)

func taskRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewTaskHandler(db, runner.NewTaskEventHub())
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

func TestValidateUpdate_RunningBlocked(t *testing.T) {
	task := models.Task{Status: models.TaskStatusRunning}
	title := "new title"
	req := updateTaskRequest{Title: &title}
	if msg := validateUpdate(task, req); msg != "cannot update a running task" {
		t.Fatalf("expected running blocked, got %q", msg)
	}
}

func TestValidateUpdate_InvalidTransition(t *testing.T) {
	task := models.Task{Status: models.TaskStatusDone}
	s := models.TaskStatusQueued
	req := updateTaskRequest{Status: &s}
	if msg := validateUpdate(task, req); msg != "invalid status transition" {
		t.Fatalf("expected invalid transition, got %q", msg)
	}
}

func TestValidateUpdate_ValidTransition(t *testing.T) {
	task := models.Task{Status: models.TaskStatusPending}
	s := models.TaskStatusQueued
	req := updateTaskRequest{Status: &s}
	if msg := validateUpdate(task, req); msg != "" {
		t.Fatalf("expected no error, got %q", msg)
	}
}

func TestBuildTaskUpdates_NilFieldsOmitted(t *testing.T) {
	req := updateTaskRequest{}
	updates := buildTaskUpdates(req)
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
	updates := buildTaskUpdates(req)
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

func TestTaskUpdate_RunningConflict(t *testing.T) {
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

func TestTaskUpdate_InvalidTransition(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusDone)
	r := taskRouter(db)

	body := `{"status":"pending"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/tasks/%s", task.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
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
