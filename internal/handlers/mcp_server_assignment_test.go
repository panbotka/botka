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

func mcpAssignRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewMCPServerAssignmentHandler(db)
	v1 := r.Group("/api/v1")
	RegisterMCPServerAssignmentRoutes(v1, h)
	return r
}

func createMCPServer(t *testing.T, db *gorm.DB, name string, isDefault bool, active bool) models.MCPServer {
	t.Helper()
	s := models.MCPServer{
		Name:       name,
		ServerType: models.MCPServerTypeStdio,
		Config:     json.RawMessage(`{"command":"test"}`),
		IsDefault:  isDefault,
		Active:     true,
	}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("create MCP server %q: %v", name, err)
	}
	if !active {
		db.Model(&s).Update("active", false)
		s.Active = false
	}
	return s
}

func parseMCPAssignResponse(t *testing.T, body []byte) []map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := resp["data"].([]interface{})
	result := make([]map[string]interface{}, len(data))
	for i, item := range data {
		result[i] = item.(map[string]interface{})
	}
	return result
}

// --- Thread MCP Server Tests ---

func TestThreadMCPServers_ListDefaultsOnly(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	def := createMCPServer(t, db, "default-server", true, true)
	nonDef := createMCPServer(t, db, "optional-server", false, true)

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/mcp-servers", thread.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	if len(items) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(items))
	}

	// Ordered by name: "default-server" then "optional-server"
	if items[0]["name"] != def.Name {
		t.Errorf("expected first=%q, got %q", def.Name, items[0]["name"])
	}
	if items[0]["enabled"] != true {
		t.Errorf("default server should be enabled")
	}
	if items[1]["name"] != nonDef.Name {
		t.Errorf("expected second=%q, got %q", nonDef.Name, items[1]["name"])
	}
	if items[1]["enabled"] != false {
		t.Errorf("non-default server should not be enabled")
	}
}

func TestThreadMCPServers_ListWithAssignments(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createMCPServer(t, db, "default-server", true, true)
	nonDef := createMCPServer(t, db, "optional-server", false, true)

	db.Create(&models.ThreadMCPServer{ThreadID: thread.ID, MCPServerID: nonDef.ID})

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/mcp-servers", thread.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	for _, item := range items {
		if item["enabled"] != true {
			t.Errorf("server %q should be enabled", item["name"])
		}
	}
}

func TestThreadMCPServers_ListExcludesInactive(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createMCPServer(t, db, "active-server", false, true)
	createMCPServer(t, db, "inactive-server", false, false)

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/mcp-servers", thread.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	if len(items) != 1 {
		t.Fatalf("expected 1 active server, got %d", len(items))
	}
	if items[0]["name"] != "active-server" {
		t.Errorf("expected active-server, got %q", items[0]["name"])
	}
}

func TestThreadMCPServers_ListNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/threads/99999/mcp-servers", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestThreadMCPServers_SetServers(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createMCPServer(t, db, "default-server", true, true)
	s1 := createMCPServer(t, db, "optional-a", false, true)
	s2 := createMCPServer(t, db, "optional-b", false, true)

	r := mcpAssignRouter(db)
	body := fmt.Sprintf(`{"mcp_server_ids":[%d,%d]}`, s1.ID, s2.ID)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/mcp-servers", thread.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	if len(items) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(items))
	}
	for _, item := range items {
		if item["enabled"] != true {
			t.Errorf("server %q should be enabled after SET", item["name"])
		}
	}
}

func TestThreadMCPServers_ReplaceServers(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	s1 := createMCPServer(t, db, "server-a", false, true)
	s2 := createMCPServer(t, db, "server-b", false, true)

	// Assign s1 first
	db.Create(&models.ThreadMCPServer{ThreadID: thread.ID, MCPServerID: s1.ID})

	// Replace with s2
	r := mcpAssignRouter(db)
	body := fmt.Sprintf(`{"mcp_server_ids":[%d]}`, s2.ID)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/mcp-servers", thread.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	for _, item := range items {
		name := item["name"].(string)
		enabled := item["enabled"].(bool)
		if name == "server-a" && enabled {
			t.Error("server-a should no longer be enabled")
		}
		if name == "server-b" && !enabled {
			t.Error("server-b should be enabled")
		}
	}

	// Verify DB state
	var count int64
	db.Model(&models.ThreadMCPServer{}).Where("thread_id = ?", thread.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 assignment row, got %d", count)
	}
}

