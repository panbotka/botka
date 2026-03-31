package handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestKillTask_InvalidID(t *testing.T) {
	router := gin.New()
	h := &RunnerHandler{} // nil runner — we only test parameter parsing
	router.POST("/api/v1/tasks/:id/kill", h.KillTask)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/not-a-uuid/kill", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
