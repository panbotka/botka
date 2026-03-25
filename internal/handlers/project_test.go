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
	"botka/internal/projects"
)

func projectRouter(db *gorm.DB, scanFn ScanFunc, syncFn SyncFunc) *gin.Engine {
	r := gin.New()
	h := NewProjectHandler(db, "/tmp/projects", scanFn, syncFn)
	v1 := r.Group("/api/v1")
	RegisterProjectRoutes(v1, h)
	return r
}

func noopScan(_ string) ([]projects.DiscoveredProject, error)   { return nil, nil }
func noopSync(_ *gorm.DB, _ []projects.DiscoveredProject) error { return nil }

func TestProject_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, "/api/v1/projects", "")

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

func TestProject_ListOnlyActive(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	active := models.Project{Name: "active-proj", Path: "/tmp/active-" + uuid.New().String()[:8], BranchStrategy: "main", Active: true}
	inactive := models.Project{Name: "inactive-proj", Path: "/tmp/inactive-" + uuid.New().String()[:8], BranchStrategy: "main", Active: true}
	db.Create(&active)
	db.Create(&inactive)
	// Must update after create: GORM skips zero-value bool on insert, so the DB default (true) wins.
	db.Model(&inactive).Update("active", false)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, "/api/v1/projects", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("expected 1 active project, got %d", len(data))
	}
	name := data[0].(map[string]interface{})["name"].(string)
	if name != "active-proj" {
		t.Errorf("expected active-proj, got %s", name)
	}
}

func TestProject_ListWithTaskCounts(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)
	createTestTask(t, db, p.ID, models.TaskStatusPending)
	createTestTask(t, db, p.ID, models.TaskStatusDone)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, "/api/v1/projects", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("expected 1 project, got %d", len(data))
	}
	proj := data[0].(map[string]interface{})
	counts := proj["task_counts"].(map[string]interface{})
	if int(counts["pending"].(float64)) != 1 {
		t.Errorf("expected pending=1, got %v", counts["pending"])
	}
	if int(counts["done"].(float64)) != 1 {
		t.Errorf("expected done=1, got %v", counts["done"])
	}
}

func TestProject_GetSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s", p.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["name"] != "test-project" {
		t.Errorf("expected name=test-project, got %v", data["name"])
	}
}

func TestProject_GetNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s", uuid.New()), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProject_UpdateBranchStrategy(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)

	r := projectRouter(db, noopScan, noopSync)
	body := `{"branch_strategy":"feature_branch"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/projects/%s", p.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["branch_strategy"] != "feature_branch" {
		t.Errorf("expected branch_strategy=feature_branch, got %v", data["branch_strategy"])
	}
}

func TestProject_UpdateInvalidBranchStrategy(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)

	r := projectRouter(db, noopScan, noopSync)
	body := `{"branch_strategy":"invalid"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/projects/%s", p.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProject_UpdateClaudeMD(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)

	r := projectRouter(db, noopScan, noopSync)
	body := `{"claude_md":"# My Project\nSome instructions"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/projects/%s", p.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["claude_md"] != "# My Project\nSome instructions" {
		t.Errorf("expected updated claude_md, got %v", data["claude_md"])
	}
}

func TestProject_ScanSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	mockScan := func(dir string) ([]projects.DiscoveredProject, error) {
		return []projects.DiscoveredProject{{Name: "new-proj", Path: "/tmp/new-proj"}}, nil
	}
	mockSync := func(db *gorm.DB, discovered []projects.DiscoveredProject) error {
		return nil
	}

	r := projectRouter(db, mockScan, mockSync)
	w := doRequest(r, http.MethodPost, "/api/v1/projects/scan", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if int(data["discovered"].(float64)) != 1 {
		t.Errorf("expected discovered=1, got %v", data["discovered"])
	}
	if int(data["new"].(float64)) != 1 {
		t.Errorf("expected new=1, got %v", data["new"])
	}
}

func TestProject_Stats(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)
	createTestTask(t, db, p.ID, models.TaskStatusPending)
	createTestTask(t, db, p.ID, models.TaskStatusDone)
	createTestTask(t, db, p.ID, models.TaskStatusDone)
	createTestTask(t, db, p.ID, models.TaskStatusFailed)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/stats", p.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})

	if int(data["total"].(float64)) != 4 {
		t.Errorf("expected total=4, got %v", data["total"])
	}

	byStatus := data["by_status"].(map[string]interface{})
	if int(byStatus["pending"].(float64)) != 1 {
		t.Errorf("expected pending=1, got %v", byStatus["pending"])
	}
	if int(byStatus["done"].(float64)) != 2 {
		t.Errorf("expected done=2, got %v", byStatus["done"])
	}
	if int(byStatus["failed"].(float64)) != 1 {
		t.Errorf("expected failed=1, got %v", byStatus["failed"])
	}

	// success_rate should be 2/(2+1) = 0.6667
	successRate := data["success_rate"].(float64)
	if successRate < 0.66 || successRate > 0.67 {
		t.Errorf("expected success_rate ~0.6667, got %v", successRate)
	}
}

func TestProject_StatsEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	p := createTestProject(t, db)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/stats", p.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})

	if int(data["total"].(float64)) != 0 {
		t.Errorf("expected total=0, got %v", data["total"])
	}
	if data["success_rate"] != nil {
		t.Errorf("expected success_rate=nil, got %v", data["success_rate"])
	}
}

func TestProject_StatsNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/stats", uuid.New()), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProject_GitLogNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/git-log", uuid.New()), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProject_GitStatusNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/git-status", uuid.New()), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProject_GitLogNonGitDir(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Create project pointing to /tmp (not a git repo)
	p := models.Project{
		Name:           "non-git-project",
		Path:           "/tmp",
		BranchStrategy: "main",
		Active:         true,
	}
	db.Create(&p)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/git-log", p.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty commits, got %d", len(data))
	}
}

func TestProject_ScanError(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	mockScan := func(dir string) ([]projects.DiscoveredProject, error) {
		return nil, fmt.Errorf("disk error")
	}

	r := projectRouter(db, mockScan, noopSync)
	w := doRequest(r, http.MethodPost, "/api/v1/projects/scan", "")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	errMsg := resp["error"].(string)
	if errMsg != "scan failed: disk error" {
		t.Errorf("expected scan error message, got %q", errMsg)
	}
}