func TestThreadMCPServers_ClearAll(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	s := createMCPServer(t, db, "server-a", false, true)
	db.Create(&models.ThreadMCPServer{ThreadID: thread.ID, MCPServerID: s.ID})

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/mcp-servers", thread.ID), `{"mcp_server_ids":[]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	for _, item := range items {
		if item["enabled"] == true && item["is_default"] == false {
			t.Errorf("server %q should not be enabled after clear", item["name"])
		}
	}

	var count int64
	db.Model(&models.ThreadMCPServer{}).Where("thread_id = ?", thread.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 assignment rows, got %d", count)
	}
}

func TestThreadMCPServers_SetNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodPut, "/api/v1/threads/99999/mcp-servers", `{"mcp_server_ids":[]}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestThreadMCPServers_DefaultServerIDsFiltered(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	def := createMCPServer(t, db, "default-server", true, true)
	nonDef := createMCPServer(t, db, "optional-server", false, true)

	// Include both default and non-default IDs
	r := mcpAssignRouter(db)
	body := fmt.Sprintf(`{"mcp_server_ids":[%d,%d]}`, def.ID, nonDef.ID)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/mcp-servers", thread.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Only non-default should have a row
	var count int64
	db.Model(&models.ThreadMCPServer{}).Where("thread_id = ?", thread.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 assignment row (default filtered), got %d", count)
	}
}

// --- Project MCP Server Tests ---

func TestProjectMCPServers_ListDefaultsOnly(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	project := createTestProject(t, db)
	def := createMCPServer(t, db, "default-server", true, true)
	nonDef := createMCPServer(t, db, "optional-server", false, true)

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/mcp-servers", project.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	if len(items) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(items))
	}

	if items[0]["name"] != def.Name {
		t.Errorf("expected first=%q, got %q", def.Name, items[0]["name"])
	}
	if items[0]["enabled"] != true {
		t.Errorf("default server should be enabled")
	}
	if items[1]["name"] != nonDef.Name {
		t.Errorf("expected second=%q, got %q", nonDef.Name, items[1]["name"])
	}
	if items[1]["enabled"] != false {
		t.Errorf("non-default server should not be enabled")
	}
}

func TestProjectMCPServers_ListWithAssignments(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	project := createTestProject(t, db)
	createMCPServer(t, db, "default-server", true, true)
	nonDef := createMCPServer(t, db, "optional-server", false, true)

	db.Create(&models.ProjectMCPServer{ProjectID: project.ID, MCPServerID: nonDef.ID})

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/mcp-servers", project.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	for _, item := range items {
		if item["enabled"] != true {
			t.Errorf("server %q should be enabled", item["name"])
		}
	}
}

func TestProjectMCPServers_ListNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/mcp-servers", uuid.New()), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProjectMCPServers_SetServers(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	project := createTestProject(t, db)
	createMCPServer(t, db, "default-server", true, true)
	s1 := createMCPServer(t, db, "optional-a", false, true)
	s2 := createMCPServer(t, db, "optional-b", false, true)

	r := mcpAssignRouter(db)
	body := fmt.Sprintf(`{"mcp_server_ids":[%d,%d]}`, s1.ID, s2.ID)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/projects/%s/mcp-servers", project.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	if len(items) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(items))
	}
	for _, item := range items {
		if item["enabled"] != true {
			t.Errorf("server %q should be enabled after SET", item["name"])
		}
	}
}

func TestProjectMCPServers_ReplaceServers(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	project := createTestProject(t, db)
	s1 := createMCPServer(t, db, "server-a", false, true)
	s2 := createMCPServer(t, db, "server-b", false, true)

	db.Create(&models.ProjectMCPServer{ProjectID: project.ID, MCPServerID: s1.ID})

	r := mcpAssignRouter(db)
	body := fmt.Sprintf(`{"mcp_server_ids":[%d]}`, s2.ID)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/projects/%s/mcp-servers", project.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	items := parseMCPAssignResponse(t, w.Body.Bytes())
	for _, item := range items {
		name := item["name"].(string)
		enabled := item["enabled"].(bool)
		if name == "server-a" && enabled {
			t.Error("server-a should no longer be enabled")
		}
		if name == "server-b" && !enabled {
			t.Error("server-b should be enabled")
		}
	}

	var count int64
	db.Model(&models.ProjectMCPServer{}).Where("project_id = ?", project.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 assignment row, got %d", count)
	}
}

func TestProjectMCPServers_ClearAll(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	project := createTestProject(t, db)
	s := createMCPServer(t, db, "server-a", false, true)
	db.Create(&models.ProjectMCPServer{ProjectID: project.ID, MCPServerID: s.ID})

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/projects/%s/mcp-servers", project.ID), `{"mcp_server_ids":[]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.ProjectMCPServer{}).Where("project_id = ?", project.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 assignment rows, got %d", count)
	}
}

func TestProjectMCPServers_SetNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/projects/%s/mcp-servers", uuid.New()), `{"mcp_server_ids":[]}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProjectMCPServers_InvalidProjectID(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := mcpAssignRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/projects/not-a-uuid/mcp-servers", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- ResolveMCPServers Tests ---

