package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/signal"
)

// mockSignalClient is a test double for SignalGroupLister that returns canned
// groups or a preset error.
type mockSignalClient struct {
	groups []signal.SignalGroup
	err    error
}

// ListGroups implements SignalGroupLister.
func (m *mockSignalClient) ListGroups(_ context.Context) ([]signal.SignalGroup, error) {
	return m.groups, m.err
}

// signalRouter builds a gin router with the signal routes wired to the given
// database and mock client.
func signalRouter(db *gorm.DB, client SignalGroupLister) *gin.Engine {
	r := gin.New()
	h := NewSignalHandler(db, client)
	v1 := r.Group("/api/v1")
	RegisterSignalRoutes(v1, h)
	return r
}

func TestSignal_ListGroups_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	mock := &mockSignalClient{
		groups: []signal.SignalGroup{
			{
				ID:      "group-a",
				Name:    "Group A",
				Members: []signal.Member{{Number: "+1"}, {Number: "+2"}},
			},
			{
				ID:      "group-b",
				Name:    "Group B",
				Members: []signal.Member{{Number: "+3"}},
			},
		},
	}
	r := signalRouter(db, mock)

	w := doRequest(r, http.MethodGet, "/api/v1/signal/groups", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []signalGroupResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != "group-a" || resp.Data[0].MemberCount != 2 {
		t.Errorf("unexpected first group: %+v", resp.Data[0])
	}
	if resp.Data[1].Name != "Group B" || resp.Data[1].MemberCount != 1 {
		t.Errorf("unexpected second group: %+v", resp.Data[1])
	}
}

func TestSignal_ListGroups_Empty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	mock := &mockSignalClient{groups: nil}
	r := signalRouter(db, mock)

	w := doRequest(r, http.MethodGet, "/api/v1/signal/groups", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []signalGroupResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Errorf("expected empty list, got %d", len(resp.Data))
	}
}

func TestSignal_ListGroups_DaemonUnreachable(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	mock := &mockSignalClient{err: fmt.Errorf("%w: dial tcp: refused", signal.ErrDaemonUnreachable)}
	r := signalRouter(db, mock)

	w := doRequest(r, http.MethodGet, "/api/v1/signal/groups", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSignal_ListGroups_GenericError(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	mock := &mockSignalClient{err: errors.New("boom")}
	r := signalRouter(db, mock)

	w := doRequest(r, http.MethodGet, "/api/v1/signal/groups", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestSignal_ListGroups_NilClient(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := signalRouter(db, nil)
	w := doRequest(r, http.MethodGet, "/api/v1/signal/groups", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestSignal_GetBridge_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := signalRouter(db, nil)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/signal-bridge", th.ID), "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSignal_GetBridge_InvalidID(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := signalRouter(db, nil)
	w := doRequest(r, http.MethodGet, "/api/v1/threads/notanumber/signal-bridge", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSignal_PutBridge_Create(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := signalRouter(db, nil)
	body := `{"group_id":"grp-123","group_name":"Alpha"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/signal-bridge", th.ID), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data models.SignalBridge `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.GroupID != "grp-123" || resp.Data.GroupName != "Alpha" {
		t.Errorf("unexpected bridge: %+v", resp.Data)
	}
	if !resp.Data.Active {
		t.Error("expected new bridge to be active by default")
	}
	if resp.Data.ThreadID != th.ID {
		t.Errorf("expected thread_id %d, got %d", th.ID, resp.Data.ThreadID)
	}

	// Verify in database.
	var persisted models.SignalBridge
	if err := db.Where("thread_id = ?", th.ID).First(&persisted).Error; err != nil {
		t.Fatalf("bridge not persisted: %v", err)
	}
	if persisted.GroupID != "grp-123" {
		t.Errorf("persisted group_id = %q, want grp-123", persisted.GroupID)
	}
}

func TestSignal_PutBridge_Update(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := signalRouter(db, nil)

	// Create.
	body := `{"group_id":"old-grp","group_name":"Old"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/signal-bridge", th.ID), body)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d", w.Code)
	}

	// Update.
	active := false
	updateBody := fmt.Sprintf(`{"group_id":"new-grp","group_name":"New","active":%t}`, active)
	w = doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/signal-bridge", th.ID), updateBody)
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data models.SignalBridge `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.GroupID != "new-grp" || resp.Data.GroupName != "New" {
		t.Errorf("update did not take effect: %+v", resp.Data)
	}
	if resp.Data.Active {
		t.Error("expected Active=false after update")
	}

	// Ensure we still have exactly one bridge for this thread.
	var count int64
	db.Model(&models.SignalBridge{}).Where("thread_id = ?", th.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 bridge for thread, got %d", count)
	}
}

func TestSignal_PutBridge_MissingGroupID(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := signalRouter(db, nil)
	body := `{"group_name":"NoID"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/signal-bridge", th.ID), body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSignal_PutBridge_ThreadNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := signalRouter(db, nil)
	body := `{"group_id":"grp-x"}`
	w := doRequest(r, http.MethodPut, "/api/v1/threads/999999/signal-bridge", body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSignal_GetBridge_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	// Seed a bridge directly.
	bridge := models.SignalBridge{
		ThreadID:  th.ID,
		GroupID:   "grp-seed",
		GroupName: "Seeded",
		Active:    true,
	}
	if err := db.Create(&bridge).Error; err != nil {
		t.Fatalf("seed bridge: %v", err)
	}

	r := signalRouter(db, nil)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/signal-bridge", th.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data models.SignalBridge `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.GroupID != "grp-seed" || resp.Data.GroupName != "Seeded" {
		t.Errorf("unexpected bridge: %+v", resp.Data)
	}
}

func TestSignal_DeleteBridge_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	// Create via PUT.
	r := signalRouter(db, nil)
	body := `{"group_id":"to-delete"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/signal-bridge", th.ID), body)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d", w.Code)
	}

	// Delete.
	w = doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/threads/%d/signal-bridge", th.ID), "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify gone.
	var count int64
	db.Model(&models.SignalBridge{}).Where("thread_id = ?", th.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected bridge deleted, found %d", count)
	}
}

func TestSignal_DeleteBridge_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := signalRouter(db, nil)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/threads/%d/signal-bridge", th.ID), "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSignal_UniqueThreadConstraint(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	// First bridge via direct insert.
	if err := db.Create(&models.SignalBridge{
		ThreadID: th.ID,
		GroupID:  "grp-1",
		Active:   true,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Second direct insert should violate unique index.
	err := db.Create(&models.SignalBridge{
		ThreadID: th.ID,
		GroupID:  "grp-2",
		Active:   true,
	}).Error
	if err == nil {
		t.Fatal("expected unique constraint violation on second bridge, got nil")
	}
}
