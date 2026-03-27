package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// MemoryHandler handles HTTP requests for memory resources.
type MemoryHandler struct {
	db *gorm.DB
}

// NewMemoryHandler creates a new MemoryHandler with the given database connection.
func NewMemoryHandler(db *gorm.DB) *MemoryHandler {
	return &MemoryHandler{db: db}
}

// RegisterMemoryRoutes attaches memory endpoints to the given router group.
func RegisterMemoryRoutes(rg *gin.RouterGroup, h *MemoryHandler) {
	rg.GET("/memories", h.List)
	rg.POST("/memories", h.Create)
	rg.PUT("/memories/:id", h.Update)
	rg.DELETE("/memories/:id", h.Delete)
}

// List returns all memories ordered by creation time.
func (h *MemoryHandler) List(c *gin.Context) {
	var memories []models.Memory
	if err := h.db.Order("created_at DESC").Find(&memories).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list memories")
		return
	}
	if memories == nil {
		memories = []models.Memory{}
	}
	respondOK(c, memories)
}

type memoryRequest struct {
	Content string `json:"content"`
}

// Create creates a new memory.
// Errors: 400 (missing/long content, limit reached), 500 (db error).
func (h *MemoryHandler) Create(c *gin.Context) {
	var req memoryRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Content == "" {
		respondError(c, http.StatusBadRequest, "content is required")
		return
	}

	if msg := validateMaxLength("content", req.Content, maxContentLength); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	var count int64
	h.db.Model(&models.Memory{}).Count(&count)
	if count >= 100 {
		respondError(c, http.StatusBadRequest, "memory limit reached (max 100)")
		return
	}

	memory := models.Memory{Content: req.Content}
	if err := h.db.Create(&memory).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create memory")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": memory})
}

// Update modifies an existing memory's content.
func (h *MemoryHandler) Update(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		respondError(c, http.StatusBadRequest, "invalid memory id")
		return
	}

	var req memoryRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Content == "" {
		respondError(c, http.StatusBadRequest, "content is required")
		return
	}

	if msg := validateMaxLength("content", req.Content, maxContentLength); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	var memory models.Memory
	if err := h.db.First(&memory, "id = ?", id).Error; err != nil {
		respondError(c, http.StatusNotFound, "memory not found")
		return
	}

	memory.Content = req.Content
	if err := h.db.Save(&memory).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update memory")
		return
	}

	respondOK(c, memory)
}

// Delete removes a memory.
func (h *MemoryHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		respondError(c, http.StatusBadRequest, "invalid memory id")
		return
	}

	if err := h.db.Where("id = ?", id).Delete(&models.Memory{}).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete memory")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}
