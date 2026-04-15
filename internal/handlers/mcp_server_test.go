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

func mcpServerRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewMCPServerHandler(db)
	v1 := r.Group("/api/v1")
	RegisterMCPServerRoutes(v1, h)
	return r
}

func TestMCPServer_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpServerRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/mcp-servers", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty list, got %d items", len(data))
	}
	total := resp["total"].(float64)
	if total != 0 {
		t.Errorf("expected total=0, got %v", total)
	}
}

func TestMCPServer_ListWithData(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	db.Create(&models.MCPServer{
		Name:       "Bravo",
		ServerType: models.MCPServerTypeStdio,
		Config:     json.RawMessage(`{"command":"bravo"}`),
		Active:     true,
	})
	db.Create(&models.MCPServer{
		Name:       "Alpha",
		ServerType: models.MCPServerTypeSSE,
		Config:     json.RawMessage(`{"url":"http://alpha"}`),
		Active:     true,
	})

	r := mcpServerRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/mcp-servers", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(data))
	}
	total := resp["total"].(float64)
	if total != 2 {
		t.Errorf("expected total=2, got %v", total)
	}
	first := data[0].(map[string]interface{})
	if first["name"] != "Alpha" {
		t.Errorf("expected first item to be Alpha (ordered by name), got %v", first["name"])
	}
}

func TestMCPServer_CreateStdio(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpServerRouter(db)
	body := `{"name":"My Stdio Server","server_type":"stdio","config":{"command":"node","args":["server.js"],"env":{"DEBUG":"true"}}}`
	w := doRequest(r, http.MethodPost, "/api/v1/mcp-servers", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["name"] != "My Stdio Server" {
		t.Errorf("expected name='My Stdio Server', got %v", data["name"])
	}
	if data["server_type"] != "stdio" {
		t.Errorf("expected server_type=stdio, got %v", data["server_type"])
	}
	if data["active"] != true {
		t.Errorf("expected active=true, got %v", data["active"])
	}
	if data["is_default"] != false {
		t.Errorf("expected is_default=false, got %v", data["is_default"])
	}
}

func TestMCPServer_CreateSSE(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpServerRouter(db)
	body := `{"name":"My SSE Server","server_type":"sse","config":{"url":"http://localhost:3000/sse","headers":{"Authorization":"Bearer token"}}}`
	w := doRequest(r, http.MethodPost, "/api/v1/mcp-servers", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["name"] != "My SSE Server" {
		t.Errorf("expected name='My SSE Server', got %v", data["name"])
	}
	if data["server_type"] != "sse" {
		t.Errorf("expected server_type=sse, got %v", data["server_type"])
	}
}

func TestMCPServer_CreateMissingName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpServerRouter(db)
	body := `{"server_type":"stdio","config":{"command":"test"}}`
	w := doRequest(r, http.MethodPost, "/api/v1/mcp-servers", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMCPServer_CreateDuplicateName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	db.Create(&models.MCPServer{
		Name:       "Existing",
		ServerType: models.MCPServerTypeStdio,
		Config:     json.RawMessage(`{"command":"test"}`),
		Active:     true,
	})

	r := mcpServerRouter(db)
	body := `{"name":"Existing","server_type":"stdio","config":{"command":"other"}}`
	w := doRequest(r, http.MethodPost, "/api/v1/mcp-servers", body)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMCPServer_CreateInvalidServerType(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpServerRouter(db)
	body := `{"name":"Bad Type","server_type":"websocket","config":{"command":"test"}}`
	w := doRequest(r, http.MethodPost, "/api/v1/mcp-servers", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMCPServer_CreateStdioMissingCommand(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpServerRouter(db)
	body := `{"name":"No Command","server_type":"stdio","config":{"args":["a"]}}`
	w := doRequest(r, http.MethodPost, "/api/v1/mcp-servers", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMCPServer_CreateSSEMissingURL(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpServerRouter(db)
	body := `{"name":"No URL","server_type":"sse","config":{"headers":{}}}`
	w := doRequest(r, http.MethodPost, "/api/v1/mcp-servers", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMCPServer_UpdateName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	s := models.MCPServer{
		Name:       "Old Name",
		ServerType: models.MCPServerTypeStdio,
		Config:     json.RawMessage(`{"command":"test"}`),
		Active:     true,
	}
	db.Create(&s)

	r := mcpServerRouter(db)
	body := `{"name":"New Name"}`
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/mcp-servers/%d", s.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["name"] != "New Name" {
		t.Errorf("expected name='New Name', got %v", data["name"])
	}
	if data["server_type"] != "stdio" {
		t.Errorf("expected server_type unchanged, got %v", data["server_type"])
	}
}

func TestMCPServer_UpdateToggleIsDefault(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	s := models.MCPServer{
		Name:       "Server",
		ServerType: models.MCPServerTypeStdio,
		Config:     json.RawMessage(`{"command":"test"}`),
		Active:     true,
		IsDefault:  false,
	}
	db.Create(&s)

	r := mcpServerRouter(db)
	body := `{"is_default":true}`
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/mcp-servers/%d", s.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["is_default"] != true {
		t.Errorf("expected is_default=true, got %v", data["is_default"])
	}
}

func TestMCPServer_UpdateToggleActive(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	s := models.MCPServer{
		Name:       "Server",
		ServerType: models.MCPServerTypeStdio,
		Config:     json.RawMessage(`{"command":"test"}`),
		Active:     true,
	}
	db.Create(&s)

	r := mcpServerRouter(db)
	body := `{"active":false}`
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/mcp-servers/%d", s.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["active"] != false {
		t.Errorf("expected active=false, got %v", data["active"])
	}
}

func TestMCPServer_UpdateNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpServerRouter(db)
	body := `{"name":"New Name"}`
	w := doRequest(r, http.MethodPatch, "/api/v1/mcp-servers/99999", body)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMCPServer_DeleteExisting(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	s := models.MCPServer{
		Name:       "ToDelete",
		ServerType: models.MCPServerTypeStdio,
		Config:     json.RawMessage(`{"command":"test"}`),
		Active:     true,
	}
	db.Create(&s)

	r := mcpServerRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/mcp-servers/%d", s.ID), "")

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	var check models.MCPServer
	if err := db.First(&check, s.ID).Error; err == nil {
		t.Error("expected MCP server to be deleted")
	}
}

func TestMCPServer_DeleteNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpServerRouter(db)
	w := doRequest(r, http.MethodDelete, "/api/v1/mcp-servers/99999", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
