package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTranscribeHandler_Status_Enabled(t *testing.T) {
	r := gin.New()
	h := NewTranscribeHandler("http://localhost:18789", "", true)
	v1 := r.Group("/api/v1")
	RegisterTranscribeRoutes(v1, h)

	w := doRequest(r, http.MethodGet, "/api/v1/transcribe/status", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", data["enabled"])
	}
}

func TestTranscribeHandler_Status_Disabled(t *testing.T) {
	r := gin.New()
	h := NewTranscribeHandler("http://localhost:18789", "", false)
	v1 := r.Group("/api/v1")
	RegisterTranscribeRoutes(v1, h)

	w := doRequest(r, http.MethodGet, "/api/v1/transcribe/status", "")

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["enabled"] != false {
		t.Errorf("expected enabled=false, got %v", data["enabled"])
	}
}

func TestTranscribeHandler_Transcribe_Disabled(t *testing.T) {
	r := gin.New()
	h := NewTranscribeHandler("http://localhost:18789", "", false)
	v1 := r.Group("/api/v1")
	RegisterTranscribeRoutes(v1, h)

	w := doRequest(r, http.MethodPost, "/api/v1/transcribe", "")

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
