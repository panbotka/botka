package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func processRouter() *gin.Engine {
	r := gin.New()
	h := NewProcessHandler()
	v1 := r.Group("/api/v1")
	RegisterProcessRoutes(v1, h)
	return r
}

func TestProcessHandler_List_Empty(t *testing.T) {
	r := processRouter()
	w := doRequest(r, http.MethodGet, "/api/v1/processes", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty process list, got %d", len(data))
	}
}

func TestProcessHandler_Kill_NotFound(t *testing.T) {
	r := processRouter()
	w := doRequest(r, http.MethodDelete, "/api/v1/processes/99999", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProcessHandler_Kill_InvalidID(t *testing.T) {
	r := processRouter()
	w := doRequest(r, http.MethodDelete, "/api/v1/processes/abc", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
