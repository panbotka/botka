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

func threadRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewThreadHandler(db, "sonnet", []string{"sonnet", "opus", "haiku"})
	v1 := r.Group("/api/v1")
	RegisterThreadRoutes(v1, h)
	return r
}

func TestThread_CreateDefault(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := threadRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/threads", "{}")

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["title"] != "New Chat" {
		t.Errorf("expected title=New Chat, got %v", data["title"])
	}
	if data["model"] != "sonnet" {
		t.Errorf("expected model=sonnet, got %v", data["model"])
	}
}

func TestThread_CreateWithPersona(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	persona := models.Persona{Name: "Code Helper", SystemPrompt: "You are a coding assistant."}
	db.Create(&persona)

	r := threadRouter(db)
	body := fmt.Sprintf(`{"persona_id":%d}`, persona.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/threads", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["title"] != "Code Helper" {
		t.Errorf("expected title=Code Helper (persona name), got %v", data["title"])
	}
	if data["persona_name"] != "Code Helper" {
		t.Errorf("expected persona_name=Code Helper, got %v", data["persona_name"])
	}
}

func TestThread_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := threadRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/threads", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty list, got %d items", len(data))
	}
}

func TestThread_ListExcludesArchived(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th1 := createTestThread(t, db)
	_ = th1

	model := "sonnet"
	archived := models.Thread{Title: "archived thread", Model: &model, Archived: true}
	db.Create(&archived)

	r := threadRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/threads", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("expected 1 non-archived thread, got %d", len(data))
	}
	title := data[0].(map[string]interface{})["title"].(string)
	if title != "test thread" {
		t.Errorf("expected test thread, got %s", title)
	}
}

func TestThread_ListWithArchivedFlag(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	createTestThread(t, db)

	model := "sonnet"
	archived := models.Thread{Title: "archived thread", Model: &model, Archived: true}
	db.Create(&archived)

	r := threadRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/threads?archived=true", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 2 {
		t.Fatalf("expected 2 threads (including archived), got %d", len(data))
	}
}

func TestThread_RenameSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	r := threadRouter(db)
	body := `{"title":"Renamed Thread"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d", th.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, th.ID)
	if updated.Title != "Renamed Thread" {
		t.Errorf("expected title=Renamed Thread, got %s", updated.Title)
	}
}

func TestThread_RenameEmptyTitle(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	r := threadRouter(db)
	body := `{"title":""}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d", th.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestThread_DeleteSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	r := threadRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/threads/%d", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var check models.Thread
	err := db.First(&check, th.ID).Error
	if err == nil {
		t.Error("expected thread to be deleted")
	}
}

func TestThread_PinSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	r := threadRouter(db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/pin", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, th.ID)
	if !updated.Pinned {
		t.Error("expected thread to be pinned")
	}
}

func TestThread_UnpinSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)
	db.Model(&models.Thread{}).Where("id = ?", th.ID).Update("pinned", true)

	r := threadRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/threads/%d/pin", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, th.ID)
	if updated.Pinned {
		t.Error("expected thread to be unpinned")
	}
}

func TestThread_ArchiveAlsoUnpins(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)
	db.Model(&models.Thread{}).Where("id = ?", th.ID).Update("pinned", true)

	r := threadRouter(db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/archive", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, th.ID)
	if !updated.Archived {
		t.Error("expected thread to be archived")
	}
	if updated.Pinned {
		t.Error("expected thread to be unpinned after archive")
	}
}

func TestThread_UpdateModel(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	r := threadRouter(db)
	body := `{"model":"opus"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/model", th.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, th.ID)
	if updated.Model == nil || *updated.Model != "opus" {
		t.Errorf("expected model=opus, got %v", updated.Model)
	}
}

func TestThread_GetByID_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	r := threadRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	thread := data["thread"].(map[string]interface{})
	if thread["title"] != "test thread" {
		t.Errorf("expected title=test thread, got %v", thread["title"])
	}
}

func TestThread_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := threadRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/threads/99999", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_GetByID_InvalidID(t *testing.T) {
	db := setupTestDB(t)

	r := threadRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/threads/not-a-number", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_Unarchive(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	// Archive first.
	db.Model(&models.Thread{}).Where("id = ?", th.ID).Update("archived", true)

	r := threadRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/threads/%d/archive", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, th.ID)
	if updated.Archived {
		t.Error("expected thread to be unarchived")
	}
}

func TestThread_SetProject(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)
	proj := createTestProject(t, db)

	r := threadRouter(db)
	body := fmt.Sprintf(`{"project_id":"%s"}`, proj.ID)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/project", th.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, th.ID)
	if updated.ProjectID == nil || *updated.ProjectID != proj.ID {
		t.Errorf("expected project_id=%s, got %v", proj.ID, updated.ProjectID)
	}
}

func TestThread_Usage(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	// Create messages with token usage.
	promptTokens := 100
	completionTokens := 50
	cost := 0.01
	msg := models.Message{
		ThreadID:         th.ID,
		Role:             "assistant",
		Content:          "test response",
		PromptTokens:     &promptTokens,
		CompletionTokens: &completionTokens,
		CostUSD:          &cost,
	}
	db.Create(&msg)

	r := threadRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/usage", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			TotalPromptTokens     int64   `json:"total_prompt_tokens"`
			TotalCompletionTokens int64   `json:"total_completion_tokens"`
			TotalTokens           int64   `json:"total_tokens"`
			TotalCostUSD          float64 `json:"total_cost_usd"`
			MessageCount          int64   `json:"message_count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.TotalPromptTokens != 100 {
		t.Errorf("expected prompt_tokens=100, got %d", resp.Data.TotalPromptTokens)
	}
	if resp.Data.TotalCompletionTokens != 50 {
		t.Errorf("expected completion_tokens=50, got %d", resp.Data.TotalCompletionTokens)
	}
	if resp.Data.TotalTokens != 150 {
		t.Errorf("expected total_tokens=150, got %d", resp.Data.TotalTokens)
	}
	if resp.Data.MessageCount != 1 {
		t.Errorf("expected message_count=1, got %d", resp.Data.MessageCount)
	}
}

