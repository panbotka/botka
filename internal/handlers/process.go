package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"botka/internal/claude"
)

// ProcessHandler handles HTTP requests for active Claude process management.
type ProcessHandler struct{}

// NewProcessHandler creates a new ProcessHandler.
func NewProcessHandler() *ProcessHandler {
	return &ProcessHandler{}
}

// RegisterProcessRoutes attaches process management endpoints to the given router group.
func RegisterProcessRoutes(rg *gin.RouterGroup, h *ProcessHandler) {
	rg.GET("/processes", h.List)
	rg.DELETE("/processes/:id", h.Kill)
}

// List returns all active Claude Code chat processes.
func (h *ProcessHandler) List(c *gin.Context) {
	respondOK(c, claude.Registry.List())
}

// Kill terminates an active Claude process by thread ID.
func (h *ProcessHandler) Kill(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}
	if !claude.Registry.Kill(threadID) {
		respondError(c, http.StatusNotFound, "process not found")
		return
	}
	respondOK(c, gin.H{"status": "ok"})
}
