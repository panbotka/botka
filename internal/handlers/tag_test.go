package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

func tagRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewTagHandler(db)
	v1 := r.Group("/api/v1")
	RegisterTagRoutes(v1, h)
	return r
}

func TestTag_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := tagRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/tags", "")

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

func TestTag_CreateWithColor(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := tagRouter(db)
	body := `{"name":"urgent","color":"#FF0000"}`
	w := doRequest(r, http.MethodPost, "/api/v1/tags", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["name"] != "urgent" {
		t.Errorf("expected name=urgent, got %v", data["name"])
	}
	if data["color"] != "#FF0000" {
		t.Errorf("expected color=#FF0000, got %v", data["color"])
	}
}

func TestTag_CreateDefaultColor(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := tagRouter(db)
	body := `{"name":"general","color":""}`
	w := doRequest(r, http.MethodPost, "/api/v1/tags", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["color"] != "#6B7280" {
		t.Errorf("expected default color #6B7280, got %v", data["color"])
	}
}

func TestTag_CreateEmptyName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := tagRouter(db)
	body := `{"name":"","color":"#FF0000"}`
	w := doRequest(r, http.MethodPost, "/api/v1/tags", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTag_CreateLongName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := tagRouter(db)
	longName := strings.Repeat("x", 51)
	body := fmt.Sprintf(`{"name":"%s","color":"#FF0000"}`, longName)
	w := doRequest(r, http.MethodPost, "/api/v1/tags", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTag_CreateDuplicateName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	db.Create(&models.Tag{Name: "dupe", Color: "#000000"})

	r := tagRouter(db)
	body := `{"name":"dupe","color":"#111111"}`
	w := doRequest(r, http.MethodPost, "/api/v1/tags", body)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTag_UpdateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	tag := models.Tag{Name: "old-tag", Color: "#AAAAAA"}
	db.Create(&tag)

	r := tagRouter(db)
	body := `{"name":"new-tag","color":"#BBBBBB"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/tags/%d", tag.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Tag
	db.First(&updated, tag.ID)
	if updated.Name != "new-tag" {
		t.Errorf("expected name=new-tag, got %v", updated.Name)
	}
	if updated.Color != "#BBBBBB" {
		t.Errorf("expected color=#BBBBBB, got %v", updated.Color)
	}
}

func TestTag_UpdateNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := tagRouter(db)
	body := `{"name":"whatever","color":"#000000"}`
	w := doRequest(r, http.MethodPut, "/api/v1/tags/99999", body)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestTag_DeleteSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	tag := models.Tag{Name: "bye", Color: "#000000"}
	db.Create(&tag)

	r := tagRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/tags/%d", tag.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var check models.Tag
	err := db.First(&check, tag.ID).Error
	if err == nil {
		t.Error("expected tag to be deleted")
	}
}

func TestTag_CountThreads(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	tag := models.Tag{Name: "counted", Color: "#123456"}
	db.Create(&tag)

	thread := createTestThread(t, db)

	db.Exec("INSERT INTO thread_tags (thread_id, tag_id) VALUES (?, ?)", thread.ID, tag.ID)

	r := tagRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/tags/%d/threads/count", tag.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	count := int(data["count"].(float64))
	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
}
