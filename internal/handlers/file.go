package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// FileHandler handles serving uploaded files.
type FileHandler struct {
	db        *gorm.DB
	uploadDir string
}

// NewFileHandler creates a new FileHandler with the given dependencies.
func NewFileHandler(db *gorm.DB, uploadDir string) *FileHandler {
	return &FileHandler{db: db, uploadDir: uploadDir}
}

// RegisterFileRoutes attaches file endpoints to the given router group.
func RegisterFileRoutes(rg *gin.RouterGroup, h *FileHandler) {
	rg.GET("/files/:id", h.ServeFile)
	rg.GET("/files/:id/download", h.DownloadFile)
}

// ServeFile serves an uploaded file inline with the correct MIME type.
func (h *FileHandler) ServeFile(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid file id")
		return
	}

	var attachment models.Attachment
	if err := h.db.First(&attachment, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "file not found")
		return
	}

	filePath := filepath.Join(h.uploadDir, attachment.StoredName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		respondError(c, http.StatusNotFound, "file not found on disk")
		return
	}

	c.Header("Content-Type", attachment.MimeType)
	c.Header("Content-Disposition", "inline; filename=\""+sanitizeFilename(attachment.OriginalName)+"\"")
	c.File(filePath)
}

// DownloadFile serves an uploaded file as an attachment download.
func (h *FileHandler) DownloadFile(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid file id")
		return
	}

	var attachment models.Attachment
	if err := h.db.First(&attachment, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "file not found")
		return
	}

	filePath := filepath.Join(h.uploadDir, attachment.StoredName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		respondError(c, http.StatusNotFound, "file not found on disk")
		return
	}

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename=\""+sanitizeFilename(attachment.OriginalName)+"\"")
	c.File(filePath)
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\\", "")
	return name
}
