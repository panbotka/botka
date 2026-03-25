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

func memoryRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewMemoryHandler(db)
	v1 := r.Group("/api/v1")
	RegisterMemoryRoutes(v1, h)
	return r
}

func TestMemory_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := memoryRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/memories", "")

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

func TestMemory_CreateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := memoryRouter(db)
	body := `{"content":"Remember this fact."}`
	w := doRequest(r, http.MethodPost, "/api/v1/memories", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["content"] != "Remember this fact." {
		t.Errorf("expected content='Remember this fact.', got %v", data["content"])
	}
	if data["id"] == nil || data["id"] == "" {
		t.Error("expected non-empty UUID id")
	}
}

func TestMemory_CreateEmptyContent(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := memoryRouter(db)
	body := `{"content":""}`
	w := doRequest(r, http.MethodPost, "/api/v1/memories", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMemory_CreateAtLimit(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	for i := 0; i < 100; i++ {
		m := models.Memory{Content: fmt.Sprintf("memory %d", i)}
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to create memory %d: %v", i, err)
		}
	}

	r := memoryRouter(db)
	body := `{"content":"one too many"}`
	w := doRequest(r, http.MethodPost, "/api/v1/memories", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	errMsg := resp["error"].(string)
	if errMsg != "memory limit reached (max 100)" {
		t.Errorf("expected limit error, got %v", errMsg)
	}
}

func TestMemory_UpdateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	m := models.Memory{Content: "old content"}
	db.Create(&m)

	r := memoryRouter(db)
	body := `{"content":"updated content"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/memories/%s", m.ID.String()), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["content"] != "updated content" {
		t.Errorf("expected content='updated content', got %v", data["content"])
	}
}

func TestMemory_UpdateNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := memoryRouter(db)
	body := `{"content":"something"}`
	w := doRequest(r, http.MethodPut, "/api/v1/memories/00000000-0000-0000-0000-000000000000", body)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMemory_DeleteSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	m := models.Memory{Content: "to delete"}
	db.Create(&m)

	r := memoryRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/memories/%s", m.ID.String()), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var check models.Memory
	err := db.First(&check, "id = ?", m.ID).Error
	if err == nil {
		t.Error("expected memory to be deleted")
	}
}

func TestMemory_CreateAndListNewestFirst(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := memoryRouter(db)

	// Create three memories in order
	for _, content := range []string{"first", "second", "third"} {
		body := fmt.Sprintf(`{"content":"%s"}`, content)
		w := doRequest(r, http.MethodPost, "/api/v1/memories", body)
		if w.Code != http.StatusCreated {
			t.Fatalf("failed to create memory %s: %d", content, w.Code)
		}
	}

	w := doRequest(r, http.MethodGet, "/api/v1/memories", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(data))
	}

	// Newest first means "third" should be at index 0
	first := data[0].(map[string]interface{})["content"].(string)
	last := data[2].(map[string]interface{})["content"].(string)
	if first != "third" {
		t.Errorf("expected newest memory first ('third'), got %q", first)
	}
	if last != "first" {
		t.Errorf("expected oldest memory last ('first'), got %q", last)
	}
}
