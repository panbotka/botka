package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"botka/internal/models"
)

func TestSettings_Get(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Seed a max_workers setting.
	db.Save(&models.Setting{Key: "max_workers", Value: "2"})

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodGet, "/api/v1/settings", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			MaxWorkers int `json:"max_workers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.MaxWorkers != 2 {
		t.Errorf("expected max_workers=2, got %d", resp.Data.MaxWorkers)
	}
}

func TestSettings_UpdateMaxWorkers(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodPut, "/api/v1/settings", `{"max_workers": 5}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			MaxWorkers int `json:"max_workers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.MaxWorkers != 5 {
		t.Errorf("expected max_workers=5, got %d", resp.Data.MaxWorkers)
	}
}

func TestSettings_UpdateMaxWorkers_InvalidLow(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodPut, "/api/v1/settings", `{"max_workers": 0}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSettings_UpdateMaxWorkers_InvalidHigh(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := doRequest(router, http.MethodPut, "/api/v1/settings", `{"max_workers": 11}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
