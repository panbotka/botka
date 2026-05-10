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

func folderRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	v1 := r.Group("/api/v1")
	RegisterFolderRoutes(v1, NewFolderHandler(db))
	RegisterThreadRoutes(v1, NewThreadHandler(db, "sonnet", []string{"sonnet", "opus", "haiku"}))
	return r
}

func createFolder(t *testing.T, db *gorm.DB, name string, parentID *int64) models.ThreadFolder {
	t.Helper()
	f := models.ThreadFolder{Name: name, ParentID: parentID}
	if err := db.Create(&f).Error; err != nil {
		t.Fatalf("create folder: %v", err)
	}
	return f
}

func TestFolder_CreateRoot(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := folderRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/folders", `{"name":"Inbox"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["name"] != "Inbox" {
		t.Errorf("expected name=Inbox, got %v", data["name"])
	}
	if data["parent_id"] != nil {
		t.Errorf("expected parent_id=nil, got %v", data["parent_id"])
	}
}

func TestFolder_CreateNested(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	root := createFolder(t, db, "Root", nil)

	r := folderRouter(db)
	body := fmt.Sprintf(`{"name":"Child","parent_id":%d}`, root.ID)
	w := doRequest(r, http.MethodPost, "/api/v1/folders", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFolder_CreateRejectsMissingParent(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := folderRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/folders", `{"name":"Child","parent_id":9999}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFolder_CreateRejectsExceedingMaxDepth(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Build a chain of 5 folders (which is the max). The 6th create should fail.
	var parent *int64
	for i := 0; i < maxFolderDepth; i++ {
		f := createFolder(t, db, fmt.Sprintf("L%d", i+1), parent)
		id := f.ID
		parent = &id
	}

	r := folderRouter(db)
	body := fmt.Sprintf(`{"name":"L6","parent_id":%d}`, *parent)
	w := doRequest(r, http.MethodPost, "/api/v1/folders", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 max-depth, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "depth") {
		t.Errorf("expected error mentioning depth, got %s", w.Body.String())
	}
}

func TestFolder_ListReturnsTree(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	a := createFolder(t, db, "A", nil)
	b := createFolder(t, db, "B", &a.ID)
	createFolder(t, db, "C", &b.ID)

	r := folderRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/folders", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []struct {
			Name     string                   `json:"name"`
			Children []map[string]interface{} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Name != "A" {
		t.Fatalf("expected one root folder A, got %+v", resp.Data)
	}
	if len(resp.Data[0].Children) != 1 || resp.Data[0].Children[0]["name"] != "B" {
		t.Fatalf("expected B under A, got %+v", resp.Data[0].Children)
	}
	bChildren, _ := resp.Data[0].Children[0]["children"].([]interface{})
	if len(bChildren) != 1 {
		t.Fatalf("expected one child of B, got %+v", bChildren)
	}
}

func TestFolder_ListIncludesThreadCounts(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	root := createFolder(t, db, "Root", nil)
	model := "sonnet"
	for i := 0; i < 3; i++ {
		th := models.Thread{Title: fmt.Sprintf("t%d", i), Model: &model, FolderID: &root.ID}
		if err := db.Create(&th).Error; err != nil {
			t.Fatalf("create thread: %v", err)
		}
	}

	r := folderRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/folders", "")
	var resp struct {
		Data []struct {
			ID          int64 `json:"id"`
			ThreadCount int   `json:"thread_count"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 1 || resp.Data[0].ThreadCount != 3 {
		t.Fatalf("expected thread_count=3, got %+v", resp.Data)
	}
}

func TestFolder_RenameViaPatch(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	f := createFolder(t, db, "Old", nil)
	r := folderRouter(db)
	body := `{"name":"New"}`
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/folders/%d", f.ID), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var reloaded models.ThreadFolder
	db.First(&reloaded, f.ID)
	if reloaded.Name != "New" {
		t.Errorf("expected name=New, got %s", reloaded.Name)
	}
}

func TestFolder_MoveRejectsCycle(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	a := createFolder(t, db, "A", nil)
	b := createFolder(t, db, "B", &a.ID)
	c := createFolder(t, db, "C", &b.ID)

	r := folderRouter(db)
	// Try to move A under C (a descendant) — should be rejected.
	body := fmt.Sprintf(`{"parent_id":%d}`, c.ID)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/folders/%d", a.ID), body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 cycle, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFolder_MoveToRoot(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	a := createFolder(t, db, "A", nil)
	b := createFolder(t, db, "B", &a.ID)

	r := folderRouter(db)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/folders/%d", b.ID),
		`{"clear_parent":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var reloaded models.ThreadFolder
	db.First(&reloaded, b.ID)
	if reloaded.ParentID != nil {
		t.Errorf("expected parent_id=nil, got %v", reloaded.ParentID)
	}
}

func TestFolder_DeleteEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	f := createFolder(t, db, "X", nil)
	r := folderRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/folders/%d", f.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.ThreadFolder{}).Where("id = ?", f.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected folder deleted, found %d", count)
	}
}

func TestFolder_DeleteNonEmptyRejected(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	parent := createFolder(t, db, "Parent", nil)
	createFolder(t, db, "Child", &parent.ID)

	r := folderRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/folders/%d", parent.ID), "")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFolder_DeleteWithThreadsRejected(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	folder := createFolder(t, db, "WithThreads", nil)
	model := "sonnet"
	th := models.Thread{Title: "t", Model: &model, FolderID: &folder.ID}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}

	r := folderRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/folders/%d", folder.ID), "")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFolder_ReorderSiblings(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	a := createFolder(t, db, "A", nil)
	b := createFolder(t, db, "B", nil)
	c := createFolder(t, db, "C", nil)

	r := folderRouter(db)
	// Reorder: C, A, B at root level via PATCH on any one of them.
	body := fmt.Sprintf(`{"sibling_ids":[%d,%d,%d]}`, c.ID, a.ID, b.ID)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/folders/%d", a.ID), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var folders []models.ThreadFolder
	db.Order("position ASC").Find(&folders)
	if len(folders) != 3 || folders[0].ID != c.ID || folders[1].ID != a.ID || folders[2].ID != b.ID {
		t.Fatalf("expected order [C, A, B], got %+v", folders)
	}
}

func TestThread_PatchFolder(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	folder := createFolder(t, db, "Bin", nil)
	th := createTestThread(t, db)

	r := folderRouter(db)
	body := fmt.Sprintf(`{"folder_id":%d}`, folder.ID)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/threads/%d", th.ID), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var reloaded models.Thread
	db.First(&reloaded, th.ID)
	if reloaded.FolderID == nil || *reloaded.FolderID != folder.ID {
		t.Errorf("expected folder_id=%d, got %v", folder.ID, reloaded.FolderID)
	}

	// Now move it back to root.
	w = doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/threads/%d", th.ID), `{"clear_folder":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on clear_folder, got %d: %s", w.Code, w.Body.String())
	}
	db.First(&reloaded, th.ID)
	if reloaded.FolderID != nil {
		t.Errorf("expected folder_id=nil after clear, got %v", reloaded.FolderID)
	}
}

func TestThread_PatchFolderInvalidFolder(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := createTestThread(t, db)
	r := folderRouter(db)
	w := doRequest(r, http.MethodPatch, fmt.Sprintf("/api/v1/threads/%d", th.ID), `{"folder_id":9999}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