func TestResolveMCPServers_DefaultOnly(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	createMCPServer(t, db, "default-a", true, true)
	createMCPServer(t, db, "default-b", true, true)
	createMCPServer(t, db, "optional", false, true)

	servers, err := models.ResolveMCPServers(db, nil, nil)
	if err != nil {
		t.Fatalf("ResolveMCPServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 default servers, got %d", len(servers))
	}
}

func TestResolveMCPServers_ThreadOverride(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createMCPServer(t, db, "default-server", true, true)
	opt := createMCPServer(t, db, "optional-server", false, true)

	db.Create(&models.ThreadMCPServer{ThreadID: thread.ID, MCPServerID: opt.ID})

	threadID := thread.ID
	servers, err := models.ResolveMCPServers(db, &threadID, nil)
	if err != nil {
		t.Fatalf("ResolveMCPServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers (1 default + 1 thread), got %d", len(servers))
	}
}

func TestResolveMCPServers_ProjectOverride(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	project := createTestProject(t, db)
	createMCPServer(t, db, "default-server", true, true)
	opt := createMCPServer(t, db, "optional-server", false, true)

	db.Create(&models.ProjectMCPServer{ProjectID: project.ID, MCPServerID: opt.ID})

	projID := project.ID
	servers, err := models.ResolveMCPServers(db, nil, &projID)
	if err != nil {
		t.Fatalf("ResolveMCPServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers (1 default + 1 project), got %d", len(servers))
	}
}

func TestResolveMCPServers_CombinedThreadAndProject(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	project := createTestProject(t, db)
	createMCPServer(t, db, "default-server", true, true)
	threadOpt := createMCPServer(t, db, "thread-opt", false, true)
	projOpt := createMCPServer(t, db, "proj-opt", false, true)

	db.Create(&models.ThreadMCPServer{ThreadID: thread.ID, MCPServerID: threadOpt.ID})
	db.Create(&models.ProjectMCPServer{ProjectID: project.ID, MCPServerID: projOpt.ID})

	threadID := thread.ID
	projID := project.ID
	servers, err := models.ResolveMCPServers(db, &threadID, &projID)
	if err != nil {
		t.Fatalf("ResolveMCPServers: %v", err)
	}
	if len(servers) != 3 {
		t.Fatalf("expected 3 servers (1 default + 1 thread + 1 project), got %d", len(servers))
	}
}

func TestResolveMCPServers_Deduplication(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	project := createTestProject(t, db)
	shared := createMCPServer(t, db, "shared-server", false, true)

	db.Create(&models.ThreadMCPServer{ThreadID: thread.ID, MCPServerID: shared.ID})
	db.Create(&models.ProjectMCPServer{ProjectID: project.ID, MCPServerID: shared.ID})

	threadID := thread.ID
	projID := project.ID
	servers, err := models.ResolveMCPServers(db, &threadID, &projID)
	if err != nil {
		t.Fatalf("ResolveMCPServers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server (deduplicated), got %d", len(servers))
	}
}

func TestResolveMCPServers_InactiveExcluded(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createMCPServer(t, db, "active-default", true, true)
	createMCPServer(t, db, "inactive-default", true, false)
	inactive := createMCPServer(t, db, "inactive-opt", false, false)

	db.Create(&models.ThreadMCPServer{ThreadID: thread.ID, MCPServerID: inactive.ID})

	threadID := thread.ID
	servers, err := models.ResolveMCPServers(db, &threadID, nil)
	if err != nil {
		t.Fatalf("ResolveMCPServers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 active server, got %d", len(servers))
	}
	if servers[0].Name != "active-default" {
		t.Errorf("expected active-default, got %q", servers[0].Name)
	}
}

func TestResolveMCPServers_DeletedServerCascade(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	s := createMCPServer(t, db, "to-delete", false, true)
	db.Create(&models.ThreadMCPServer{ThreadID: thread.ID, MCPServerID: s.ID})

	// Simulate CASCADE: delete assignment then server
	// (AutoMigrate doesn't create FK CASCADE constraints, real migrations do)
	db.Where("mcp_server_id = ?", s.ID).Delete(&models.ThreadMCPServer{})
	db.Delete(&models.MCPServer{}, s.ID)

	var count int64
	db.Model(&models.ThreadMCPServer{}).Where("thread_id = ?", thread.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 assignment rows after delete, got %d", count)
	}

	threadID := thread.ID
	servers, err := models.ResolveMCPServers(db, &threadID, nil)
	if err != nil {
		t.Fatalf("ResolveMCPServers: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 resolved servers, got %d", len(servers))
	}
}
