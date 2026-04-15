package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// MCPServerHandler handles HTTP requests for MCP server resources.
type MCPServerHandler struct {
	db *gorm.DB
}

// NewMCPServerHandler creates a new MCPServerHandler with the given database connection.
func NewMCPServerHandler(db *gorm.DB) *MCPServerHandler {
	return &MCPServerHandler{db: db}
}

// RegisterMCPServerRoutes attaches MCP server endpoints to the given router group.
func RegisterMCPServerRoutes(rg *gin.RouterGroup, h *MCPServerHandler) {
	rg.GET("/mcp-servers", h.List)
	rg.POST("/mcp-servers", h.Create)
	rg.PATCH("/mcp-servers/:id", h.Update)
	rg.DELETE("/mcp-servers/:id", h.Delete)
}

// List returns all MCP servers ordered by name.
func (h *MCPServerHandler) List(c *gin.Context) {
	var servers []models.MCPServer
	var total int64

	h.db.Model(&models.MCPServer{}).Count(&total)
	if err := h.db.Order("name ASC").Find(&servers).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list MCP servers")
		return
	}
	if servers == nil {
		servers = []models.MCPServer{}
	}
	respondList(c, servers, total)
}

type createMCPServerRequest struct {
	Name       string          `json:"name"`
	ServerType string          `json:"server_type"`
	Config     json.RawMessage `json:"config"`
	IsDefault  *bool           `json:"is_default"`
	Active     *bool           `json:"active"`
}

// Create creates a new MCP server.
func (h *MCPServerHandler) Create(c *gin.Context) {
	var req createMCPServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name = strings.TrimSpace(req.Name); req.Name == "" {
		respondError(c, http.StatusBadRequest, "name is required")
		return
	}

	serverType := models.MCPServerType(req.ServerType)
	if !serverType.IsValid() {
		respondError(c, http.StatusBadRequest, "server_type must be \"stdio\" or \"sse\"")
		return
	}

	if err := validateMCPConfig(serverType, req.Config); err != "" {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	server := models.MCPServer{
		Name:       req.Name,
		ServerType: serverType,
		Config:     req.Config,
	}
	if req.IsDefault != nil {
		server.IsDefault = *req.IsDefault
	}
	if req.Active != nil {
		server.Active = *req.Active
	} else {
		server.Active = true
	}

	if err := h.db.Create(&server).Error; err != nil {
		if isUniqueViolation(err) {
			respondError(c, http.StatusConflict, "MCP server name already exists")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to create MCP server")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": server})
}

// Update modifies an existing MCP server (partial update).
func (h *MCPServerHandler) Update(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid MCP server id")
		return
	}

	var server models.MCPServer
	if err := h.db.First(&server, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "MCP server not found")
		return
	}

	var raw map[string]json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if nameJSON, ok := raw["name"]; ok {
		var name string
		if json.Unmarshal(nameJSON, &name) != nil || strings.TrimSpace(name) == "" {
			respondError(c, http.StatusBadRequest, "name is required")
			return
		}
		server.Name = strings.TrimSpace(name)
	}

	if stJSON, ok := raw["server_type"]; ok {
		var st string
		if json.Unmarshal(stJSON, &st) != nil {
			respondError(c, http.StatusBadRequest, "invalid server_type")
			return
		}
		serverType := models.MCPServerType(st)
		if !serverType.IsValid() {
			respondError(c, http.StatusBadRequest, "server_type must be \"stdio\" or \"sse\"")
			return
		}
		server.ServerType = serverType
	}

	if cfgJSON, ok := raw["config"]; ok {
		if errMsg := validateMCPConfig(server.ServerType, cfgJSON); errMsg != "" {
			respondError(c, http.StatusBadRequest, errMsg)
			return
		}
		server.Config = cfgJSON
	}

	if isDefaultJSON, ok := raw["is_default"]; ok {
		var isDefault bool
		if json.Unmarshal(isDefaultJSON, &isDefault) != nil {
			respondError(c, http.StatusBadRequest, "is_default must be a boolean")
			return
		}
		server.IsDefault = isDefault
	}

	if activeJSON, ok := raw["active"]; ok {
		var active bool
		if json.Unmarshal(activeJSON, &active) != nil {
			respondError(c, http.StatusBadRequest, "active must be a boolean")
			return
		}
		server.Active = active
	}

	if err := h.db.Save(&server).Error; err != nil {
		if isUniqueViolation(err) {
			respondError(c, http.StatusConflict, "MCP server name already exists")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to update MCP server")
		return
	}

	respondOK(c, server)
}

// Delete removes an MCP server.
func (h *MCPServerHandler) Delete(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid MCP server id")
		return
	}

	result := h.db.Delete(&models.MCPServer{}, id)
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete MCP server")
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "MCP server not found")
		return
	}

	c.Status(http.StatusNoContent)
}

// validateMCPConfig validates the config JSON based on server type.
func validateMCPConfig(serverType models.MCPServerType, config json.RawMessage) string {
	if len(config) == 0 {
		return "config is required"
	}

	var parsed map[string]json.RawMessage
	if json.Unmarshal(config, &parsed) != nil {
		return "config must be a valid JSON object"
	}

	switch serverType {
	case models.MCPServerTypeStdio:
		cmdJSON, ok := parsed["command"]
		if !ok {
			return "stdio config requires \"command\" field"
		}
		var cmd string
		if json.Unmarshal(cmdJSON, &cmd) != nil || cmd == "" {
			return "stdio config \"command\" must be a non-empty string"
		}
	case models.MCPServerTypeSSE:
		urlJSON, ok := parsed["url"]
		if !ok {
			return "sse config requires \"url\" field"
		}
		var url string
		if json.Unmarshal(urlJSON, &url) != nil || url == "" {
			return "sse config \"url\" must be a non-empty string"
		}
	}

	return ""
}

// isUniqueViolation checks if a database error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "UNIQUE constraint")
}
