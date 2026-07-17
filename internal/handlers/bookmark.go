package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// BookmarkHandler handles HTTP requests for app-level bookmark resources.
type BookmarkHandler struct {
	db *gorm.DB
}

// NewBookmarkHandler creates a new BookmarkHandler.
func NewBookmarkHandler(db *gorm.DB) *BookmarkHandler {
	return &BookmarkHandler{db: db}
}

// RegisterBookmarkRoutes attaches bookmark endpoints to the given router group.
func RegisterBookmarkRoutes(rg *gin.RouterGroup, h *BookmarkHandler) {
	rg.GET("/bookmarks", h.List)
	rg.POST("/bookmarks", h.Create)
	rg.DELETE("/bookmarks/:id", h.Delete)
}

// List returns all bookmarks, ordered by sort_order.
func (h *BookmarkHandler) List(c *gin.Context) {
	var bookmarks []models.Bookmark
	if err := h.db.Order("sort_order ASC, id ASC").Find(&bookmarks).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list bookmarks")
		return
	}
	if bookmarks == nil {
		bookmarks = []models.Bookmark{}
	}
	respondOK(c, bookmarks)
}

type createBookmarkRequest struct {
	URL string `json:"url"`
}

// Create adds a new bookmark from a URL. The page title and favicon are fetched
// from the URL; a fetch failure never blocks adding — the bookmark is still
// saved with the hostname as its title and a default icon.
func (h *BookmarkHandler) Create(c *gin.Context) {
	var req createBookmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "url is required")
		return
	}

	normalized := normalizeBookmarkURL(req.URL)
	if normalized == "" {
		respondError(c, http.StatusBadRequest, "url is required")
		return
	}
	if msg := validateMaxLength("url", normalized, maxURLLength); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	title, faviconURL := fetchBookmarkMeta(c.Request.Context(), normalized)

	var maxOrder int
	h.db.Model(&models.Bookmark{}).Select("COALESCE(MAX(sort_order), -1)").Scan(&maxOrder)

	bookmark := models.Bookmark{
		URL:        normalized,
		Title:      title,
		FaviconURL: faviconURL,
		SortOrder:  maxOrder + 1,
	}
	if err := h.db.Create(&bookmark).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create bookmark")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": bookmark})
}

// Delete removes a bookmark by ID.
func (h *BookmarkHandler) Delete(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid bookmark id")
		return
	}

	result := h.db.Where("id = ?", id).Delete(&models.Bookmark{})
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete bookmark")
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "bookmark not found")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}
