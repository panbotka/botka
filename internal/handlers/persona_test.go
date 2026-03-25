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

func personaRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewPersonaHandler(db)
	v1 := r.Group("/api/v1")
	RegisterPersonaRoutes(v1, h)
	return r
}

func TestPersona_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := personaRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/personas", "")

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

func TestPersona_CreateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := personaRouter(db)
	body := `{"name":"Assistant","system_prompt":"You are helpful."}`
	w := doRequest(r, http.MethodPost, "/api/v1/personas", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["name"] != "Assistant" {
		t.Errorf("expected name=Assistant, got %v", data["name"])
	}
	if data["system_prompt"] != "You are helpful." {
		t.Errorf("expected system_prompt='You are helpful.', got %v", data["system_prompt"])
	}
}

func TestPersona_CreateMissingName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := personaRouter(db)
	body := `{"system_prompt":"You are helpful."}`
	w := doRequest(r, http.MethodPost, "/api/v1/personas", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPersona_CreateWithOptionalFields(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := personaRouter(db)
	body := `{"name":"Coder","system_prompt":"Code expert","default_model":"opus","icon":"🤖","starter_message":"Hello!","sort_order":5}`
	w := doRequest(r, http.MethodPost, "/api/v1/personas", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["default_model"] != "opus" {
		t.Errorf("expected default_model=opus, got %v", data["default_model"])
	}
	if data["starter_message"] != "Hello!" {
		t.Errorf("expected starter_message=Hello!, got %v", data["starter_message"])
	}
	if int(data["sort_order"].(float64)) != 5 {
		t.Errorf("expected sort_order=5, got %v", data["sort_order"])
	}
}

func TestPersona_UpdateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := models.Persona{Name: "Old Name", SystemPrompt: "old"}
	db.Create(&p)

	r := personaRouter(db)
	body := `{"name":"New Name","system_prompt":"new prompt"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/personas/%d", p.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["name"] != "New Name" {
		t.Errorf("expected name=New Name, got %v", data["name"])
	}
}

func TestPersona_UpdateNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := personaRouter(db)
	body := `{"name":"Name","system_prompt":"prompt"}`
	w := doRequest(r, http.MethodPut, "/api/v1/personas/99999", body)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPersona_DeleteSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := models.Persona{Name: "ToDelete", SystemPrompt: "bye"}
	db.Create(&p)

	r := personaRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/personas/%d", p.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var check models.Persona
	err := db.First(&check, p.ID).Error
	if err == nil {
		t.Error("expected persona to be deleted")
	}
}

func TestPersona_ListOrderedBySortOrder(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	db.Create(&models.Persona{Name: "Charlie", SystemPrompt: "c", SortOrder: 3})
	db.Create(&models.Persona{Name: "Alpha", SystemPrompt: "a", SortOrder: 1})
	db.Create(&models.Persona{Name: "Bravo", SystemPrompt: "b", SortOrder: 2})

	r := personaRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/personas", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 3 {
		t.Fatalf("expected 3 personas, got %d", len(data))
	}

	names := make([]string, len(data))
	for i, item := range data {
		names[i] = item.(map[string]interface{})["name"].(string)
	}
	expected := []string{"Alpha", "Bravo", "Charlie"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], name)
		}
	}
}
