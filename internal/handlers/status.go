package handlers

import (
	"github.com/gin-gonic/gin"
)

// StatusHandler handles server status and model information endpoints.
type StatusHandler struct {
	defaultModel    string
	availableModels []string
	whisperEnabled  bool
}

// NewStatusHandler creates a new StatusHandler.
func NewStatusHandler(defaultModel string, availableModels []string, whisperEnabled bool) *StatusHandler {
	return &StatusHandler{
		defaultModel:    defaultModel,
		availableModels: availableModels,
		whisperEnabled:  whisperEnabled,
	}
}

// RegisterStatusRoutes attaches status endpoints to the given router group.
func RegisterStatusRoutes(rg *gin.RouterGroup, h *StatusHandler) {
	rg.GET("/models", h.Models)
	rg.GET("/status", h.Status)
}

// Models returns the list of available AI models.
func (h *StatusHandler) Models(c *gin.Context) {
	respondOK(c, gin.H{
		"default": h.defaultModel,
		"models":  h.availableModels,
	})
}

// Status returns server status information.
func (h *StatusHandler) Status(c *gin.Context) {
	respondOK(c, gin.H{
		"default_model":   h.defaultModel,
		"whisper_enabled": h.whisperEnabled,
	})
}
