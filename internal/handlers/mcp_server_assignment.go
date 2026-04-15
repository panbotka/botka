package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

// MCPServerAssignmentHandler handles MCP server enablement per thread/project.
type MCPServerAssignmentHandler struct {
	db *gorm.DB
}

// NewMCPServerAssignmentHandler creates a new MCPServerAssignmentHandler.
func NewMCPServerAssignmentHandler(db *gorm.DB) *MCPServerAssignmentHandler {
	return &MCPServerAssignmentHandler{db: db}
}

// RegisterMCPServerAssignmentRoutes attaches MCP server assignment endpoints.
func RegisterMCPServerAssignmentRoutes(rg *gin.RouterGroup, h *MCPServerAssignmentHandler) {
	rg.GET("/threads/:id/mcp-servers", h.ListThreadServers)
	rg.PUT("/threads/:id/mcp-servers", h.SetThreadServers)
	rg.GET("/projects/:id/mcp-servers", h.ListProjectServers)
	rg.PUT("/projects/:id/mcp-servers", h.SetProjectServers)
}

type mcpServerWithStatus struct {
	ID         int64                `json:"id"`
	Name       string               `json:"name"`
	ServerType models.MCPServerType `json:"server_type"`
	IsDefault  bool                 `json:"is_default"`
	Active     bool                 `json:"active"`
	Enabled    bool                 `json:"enabled"`
}

// ListThreadServers returns all active MCP servers with enablement status for a thread.
func (h *MCPServerAssignmentHandler) ListThreadServers(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, threadID).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	result, err := h.listServersForThread(threadID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list MCP servers")
		return
	}

	respondOK(c, result)
}

// SetThreadServers replaces the explicit MCP server assignments for a thread.
func (h *MCPServerAssignmentHandler) SetThreadServers(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, threadID).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	var req struct {
		MCPServerIDs []int64 `json:"mcp_server_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	nonDefaultIDs, err := h.filterNonDefaultIDs(req.MCPServerIDs)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to filter server IDs")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("thread_id = ?", threadID).Delete(&models.ThreadMCPServer{}).Error; err != nil {
			return err
		}
		for _, sid := range nonDefaultIDs {
			if err := tx.Create(&models.ThreadMCPServer{ThreadID: threadID, MCPServerID: sid}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update thread MCP servers")
		return
	}

	result, err := h.listServersForThread(threadID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list MCP servers")
		return
	}

	respondOK(c, result)
}

// ListProjectServers returns all active MCP servers with enablement status for a project.
func (h *MCPServerAssignmentHandler) ListProjectServers(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	var project models.Project
	if err := h.db.First(&project, "id = ?", projectID).Error; err != nil {
		respondError(c, http.StatusNotFound, "project not found")
		return
	}

	result, err := h.listServersForProject(projectID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list MCP servers")
		return
	}

	respondOK(c, result)
}

// SetProjectServers replaces the explicit MCP server assignments for a project.
func (h *MCPServerAssignmentHandler) SetProjectServers(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	var project models.Project
	if err := h.db.First(&project, "id = ?", projectID).Error; err != nil {
		respondError(c, http.StatusNotFound, "project not found")
		return
	}

	var req struct {
		MCPServerIDs []int64 `json:"mcp_server_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	nonDefaultIDs, err := h.filterNonDefaultIDs(req.MCPServerIDs)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to filter server IDs")
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", projectID).Delete(&models.ProjectMCPServer{}).Error; err != nil {
			return err
		}
		for _, sid := range nonDefaultIDs {
			if err := tx.Create(&models.ProjectMCPServer{ProjectID: projectID, MCPServerID: sid}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update project MCP servers")
		return
	}

	result, err := h.listServersForProject(projectID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list MCP servers")
		return
	}

	respondOK(c, result)
}

func (h *MCPServerAssignmentHandler) listServersForThread(threadID int64) ([]mcpServerWithStatus, error) {
	var servers []models.MCPServer
	if err := h.db.Where("active = ?", true).Order("name ASC").Find(&servers).Error; err != nil {
		return nil, err
	}

	var assignments []models.ThreadMCPServer
	if err := h.db.Where("thread_id = ?", threadID).Find(&assignments).Error; err != nil {
		return nil, err
	}

	assigned := make(map[int64]bool, len(assignments))
	for _, a := range assignments {
		assigned[a.MCPServerID] = true
	}

	result := make([]mcpServerWithStatus, 0, len(servers))
	for _, s := range servers {
		result = append(result, mcpServerWithStatus{
			ID:         s.ID,
			Name:       s.Name,
			ServerType: s.ServerType,
			IsDefault:  s.IsDefault,
			Active:     s.Active,
			Enabled:    s.IsDefault || assigned[s.ID],
		})
	}
	return result, nil
}

func (h *MCPServerAssignmentHandler) listServersForProject(projectID uuid.UUID) ([]mcpServerWithStatus, error) {
	var servers []models.MCPServer
	if err := h.db.Where("active = ?", true).Order("name ASC").Find(&servers).Error; err != nil {
		return nil, err
	}

	var assignments []models.ProjectMCPServer
	if err := h.db.Where("project_id = ?", projectID).Find(&assignments).Error; err != nil {
		return nil, err
	}

	assigned := make(map[int64]bool, len(assignments))
	for _, a := range assignments {
		assigned[a.MCPServerID] = true
	}

	result := make([]mcpServerWithStatus, 0, len(servers))
	for _, s := range servers {
		result = append(result, mcpServerWithStatus{
			ID:         s.ID,
			Name:       s.Name,
			ServerType: s.ServerType,
			IsDefault:  s.IsDefault,
			Active:     s.Active,
			Enabled:    s.IsDefault || assigned[s.ID],
		})
	}
	return result, nil
}

func (h *MCPServerAssignmentHandler) filterNonDefaultIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var defaults []int64
	if err := h.db.Model(&models.MCPServer{}).
		Where("id IN ? AND is_default = ?", ids, true).
		Pluck("id", &defaults).Error; err != nil {
		return nil, err
	}

	defaultSet := make(map[int64]bool, len(defaults))
	for _, id := range defaults {
		defaultSet[id] = true
	}

	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !defaultSet[id] {
			result = append(result, id)
		}
	}
	return result, nil
}