func TestThread_ClearMessages(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	// Create a message.
	db.Create(&models.Message{ThreadID: th.ID, Role: "user", Content: "hello"})

	r := threadRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/threads/%d/messages", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify messages are gone.
	var count int64
	db.Model(&models.Message{}).Where("thread_id = ?", th.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 messages after clear, got %d", count)
	}
}

func TestThread_SetTags(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	// Create tags.
	tag1 := models.Tag{Name: "tag1", Color: "#ff0000"}
	tag2 := models.Tag{Name: "tag2", Color: "#00ff00"}
	db.Create(&tag1)
	db.Create(&tag2)

	r := threadRouter(db)
	body := fmt.Sprintf(`{"tag_ids":[%d,%d]}`, tag1.ID, tag2.ID)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/tags", th.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify tags are set.
	var count int64
	db.Raw("SELECT COUNT(*) FROM thread_tags WHERE thread_id = ?", th.ID).Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 thread_tags, got %d", count)
	}
}

func TestThread_PinIdempotent(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)
	db.Model(&models.Thread{}).Where("id = ?", th.ID).Update("pinned", true)

	r := threadRouter(db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/pin", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for already-pinned thread, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, th.ID)
	if !updated.Pinned {
		t.Error("expected thread to remain pinned")
	}
}

func TestThread_PinIdempotentAtLimit(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := threadRouter(db)

	// Pin 10 threads.
	var lastPinned models.Thread
	for i := 0; i < 10; i++ {
		th := createTestThread(t, db)
		doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/pin", th.ID), "")
		lastPinned = th
	}

	// Re-pinning one that's already pinned should succeed (idempotent).
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/pin", lastPinned.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for re-pin at limit, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_PinNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := threadRouter(db)
	w := doRequest(r, http.MethodPut, "/api/v1/threads/99999/pin", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent thread, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_PinLimit(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := threadRouter(db)

	// Pin 10 threads.
	for i := 0; i < 10; i++ {
		th := createTestThread(t, db)
		w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/pin", th.ID), "")
		if w.Code != http.StatusOK {
			t.Fatalf("pin thread %d: expected 200, got %d: %s", i+1, w.Code, w.Body.String())
		}
	}

	// 11th pin should fail.
	th := createTestThread(t, db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/pin", th.ID), "")

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for 11th pin, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_RenameTitleTooLong(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	r := threadRouter(db)
	longTitle := `{"title":"` + string(make([]byte, maxTitleLength+1)) + `"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d", th.ID), longTitle)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for long title, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_UpdateModelInvalid(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	r := threadRouter(db)
	body := `{"model":"invalid-model"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/model", th.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid model, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_UpdateCustomContext(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := models.Thread{Title: "Test"}
	db.Create(&thread)

	r := threadRouter(db)
	body := `{"custom_context":"API docs: POST /users creates a user"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/custom-context", thread.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it was saved
	var updated models.Thread
	db.First(&updated, thread.ID)
	if updated.CustomContext != "API docs: POST /users creates a user" {
		t.Errorf("expected custom context to be saved, got %q", updated.CustomContext)
	}
}

func TestThread_UpdateCustomContextEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := models.Thread{Title: "Test", CustomContext: "old content"}
	db.Create(&thread)

	r := threadRouter(db)
	body := `{"custom_context":""}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/custom-context", thread.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, thread.ID)
	if updated.CustomContext != "" {
		t.Errorf("expected empty custom context, got %q", updated.CustomContext)
	}
}

func TestThread_UpdateCustomContextNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := threadRouter(db)
	body := `{"custom_context":"some content"}`
	w := doRequest(r, http.MethodPut, "/api/v1/threads/99999/custom-context", body)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_UpdateModelEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)

	r := threadRouter(db)
	body := `{"model":""}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/model", th.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty model, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_CreateWithProject(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createTestProject(t, db)

	r := threadRouter(db)
	body := fmt.Sprintf(`{"project_id":"%s"}`, proj.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/threads", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["project_id"] == nil {
		t.Error("expected project_id to be set")
	}
}
