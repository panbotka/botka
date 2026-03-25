package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func statusRouter() *gin.Engine {
	r := gin.New()
	h := NewStatusHandler("sonnet", []string{"sonnet", "opus", "haiku"}, true)
	v1 := r.Group("/api/v1")
	RegisterStatusRoutes(v1, h)
	return r
}

func TestStatusHandler_Models(t *testing.T) {
	r := statusRouter()
	w := doRequest(r, http.MethodGet, "/api/v1/models", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})

	if data["default"] != "sonnet" {
		t.Errorf("expected default=sonnet, got %v", data["default"])
	}
	models := data["models"].([]interface{})
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}
}

func TestStatusHandler_Status(t *testing.T) {
	r := statusRouter()
	w := doRequest(r, http.MethodGet, "/api/v1/status", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})

	if data["default_model"] != "sonnet" {
		t.Errorf("expected default_model=sonnet, got %v", data["default_model"])
	}
	if data["whisper_enabled"] != true {
		t.Errorf("expected whisper_enabled=true, got %v", data["whisper_enabled"])
	}
}

func TestStatusHandler_Status_WhisperDisabled(t *testing.T) {
	r := gin.New()
	h := NewStatusHandler("sonnet", []string{"sonnet"}, false)
	v1 := r.Group("/api/v1")
	RegisterStatusRoutes(v1, h)

	w := doRequest(r, http.MethodGet, "/api/v1/status", "")

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})

	if data["whisper_enabled"] != false {
		t.Errorf("expected whisper_enabled=false, got %v", data["whisper_enabled"])
	}
}
