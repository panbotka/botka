package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

func taskNoteRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewTaskNoteHandler(db)
	v1 := r.Group("/api/v1")
	RegisterTaskNoteRoutes(v1, h)
	return r
}

func TestTaskNote_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)

	r := taskNoteRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%s/notes", task.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty list, got %d items", len(data))
	}
}

func TestTaskNote_ListUnknownTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskNoteRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%s/notes", uuid.New()), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskNote_CreateAndList(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)

	r := taskNoteRouter(db)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/notes", task.ID),
		`{"body":"flaky test, retry tomorrow"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	created := createResp["data"].(map[string]interface{})
	if created["body"] != "flaky test, retry tomorrow" {
		t.Errorf("unexpected body: %v", created["body"])
	}
	if created["author"] != "user" {
		t.Errorf("expected author=user, got %v", created["author"])
	}

	// Add a second note so we can verify ordering.
	doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/notes", task.ID),
		`{"body":"second note"}`)

	w = doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%s/notes", task.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var listResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	notes := listResp["data"].([]interface{})
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}
	first := notes[0].(map[string]interface{})
	if first["body"] != "flaky test, retry tomorrow" {
		t.Errorf("expected oldest note first, got %v", first["body"])
	}
}

func TestTaskNote_CreateRejectsEmptyBody(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)

	r := taskNoteRouter(db)
	for _, body := range []string{`{"body":""}`, `{"body":"   "}`, `{}`} {
		w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/notes", task.ID), body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body=%q expected 400, got %d", body, w.Code)
		}
	}
}

func TestTaskNote_CreateRejectsTooLongBody(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)

	r := taskNoteRouter(db)
	body := fmt.Sprintf(`{"body":%q}`, strings.Repeat("a", maxNoteBodyLength+1))
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/notes", task.ID), body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskNote_CreateUnknownTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskNoteRouter(db)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/notes", uuid.New()), `{"body":"hi"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskNote_PatchUpdatesBody(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	note := models.TaskNote{TaskID: task.ID, Body: "old", Author: "user"}
	if err := db.Create(&note).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := taskNoteRouter(db)
	w := doRequest(r, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%s/notes/%d", task.ID, note.ID),
		`{"body":"new body"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got models.TaskNote
	if err := db.First(&got, note.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Body != "new body" {
		t.Errorf("expected body=new body, got %q", got.Body)
	}
	if !got.UpdatedAt.After(got.CreatedAt) && !got.UpdatedAt.Equal(got.CreatedAt) {
		t.Errorf("updated_at should be >= created_at, got created=%v updated=%v",
			got.CreatedAt, got.UpdatedAt)
	}
}

func TestTaskNote_PatchUnknownNote(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)

	r := taskNoteRouter(db)
	w := doRequest(r, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%s/notes/9999", task.ID), `{"body":"x"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestTaskNote_PatchRejectsCrossTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	taskA := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	taskB := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	note := models.TaskNote{TaskID: taskA.ID, Body: "x", Author: "user"}
	db.Create(&note)

	r := taskNoteRouter(db)
	// Use taskB's id with note's id — should not find.
	w := doRequest(r, http.MethodPatch,
		fmt.Sprintf("/api/v1/tasks/%s/notes/%d", taskB.ID, note.ID),
		`{"body":"hijack"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestTaskNote_DeleteSoftDeletes(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	note := models.TaskNote{TaskID: task.ID, Body: "x", Author: "user"}
	db.Create(&note)

	r := taskNoteRouter(db)
	w := doRequest(r, http.MethodDelete,
		fmt.Sprintf("/api/v1/tasks/%s/notes/%d", task.ID, note.ID), "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// List excludes soft-deleted note.
	w = doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tasks/%s/notes", task.ID), "")
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	notes := resp["data"].([]interface{})
	if len(notes) != 0 {
		t.Errorf("expected 0 notes after delete, got %d", len(notes))
	}

	// Row still exists with deleted_at set.
	var raw struct {
		DeletedAt *string
	}
	db.Raw("SELECT deleted_at FROM task_notes WHERE id = ?", note.ID).Scan(&raw)
	if raw.DeletedAt == nil {
		t.Error("expected deleted_at to be set, got nil")
	}
}

func TestTaskNote_DeleteCascadesWithTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	note := models.TaskNote{TaskID: task.ID, Body: "x", Author: "user"}
	if err := db.Create(&note).Error; err != nil {
		t.Fatalf("seed note: %v", err)
	}

	if err := db.Unscoped().Delete(&task).Error; err != nil {
		t.Fatalf("delete task: %v", err)
	}

	var count int64
	db.Unscoped().Model(&models.TaskNote{}).
		Where("id = ?", note.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected ON DELETE CASCADE to remove note, %d remain", count)
	}
}

func TestTaskList_NotesCountIncludesOnlyLiveNotes(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)

	live := models.TaskNote{TaskID: task.ID, Body: "live", Author: "user"}
	dead := models.TaskNote{TaskID: task.ID, Body: "dead", Author: "user"}
	db.Create(&live)
	db.Create(&dead)
	if err := db.Delete(&dead).Error; err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	r := taskRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/tasks", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("expected 1 task, got %d", len(data))
	}
	first := data[0].(map[string]interface{})
	if got := first["notes_count"].(float64); got != 1 {
		t.Errorf("expected notes_count=1, got %v", got)
	}
}
