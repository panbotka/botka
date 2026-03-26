package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// ThreadSourceHandler handles HTTP requests for thread URL source resources.
type ThreadSourceHandler struct {
	db *gorm.DB
}

// NewThreadSourceHandler creates a new ThreadSourceHandler.
func NewThreadSourceHandler(db *gorm.DB) *ThreadSourceHandler {
	return &ThreadSourceHandler{db: db}
}

// RegisterThreadSourceRoutes attaches thread source endpoints to the given router group.
func RegisterThreadSourceRoutes(rg *gin.RouterGroup, h *ThreadSourceHandler) {
	rg.GET("/threads/:id/sources", h.List)
	rg.POST("/threads/:id/sources", h.Create)
	rg.PUT("/threads/:id/sources/reorder", h.Reorder)
	rg.PUT("/threads/:id/sources/:source_id", h.Update)
	rg.DELETE("/threads/:id/sources/:source_id", h.Delete)
}

// List returns all sources for a thread, ordered by position.
func (h *ThreadSourceHandler) List(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var sources []models.ThreadSource
	if err := h.db.Where("thread_id = ?", threadID).Order("position ASC, id ASC").Find(&sources).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list sources")
		return
	}
	if sources == nil {
		sources = []models.ThreadSource{}
	}
	respondOK(c, sources)
}

type createSourceRequest struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

// Create adds a new source to a thread.
func (h *ThreadSourceHandler) Create(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.URL == "" {
		respondError(c, http.StatusBadRequest, "url is required")
		return
	}

	// Get next position
	var maxPos int
	h.db.Model(&models.ThreadSource{}).Where("thread_id = ?", threadID).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)

	source := models.ThreadSource{
		ThreadID: threadID,
		URL:      req.URL,
		Label:    req.Label,
		Position: maxPos + 1,
	}
	if err := h.db.Create(&source).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create source")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": source})
}

// Update modifies an existing source.
func (h *ThreadSourceHandler) Update(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}
	sourceID, err := paramInt64(c, "source_id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid source id")
		return
	}

	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.URL == "" {
		respondError(c, http.StatusBadRequest, "url is required")
		return
	}

	var source models.ThreadSource
	if err := h.db.Where("id = ? AND thread_id = ?", sourceID, threadID).First(&source).Error; err != nil {
		respondError(c, http.StatusNotFound, "source not found")
		return
	}

	source.URL = req.URL
	source.Label = req.Label
	if err := h.db.Save(&source).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update source")
		return
	}

	respondOK(c, source)
}

// Delete removes a source from a thread.
func (h *ThreadSourceHandler) Delete(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}
	sourceID, err := paramInt64(c, "source_id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid source id")
		return
	}

	result := h.db.Where("id = ? AND thread_id = ?", sourceID, threadID).Delete(&models.ThreadSource{})
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "source not found")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}

type reorderRequest struct {
	IDs []int64 `json:"ids"`
}

// Reorder updates the position of sources within a thread.
func (h *ThreadSourceHandler) Reorder(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req reorderRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondError(c, http.StatusBadRequest, "ids array is required")
		return
	}

	tx := h.db.Begin()
	for i, id := range req.IDs {
		if err := tx.Model(&models.ThreadSource{}).Where("id = ? AND thread_id = ?", id, threadID).Update("position", i).Error; err != nil {
			tx.Rollback()
			respondError(c, http.StatusInternalServerError, "failed to reorder sources")
			return
		}
	}
	tx.Commit()

	respondOK(c, gin.H{"status": "ok"})
}
