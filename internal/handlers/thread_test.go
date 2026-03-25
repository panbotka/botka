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
