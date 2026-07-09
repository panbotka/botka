package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/skills"
)

// skillRouter builds a router whose rescan endpoint discovers exactly the
// given skills, so tests never depend on what is installed on the host.
func skillRouter(db *gorm.DB, discovered []skills.Discovered, scanErr error) *gin.Engine {
	scan := func(_, _ string) ([]skills.Discovered, error) { return discovered, scanErr }
	r := gin.New()
	h := NewSkillHandler(db, "/home/test", "/projects", scan, skills.SyncToDatabase)
	v1 := r.Group("/api/v1")
	RegisterSkillRoutes(v1, h)
	return r
}

// createTestSkill inserts a registry row and returns it.
func createTestSkill(t *testing.T, db *gorm.DB, name string, defaultEnabled bool) models.Skill {
	t.Helper()
	s := models.Skill{Name: name, Description: name + " desc", Source: skills.SourceUser, DefaultEnabled: defaultEnabled, Active: true}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("create skill %q: %v", name, err)
	}
	return s
}

// decodeSkillList unwraps a {"data": [...]} envelope into effective skills.
func decodeSkillList(t *testing.T, body []byte) []models.EffectiveSkill {
	t.Helper()
	var resp struct {
		Data []models.EffectiveSkill `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response %s: %v", body, err)
	}
	return resp.Data
}

func TestSkill_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	w := doRequest(skillRouter(db, nil, nil), http.MethodGet, "/api/v1/skills", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data  []models.Skill `json:"data"`
		Total int64          `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 0 || resp.Total != 0 {
		t.Errorf("expected empty registry, got %+v", resp)
	}
}

func TestSkill_ListOrdersActiveFirst(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	createTestSkill(t, db, "zeta", true)
	inactive := createTestSkill(t, db, "alpha", true)
	db.Model(&inactive).Update("active", false)

	w := doRequest(skillRouter(db, nil, nil), http.MethodGet, "/api/v1/skills", "")
	var resp struct {
		Data []models.Skill `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "zeta" || resp.Data[1].Name != "alpha" {
		t.Errorf("expected active skill first, got %q then %q", resp.Data[0].Name, resp.Data[1].Name)
	}
}

func TestSkill_UpdateDefaultEnabled(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	createTestSkill(t, db, "alpha", true)

	r := skillRouter(db, nil, nil)
	w := doRequest(r, http.MethodPatch, "/api/v1/skills/alpha", `{"default_enabled":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stored models.Skill
	db.Where("name = ?", "alpha").First(&stored)
	if stored.DefaultEnabled {
		t.Error("expected default_enabled to be false after PATCH")
	}
}

func TestSkill_UpdateErrors(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	createTestSkill(t, db, "alpha", true)
	r := skillRouter(db, nil, nil)

	tests := []struct {
		name     string
		path     string
		body     string
		wantCode int
	}{
		{name: "unknown skill", path: "/api/v1/skills/nope", body: `{"default_enabled":false}`, wantCode: http.StatusNotFound},
		{name: "malformed body", path: "/api/v1/skills/alpha", body: `{`, wantCode: http.StatusBadRequest},
		{name: "missing field", path: "/api/v1/skills/alpha", body: `{}`, wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(r, http.MethodPatch, tt.path, tt.body)
			if w.Code != tt.wantCode {
				t.Errorf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

// TestSkill_RescanSyncsRegistry covers the registry sync end to end: new skills
// are inserted default-on, user-set defaults survive, and vanished skills are
// deactivated rather than deleted.
func TestSkill_RescanSyncsRegistry(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Pre-existing skill the user turned off, plus one that will vanish.
	createTestSkill(t, db, "kept", false)
	createTestSkill(t, db, "gone", true)

	discovered := []skills.Discovered{
		{Name: "kept", Description: "updated desc", Source: skills.SourceProject},
		{Name: "fresh", Description: "brand new", Source: skills.SourceUser},
	}

	w := doRequest(skillRouter(db, discovered, nil), http.MethodPost, "/api/v1/skills/rescan", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var kept, fresh, gone models.Skill
	db.Where("name = ?", "kept").First(&kept)
	db.Where("name = ?", "fresh").First(&fresh)
	db.Where("name = ?", "gone").First(&gone)

	if kept.DefaultEnabled {
		t.Error("rescan overwrote the user's default_enabled for an existing skill")
	}
	if kept.Description != "updated desc" || kept.Source != skills.SourceProject {
		t.Errorf("rescan did not refresh disk fields: %+v", kept)
	}
	if !fresh.DefaultEnabled || !fresh.Active {
		t.Errorf("newly discovered skill should default to enabled+active, got %+v", fresh)
	}
	if gone.ID == 0 {
		t.Fatal("vanished skill was deleted, expected deactivation")
	}
	if gone.Active {
		t.Error("vanished skill should be deactivated")
	}
}

func TestSkill_RescanIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	discovered := []skills.Discovered{{Name: "alpha", Description: "a", Source: skills.SourceUser}}
	r := skillRouter(db, discovered, nil)

	for i := range 2 {
		if w := doRequest(r, http.MethodPost, "/api/v1/skills/rescan", ""); w.Code != http.StatusOK {
			t.Fatalf("rescan %d: expected 200, got %d", i, w.Code)
		}
	}

	var count int64
	db.Model(&models.Skill{}).Where("name = ?", "alpha").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 row after repeated rescan, got %d", count)
	}
}

func TestSkill_RescanScanFailure(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := skillRouter(db, nil, errors.New("disk on fire"))
	if w := doRequest(r, http.MethodPost, "/api/v1/skills/rescan", ""); w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSkill_RescanKeepsOverrides guards the promise that a skill temporarily
// missing from disk does not lose its per-thread overrides.
func TestSkill_RescanKeepsOverrides(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createTestSkill(t, db, "alpha", true)
	db.Create(&models.ThreadSkill{ThreadID: thread.ID, SkillName: "alpha", Enabled: false})

	// Rescan discovers nothing: alpha vanishes from disk.
	doRequest(skillRouter(db, nil, nil), http.MethodPost, "/api/v1/skills/rescan", "")

	var count int64
	db.Model(&models.ThreadSkill{}).Where("thread_id = ?", thread.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected the override to survive deactivation, got %d rows", count)
	}
}

// TestSkill_ThreadEffectiveState covers effective-state resolution: a thread
// with no overrides inherits the current defaults.
func TestSkill_ThreadEffectiveState(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createTestSkill(t, db, "on-by-default", true)
	createTestSkill(t, db, "off-by-default", false)

	r := skillRouter(db, nil, nil)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/skills", thread.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got := decodeSkillList(t, w.Body.Bytes())
	if len(got) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(got))
	}
	byName := map[string]models.EffectiveSkill{got[0].Name: got[0], got[1].Name: got[1]}
	if !byName["on-by-default"].Enabled || byName["on-by-default"].Overridden {
		t.Errorf("default-on skill should be enabled and not overridden: %+v", byName["on-by-default"])
	}
	if byName["off-by-default"].Enabled || byName["off-by-default"].Overridden {
		t.Errorf("default-off skill should be disabled and not overridden: %+v", byName["off-by-default"])
	}
}

// TestSkill_SetThreadSkillsOnlyStoresDeviations verifies the override model:
// a selection matching the default stores no row, so later default changes
// still reach the thread.
func TestSkill_SetThreadSkillsOnlyStoresDeviations(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createTestSkill(t, db, "alpha", true)
	createTestSkill(t, db, "beta", true)

	r := skillRouter(db, nil, nil)
	path := fmt.Sprintf("/api/v1/threads/%d/skills", thread.ID)

	// Turn beta off; alpha stays at its default.
	w := doRequest(r, http.MethodPut, path, `{"enabled_skills":["alpha"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var overrides []models.ThreadSkill
	db.Where("thread_id = ?", thread.ID).Find(&overrides)
	if len(overrides) != 1 || overrides[0].SkillName != "beta" || overrides[0].Enabled {
		t.Fatalf("expected exactly one 'beta off' override, got %+v", overrides)
	}

	// Flipping alpha's default now reaches the thread; beta stays overridden.
	db.Model(&models.Skill{}).Where("name = ?", "alpha").Update("default_enabled", false)
	db.Model(&models.Skill{}).Where("name = ?", "beta").Update("default_enabled", false)

	disabled, err := models.DisabledSkillsForThread(db, thread.ID)
	if err != nil {
		t.Fatalf("DisabledSkillsForThread: %v", err)
	}
	if len(disabled) != 2 {
		t.Errorf("expected both skills disabled, got %v", disabled)
	}
}

// TestSkill_SetThreadSkillsRemovesRedundantOverride checks that re-selecting a
// skill's default state drops its override row.
func TestSkill_SetThreadSkillsRemovesRedundantOverride(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createTestSkill(t, db, "alpha", true)
	db.Create(&models.ThreadSkill{ThreadID: thread.ID, SkillName: "alpha", Enabled: false})

	r := skillRouter(db, nil, nil)
	path := fmt.Sprintf("/api/v1/threads/%d/skills", thread.ID)
	w := doRequest(r, http.MethodPut, path, `{"enabled_skills":["alpha"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.ThreadSkill{}).Where("thread_id = ?", thread.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected the redundant override to be deleted, got %d rows", count)
	}
}

// TestSkill_SetThreadSkillsIsIdempotent guards the upsert path — a second PUT
// with the same payload must not fail on the composite primary key.
func TestSkill_SetThreadSkillsIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createTestSkill(t, db, "alpha", true)

	r := skillRouter(db, nil, nil)
	path := fmt.Sprintf("/api/v1/threads/%d/skills", thread.ID)
	for i := range 2 {
		if w := doRequest(r, http.MethodPut, path, `{"enabled_skills":[]}`); w.Code != http.StatusOK {
			t.Fatalf("PUT %d: expected 200, got %d: %s", i, w.Code, w.Body.String())
		}
	}

	got := decodeSkillList(t, doRequest(r, http.MethodGet, path, "").Body.Bytes())
	if len(got) != 1 || got[0].Enabled || !got[0].Overridden {
		t.Errorf("expected alpha to be overridden off, got %+v", got)
	}
}

func TestSkill_ThreadEndpointErrors(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	r := skillRouter(db, nil, nil)
	thread := createTestThread(t, db)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
	}{
		{name: "get invalid id", method: http.MethodGet, path: "/api/v1/threads/abc/skills", wantCode: http.StatusBadRequest},
		{name: "get missing thread", method: http.MethodGet, path: "/api/v1/threads/999999/skills", wantCode: http.StatusNotFound},
		{name: "put invalid id", method: http.MethodPut, path: "/api/v1/threads/abc/skills", body: `{}`, wantCode: http.StatusBadRequest},
		{name: "put missing thread", method: http.MethodPut, path: "/api/v1/threads/999999/skills", body: `{}`, wantCode: http.StatusNotFound},
		{
			name: "put malformed body", method: http.MethodPut,
			path: fmt.Sprintf("/api/v1/threads/%d/skills", thread.ID), body: `{`, wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(r, tt.method, tt.path, tt.body)
			if w.Code != tt.wantCode {
				t.Errorf("expected %d, got %d: %s", tt.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

// TestSkill_DisabledSkillsForThread covers the deny-set construction that both
// spawn paths consume: sorted, active-only, override-aware.
func TestSkill_DisabledSkillsForThread(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	createTestSkill(t, db, "zeta", false) // off by default
	createTestSkill(t, db, "alpha", true) // on by default, overridden off below
	createTestSkill(t, db, "beta", true)  // on by default, stays on
	stale := createTestSkill(t, db, "stale", false)
	db.Model(&stale).Update("active", false) // inactive skills never get denied

	db.Create(&models.ThreadSkill{ThreadID: thread.ID, SkillName: "alpha", Enabled: false})

	got, err := models.DisabledSkillsForThread(db, thread.ID)
	if err != nil {
		t.Fatalf("DisabledSkillsForThread: %v", err)
	}
	want := []string{"alpha", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("DisabledSkillsForThread = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DisabledSkillsForThread = %v, want %v (sorted)", got, want)
		}
	}
}

// TestSkill_NewThreadInheritsDefaults asserts the acceptance criterion that a
// brand-new chat is denied exactly the default-off skills.
func TestSkill_NewThreadInheritsDefaults(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	createTestSkill(t, db, "on", true)
	createTestSkill(t, db, "off", false)

	fresh := createTestThread(t, db)
	got, err := models.DisabledSkillsForThread(db, fresh.ID)
	if err != nil {
		t.Fatalf("DisabledSkillsForThread: %v", err)
	}
	if len(got) != 1 || got[0] != "off" {
		t.Errorf("new thread denied %v, want only the default-off skill", got)
	}
}
