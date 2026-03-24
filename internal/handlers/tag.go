package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// TagHandler handles HTTP requests for tag resources.
type TagHandler struct {
	db *gorm.DB
}

// NewTagHandler creates a new TagHandler with the given database connection.
func NewTagHandler(db *gorm.DB) *TagHandler {
	return &TagHandler{db: db}
}

// RegisterTagRoutes attaches tag endpoints to the given router group.
func RegisterTagRoutes(rg *gin.RouterGroup, h *TagHandler) {
	rg.GET("/tags", h.List)
	rg.POST("/tags", h.Create)
	rg.PUT("/tags/:id", h.Update)
	rg.DELETE("/tags/:id", h.Delete)
	rg.GET("/tags/:id/threads/count", h.CountThreads)
}

// List returns all tags.
func (h *TagHandler) List(c *gin.Context) {
	var tags []models.Tag
	if err := h.db.Order("name ASC").Find(&tags).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list tags")
		return
	}
	if tags == nil {
		tags = []models.Tag{}
	}
	respondOK(c, tags)
}

type createTagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Create creates a new tag.
func (h *TagHandler) Create(c *gin.Context) {
	var req createTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 50 {
		respondError(c, http.StatusBadRequest, "name is required (max 50 chars)")
		return
	}

	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#6B7280"
	}

	tag := models.Tag{Name: name, Color: color}
	if err := h.db.Create(&tag).Error; err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			respondError(c, http.StatusConflict, "tag name already exists")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to create tag")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": tag})
}

// Update modifies an existing tag's name and color.
func (h *TagHandler) Update(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid tag id")
		return
	}

	var req createTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 50 {
		respondError(c, http.StatusBadRequest, "name is required (max 50 chars)")
		return
	}

	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#6B7280"
	}

	result := h.db.Model(&models.Tag{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":  name,
		"color": color,
	})
	if result.Error != nil {
		if strings.Contains(result.Error.Error(), "unique") || strings.Contains(result.Error.Error(), "duplicate") {
			respondError(c, http.StatusConflict, "tag name already exists")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to update tag")
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "tag not found")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}

// Delete removes a tag and its thread associations.
func (h *TagHandler) Delete(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid tag id")
		return
	}

	tx := h.db.Begin()
	tx.Exec("DELETE FROM thread_tags WHERE tag_id = ?", id)
	if err := tx.Delete(&models.Tag{}, id).Error; err != nil {
		tx.Rollback()
		respondError(c, http.StatusInternalServerError, "failed to delete tag")
		return
	}
	tx.Commit()

	respondOK(c, gin.H{"status": "ok"})
}

// CountThreads returns the number of threads associated with a tag.
func (h *TagHandler) CountThreads(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid tag id")
		return
	}

	var count int64
	h.db.Raw("SELECT COUNT(*) FROM thread_tags WHERE tag_id = ?", id).Scan(&count)

	respondOK(c, gin.H{"count": count})
}
