package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

func taskTagRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewTaskTagHandler(db)
	v1 := r.Group("/api/v1")
	RegisterTaskTagRoutes(v1, h)
	return r
}

func TestTaskTag_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/task-tags", "")

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

func TestTaskTag_CreateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/task-tags", `{"name":"bug","color":"#FF5733"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["name"] != "bug" {
		t.Errorf("expected name=bug, got %v", data["name"])
	}
	if data["color"] != "#FF5733" {
		t.Errorf("expected color=#FF5733, got %v", data["color"])
	}
}

func TestTaskTag_CreateDefaultColor(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/task-tags", `{"name":"general"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["color"] != defaultTaskTagColor {
		t.Errorf("expected color=%s, got %v", defaultTaskTagColor, data["color"])
	}
}

func TestTaskTag_CreateInvalidColor(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/task-tags", `{"name":"bug","color":"red"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTaskTag_CreateEmptyName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/task-tags", `{"name":"","color":"#FF0000"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTaskTag_CreateDuplicateNameCaseInsensitive(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	if err := db.Create(&models.TaskTag{Name: "Bug", Color: "#FF0000"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/task-tags", `{"name":"BUG","color":"#000000"}`)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskTag_PatchRename(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	tag := models.TaskTag{Name: "old", Color: "#AAAAAA"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/task-tags/%d", tag.ID), `{"name":"new"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got models.TaskTag
	if err := db.First(&got, tag.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Name != "new" {
		t.Errorf("expected name=new, got %s", got.Name)
	}
	if got.Color != "#AAAAAA" {
		t.Errorf("expected color preserved, got %s", got.Color)
	}
}

func TestTaskTag_PatchInvalidColor(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	tag := models.TaskTag{Name: "bug", Color: "#AAAAAA"}
	db.Create(&tag)

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/task-tags/%d", tag.ID), `{"color":"not-a-color"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTaskTag_DeleteCascadesAssignments(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	tag := models.TaskTag{Name: "bug", Color: "#FF0000"}
	db.Create(&tag)
	db.Create(&models.TaskTagAssignment{TaskID: task.ID, TagID: tag.ID})

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/task-tags/%d", tag.ID), "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.TaskTagAssignment{}).Where("task_id = ?", task.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected assignment cascade-deleted, got %d remaining", count)
	}
}

func TestTaskTag_AssignReplacesExistingTags(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	bug := models.TaskTag{Name: "bug", Color: "#FF0000"}
	feature := models.TaskTag{Name: "feature", Color: "#00FF00"}
	chore := models.TaskTag{Name: "chore", Color: "#0000FF"}
	db.Create(&bug)
	db.Create(&feature)
	db.Create(&chore)
	// Existing assignment that must be cleared by the replace semantics.
	db.Create(&models.TaskTagAssignment{TaskID: task.ID, TagID: chore.ID})

	r := taskTagRouter(db)
	body := fmt.Sprintf(`{"tag_ids":[%d,%d]}`, bug.ID, feature.ID)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/tags", task.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var assignments []models.TaskTagAssignment
	db.Where("task_id = ?", task.ID).Find(&assignments)
	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(assignments))
	}
	gotIDs := map[int64]bool{}
	for _, a := range assignments {
		gotIDs[a.TagID] = true
	}
	if !gotIDs[bug.ID] || !gotIDs[feature.ID] {
		t.Errorf("expected bug+feature tags, got %v", gotIDs)
	}
}

func TestTaskTag_AssignClearsAll(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	tag := models.TaskTag{Name: "bug", Color: "#FF0000"}
	db.Create(&tag)
	db.Create(&models.TaskTagAssignment{TaskID: task.ID, TagID: tag.ID})

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/tags", task.ID), `{"tag_ids":[]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var count int64
	db.Model(&models.TaskTagAssignment{}).Where("task_id = ?", task.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 assignments after clear, got %d", count)
	}
}

func TestTaskTag_AssignUnknownTagRejected(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)

	r := taskTagRouter(db)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/tasks/%s/tags", task.ID), `{"tag_ids":[99999]}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskList_FilterByTagID_AllRequiredTags(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	t1 := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	t2 := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	t3 := createTestTask(t, db, proj.ID, models.TaskStatusPending)

	bug := models.TaskTag{Name: "bug", Color: "#FF0000"}
	feat := models.TaskTag{Name: "feature", Color: "#00FF00"}
	db.Create(&bug)
	db.Create(&feat)
	// t1: bug + feature, t2: bug only, t3: no tags
	db.Create(&[]models.TaskTagAssignment{
		{TaskID: t1.ID, TagID: bug.ID},
		{TaskID: t1.ID, TagID: feat.ID},
		{TaskID: t2.ID, TagID: bug.ID},
	})
	_ = t3 // unused except as a control

	r := taskRouter(db)

	// Single tag: should return t1 and t2
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tasks?tag_id=%d", bug.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("single-tag filter: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if total := int(resp["total"].(float64)); total != 2 {
		t.Errorf("single-tag total: expected 2, got %d", total)
	}

	// Both tags (intersection): should return only t1
	w = doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tasks?tag_id=%d&tag_id=%d", bug.ID, feat.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("multi-tag filter: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if total := int(resp["total"].(float64)); total != 1 {
		t.Errorf("multi-tag total: expected 1, got %d", total)
	}
	data := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("multi-tag data: expected 1 result, got %d", len(data))
	}
	first := data[0].(map[string]interface{})
	if first["id"] != t1.ID.String() {
		t.Errorf("expected task %s, got %v", t1.ID, first["id"])
	}
}

func TestTaskList_IncludesTagsInResponse(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusPending)
	bug := models.TaskTag{Name: "bug", Color: "#FF0000"}
	db.Create(&bug)
	db.Create(&models.TaskTagAssignment{TaskID: task.ID, TagID: bug.ID})

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
	tags, ok := first["tags"].([]interface{})
	if !ok {
		t.Fatalf("expected tags field, got %v", first["tags"])
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	tagJSON := tags[0].(map[string]interface{})
	if tagJSON["name"] != "bug" {
		t.Errorf("expected tag name=bug, got %v", tagJSON["name"])
	}
}

func TestTaskList_InvalidTagIDRejected(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := taskRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/tasks?tag_id=abc", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
