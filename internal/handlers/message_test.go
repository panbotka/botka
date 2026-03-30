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

func messageRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewMessageHandler(db)
	v1 := r.Group("/api/v1")
	RegisterMessageRoutes(v1, h)
	return r
}

func TestMessage_ToggleHideSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	msg := createMessage(t, db, thread.ID, nil, "user", "hello")

	r := messageRouter(db)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/messages/%d/hide", msg.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["hidden"] != true {
		t.Errorf("expected hidden=true, got %v", data["hidden"])
	}

	// Verify DB was updated.
	var updated models.Message
	db.First(&updated, msg.ID)
	if !updated.Hidden {
		t.Error("expected message to be hidden in DB")
	}
}

func TestMessage_ToggleHideUnhide(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	msg := createMessage(t, db, thread.ID, nil, "assistant", "response")

	r := messageRouter(db)

	// First toggle: hide.
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/messages/%d/hide", msg.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("hide: expected 200, got %d", w.Code)
	}

	// Second toggle: unhide.
	w = doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/messages/%d/hide", msg.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("unhide: expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["hidden"] != false {
		t.Errorf("expected hidden=false after double toggle, got %v", data["hidden"])
	}

	var updated models.Message
	db.First(&updated, msg.ID)
	if updated.Hidden {
		t.Error("expected message to be unhidden in DB after double toggle")
	}
}

func TestMessage_ToggleHideNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := messageRouter(db)
	w := doRequest(r, http.MethodPatch, "/api/v1/messages/99999/hide", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMessage_ToggleHideInvalidID(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := messageRouter(db)
	w := doRequest(r, http.MethodPatch, "/api/v1/messages/abc/hide", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
