package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

func commandRouter(db *gorm.DB) (*gin.Engine, *CommandTracker) {
	r := gin.New()
	tracker := NewCommandTracker()
	h := NewCommandHandler(db, tracker)
	v1 := r.Group("/api/v1")
	RegisterCommandRoutes(v1, h)
	return r, tracker
}

func TestCommand_RunNotConfigured(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)
	r, _ := commandRouter(db)

	body := `{"command":"dev"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/run-command", p.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "dev command not configured for this project" {
		t.Errorf("unexpected error: %v", resp["error"])
	}
}

func TestCommand_RunDeployNotConfigured(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)
	r, _ := commandRouter(db)

	body := `{"command":"deploy"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/run-command", p.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "deploy command not configured for this project" {
		t.Errorf("unexpected error: %v", resp["error"])
	}
}

func TestCommand_RunInvalidCommandType(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)
	r, _ := commandRouter(db)

	body := `{"command":"invalid"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/run-command", p.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCommand_RunProjectNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r, _ := commandRouter(db)

	body := `{"command":"dev"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/run-command", uuid.New()), body)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCommand_RunDevSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	devCmd := "sleep 60"
	p := models.Project{
		Name:           "cmd-test",
		Path:           "/tmp",
		BranchStrategy: "main",
		Active:         true,
		DevCommand:     &devCmd,
	}
	db.Create(&p)

	r, tracker := commandRouter(db)

	body := `{"command":"dev"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/run-command", p.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})

	pid := int(data["pid"].(float64))
	if pid == 0 {
		t.Error("expected non-zero pid")
	}
	if data["command_type"] != "dev" {
		t.Errorf("expected command_type=dev, got %v", data["command_type"])
	}

	// Verify tracked
	tracker.mu.Lock()
	_, exists := tracker.commands[pid]
	tracker.mu.Unlock()
	if !exists {
		t.Error("expected command to be tracked")
	}

	// Clean up: kill the process
	w = doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s/commands/%d", p.ID, pid), "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestCommand_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)
	r, _ := commandRouter(db)

	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/commands", p.ID), "")

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

func TestCommand_KillNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)
	r, _ := commandRouter(db)

	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/projects/%s/commands/99999", p.ID), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCommand_InvalidProjectID(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r, _ := commandRouter(db)

	w := doRequest(r, http.MethodPost, "/api/v1/projects/invalid/run-command", `{"command":"dev"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	w = doRequest(r, http.MethodGet, "/api/v1/projects/invalid/commands", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
