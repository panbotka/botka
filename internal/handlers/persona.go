package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// PersonaHandler handles HTTP requests for persona resources.
type PersonaHandler struct {
	db *gorm.DB
}

// NewPersonaHandler creates a new PersonaHandler with the given database connection.
func NewPersonaHandler(db *gorm.DB) *PersonaHandler {
	return &PersonaHandler{db: db}
}

// RegisterPersonaRoutes attaches persona endpoints to the given router group.
func RegisterPersonaRoutes(rg *gin.RouterGroup, h *PersonaHandler) {
	rg.GET("/personas", h.List)
	rg.POST("/personas", h.Create)
	rg.PUT("/personas/:id", h.Update)
	rg.DELETE("/personas/:id", h.Delete)
}

// List returns all personas ordered by sort_order.
func (h *PersonaHandler) List(c *gin.Context) {
	var personas []models.Persona
	if err := h.db.Order("sort_order ASC, name ASC").Find(&personas).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list personas")
		return
	}
	if personas == nil {
		personas = []models.Persona{}
	}
	respondOK(c, personas)
}

type createPersonaRequest struct {
	Name           string  `json:"name"`
	SystemPrompt   string  `json:"system_prompt"`
	DefaultModel   *string `json:"default_model"`
	Icon           *string `json:"icon"`
	StarterMessage *string `json:"starter_message"`
	SortOrder      int     `json:"sort_order"`
}

// Create creates a new persona.
// Errors: 400 (missing name, name/prompt too long), 500 (db error).
func (h *PersonaHandler) Create(c *gin.Context) {
	var req createPersonaRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		respondError(c, http.StatusBadRequest, "name is required")
		return
	}

	if msg := firstError(
		validateMaxLength("name", req.Name, maxTitleLength),
		validateMaxLength("system_prompt", req.SystemPrompt, maxSystemPromptLength),
	); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	persona := models.Persona{
		Name:           req.Name,
		SystemPrompt:   req.SystemPrompt,
		DefaultModel:   req.DefaultModel,
		Icon:           req.Icon,
		StarterMessage: req.StarterMessage,
		SortOrder:      req.SortOrder,
	}
	if err := h.db.Create(&persona).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create persona")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": persona})
}

// Update modifies an existing persona.
// Errors: 400 (invalid id, missing name, name/prompt too long), 404, 500.
func (h *PersonaHandler) Update(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid persona id")
		return
	}

	var req createPersonaRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		respondError(c, http.StatusBadRequest, "name is required")
		return
	}

	if msg := firstError(
		validateMaxLength("name", req.Name, maxTitleLength),
		validateMaxLength("system_prompt", req.SystemPrompt, maxSystemPromptLength),
	); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	var persona models.Persona
	if err := h.db.First(&persona, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "persona not found")
		return
	}

	persona.Name = req.Name
	persona.SystemPrompt = req.SystemPrompt
	persona.DefaultModel = req.DefaultModel
	persona.Icon = req.Icon
	persona.StarterMessage = req.StarterMessage
	persona.SortOrder = req.SortOrder

	if err := h.db.Save(&persona).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update persona")
		return
	}

	respondOK(c, persona)
}

// Delete removes a persona.
func (h *PersonaHandler) Delete(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid persona id")
		return
	}

	if err := h.db.Delete(&models.Persona{}, id).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete persona")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}
