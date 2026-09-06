package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newSettingsRouter wires a settings handler over the test database.
func newSettingsRouter(t *testing.T) (*gin.Engine, *SettingsHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := setupTestDB(t)
	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	return router, h
}

// putSettings sends a settings update and returns the decoded data envelope.
func putSettings(t *testing.T, router *gin.Engine, body string) map[string]any {
	t.Helper()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Data
}

func TestSettingsHandler_UpdateBoxAutoOff(t *testing.T) {
	router, h := newSettingsRouter(t)

	var gotKey, gotValue string
	h.SetOnChange(func(key, value string) { gotKey, gotValue = key, value })

	data := putSettings(t, router, `{"box_auto_off": true}`)

	if data["box_auto_off"] != true {
		t.Errorf("box_auto_off = %v (%T), want the boolean true", data["box_auto_off"], data["box_auto_off"])
	}
	if gotKey != "box_auto_off" || gotValue != "true" {
		t.Errorf("onChange(%q, %q), want (box_auto_off, true)", gotKey, gotValue)
	}
}

func TestSettingsHandler_BoxAutoOffRoundTripsFalse(t *testing.T) {
	router, _ := newSettingsRouter(t)

	for _, want := range []bool{true, false} {
		data := putSettings(t, router, fmt.Sprintf(`{"box_auto_off": %t}`, want))
		if data["box_auto_off"] != want {
			t.Errorf("box_auto_off = %v, want %t", data["box_auto_off"], want)
		}
	}
}

func TestSettingsHandler_OmittedBoxAutoOffIsNotWritten(t *testing.T) {
	// A partial update must not silently clear the switch: sending only
	// max_workers has to leave box_auto_off alone.
	router, _ := newSettingsRouter(t)

	putSettings(t, router, `{"box_auto_off": true}`)
	data := putSettings(t, router, `{"max_workers": 2}`)

	if data["box_auto_off"] != true {
		t.Errorf("box_auto_off = %v after an unrelated update, want it untouched", data["box_auto_off"])
	}
	if data["max_workers"] != float64(2) {
		t.Errorf("max_workers = %v, want 2", data["max_workers"])
	}
}
