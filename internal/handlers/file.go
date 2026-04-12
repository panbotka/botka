package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/middleware"
	"botka/internal/models"
)

// FileHandler handles serving uploaded files.
type FileHandler struct {
	db        *gorm.DB
	uploadDir string
}

// checkAttachmentAccess verifies that the current user can access the
// attachment. Admin users always have access. External users must have
// thread_access for the thread containing the attachment's message.
func (h *FileHandler) checkAttachmentAccess(c *gin.Context, attachment *models.Attachment) bool {
	user := middleware.GetUser(c)
	if user == nil || user.IsAdmin() {
		return true
	}

	var msg models.Message
	if err := h.db.Select("thread_id").First(&msg, attachment.MessageID).Error; err != nil {
		respondError(c, http.StatusNotFound, "file not found")
		return false
	}

	if !middleware.HasThreadAccess(h.db, user.ID, msg.ThreadID) {
		respondError(c, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// NewFileHandler creates a new FileHandler with the given dependencies.
func NewFileHandler(db *gorm.DB, uploadDir string) *FileHandler {
	return &FileHandler{db: db, uploadDir: uploadDir}
}

// RegisterFileRoutes attaches file endpoints to the given router group.
func RegisterFileRoutes(rg *gin.RouterGroup, h *FileHandler) {
	rg.GET("/files/:id", h.ServeFile)
	rg.GET("/files/:id/download", h.DownloadFile)
	rg.GET("/uploads/:filename", h.ServeByName)
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

	if !h.checkAttachmentAccess(c, &attachment) {
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

	if !h.checkAttachmentAccess(c, &attachment) {
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

// ServeByName serves an uploaded file by its stored filename.
func (h *FileHandler) ServeByName(c *gin.Context) {
	filename := c.Param("filename")

	// Prevent path traversal
	if strings.ContainsAny(filename, "/\\") || filename == ".." || filename == "." {
		respondError(c, http.StatusBadRequest, "invalid filename")
		return
	}

	var attachment models.Attachment
	if err := h.db.Where("stored_name = ?", filename).First(&attachment).Error; err != nil {
		respondError(c, http.StatusNotFound, "file not found")
		return
	}

	if !h.checkAttachmentAccess(c, &attachment) {
		return
	}

	filePath := filepath.Join(h.uploadDir, attachment.StoredName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		respondError(c, http.StatusNotFound, "file not found on disk")
		return
	}

	c.Header("Content-Type", attachment.MimeType)
	c.Header("Content-Disposition", "inline; filename=\""+sanitizeFilename(attachment.OriginalName)+"\"")
	c.Header("Cache-Control", "public, max-age=86400")
	c.File(filePath)
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\\", "")
	return name
}
