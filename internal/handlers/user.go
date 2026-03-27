package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"botka/internal/middleware"
	"botka/internal/models"
)

// UserHandler handles external user management endpoints (admin only).
type UserHandler struct {
	db *gorm.DB
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

// RegisterUserRoutes attaches user management endpoints to the given router group.
// All routes require admin access.
func RegisterUserRoutes(rg *gin.RouterGroup, h *UserHandler) {
	users := rg.Group("/users")
	users.Use(middleware.AdminOnly())
	users.GET("", h.List)
	users.POST("", h.Create)
	users.DELETE("/:id", h.Delete)
	users.PUT("/:id/password", h.ResetPassword)
	users.GET("/:id/threads", h.ListThreads)
	users.POST("/:id/threads", h.GrantThread)
	users.DELETE("/:id/threads/:thread_id", h.RevokeThread)
}

type createUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Create creates a new external user.
// Errors: 400 (missing fields, short password, long username/password), 409 (duplicate), 500.
func (h *UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "username and password required")
		return
	}

	if msg := firstError(
		validateMaxLength("username", req.Username, maxUsernameLength),
		validateMaxLength("password", req.Password, maxPasswordLength),
	); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	if len(req.Password) < 8 {
		respondError(c, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user := models.User{
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         models.RoleExternal,
	}

	if err := h.db.Create(&user).Error; err != nil {
		respondError(c, http.StatusConflict, "username already exists")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"role":       user.Role,
		"created_at": user.CreatedAt,
	}})
}

// List returns all users with their thread access counts.
func (h *UserHandler) List(c *gin.Context) {
	var users []models.User
	if err := h.db.Order("created_at ASC").Find(&users).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list users")
		return
	}

	result := make([]gin.H, 0, len(users))
	for _, u := range users {
		var threadCount int64
		if u.Role == models.RoleExternal {
			h.db.Model(&models.ThreadAccess{}).Where("user_id = ?", u.ID).Count(&threadCount)
		}
		result = append(result, gin.H{
			"id":           u.ID,
			"username":     u.Username,
			"role":         u.Role,
			"thread_count": threadCount,
			"created_at":   u.CreatedAt,
		})
	}

	respondList(c, result, int64(len(result)))
}

// Delete removes an external user. Cannot delete self or admin users.
func (h *UserHandler) Delete(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid user id")
		return
	}

	currentUser := middleware.GetUser(c)
	if currentUser.ID == id {
		respondError(c, http.StatusBadRequest, "cannot delete yourself")
		return
	}

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "user not found")
		return
	}

	if user.Role == models.RoleAdmin {
		respondError(c, http.StatusBadRequest, "cannot delete admin users")
		return
	}

	// Delete thread access records and sessions, then the user.
	tx := h.db.Begin()
	tx.Where("user_id = ?", id).Delete(&models.ThreadAccess{})
	tx.Where("user_id = ?", id).Delete(&models.Session{})
	if err := tx.Delete(&models.User{}, id).Error; err != nil {
		tx.Rollback()
		respondError(c, http.StatusInternalServerError, "failed to delete user")
		return
	}
	tx.Commit()

	respondOK(c, gin.H{"status": "ok"})
}

type resetPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

// ResetPassword sets a new password for an external user.
func (h *UserHandler) ResetPassword(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid user id")
		return
	}

	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "password required")
		return
	}

	if len(req.Password) < 8 {
		respondError(c, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	if msg := validateMaxLength("password", req.Password, maxPasswordLength); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "user not found")
		return
	}

	if user.Role == models.RoleAdmin {
		respondError(c, http.StatusBadRequest, "use the password change endpoint for admin users")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := h.db.Model(&user).Update("password_hash", string(hash)).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update password")
		return
	}

	// Invalidate existing sessions for this user.
	h.db.Where("user_id = ?", id).Delete(&models.Session{})

	respondOK(c, gin.H{"status": "ok"})
}

type grantThreadRequest struct {
	ThreadID int64 `json:"thread_id" binding:"required"`
}

// GrantThread gives an external user access to a thread.
func (h *UserHandler) GrantThread(c *gin.Context) {
	userID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid user id")
		return
	}

	var req grantThreadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "thread_id required")
		return
	}

	// Verify user exists and is external.
	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		respondError(c, http.StatusNotFound, "user not found")
		return
	}
	if user.Role != models.RoleExternal {
		respondError(c, http.StatusBadRequest, "can only grant thread access to external users")
		return
	}

	// Verify thread exists.
	var thread models.Thread
	if err := h.db.First(&thread, req.ThreadID).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	access := models.ThreadAccess{
		UserID:   userID,
		ThreadID: req.ThreadID,
	}
	if err := h.db.Create(&access).Error; err != nil {
		respondError(c, http.StatusConflict, "access already granted")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": access})
}

// RevokeThread removes an external user's access to a thread.
func (h *UserHandler) RevokeThread(c *gin.Context) {
	userID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid user id")
		return
	}

	threadID, err := paramInt64(c, "thread_id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	result := h.db.Where("user_id = ? AND thread_id = ?", userID, threadID).Delete(&models.ThreadAccess{})
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "access not found")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}

// ListThreads returns threads an external user has access to.
func (h *UserHandler) ListThreads(c *gin.Context) {
	userID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid user id")
		return
	}

	type accessRow struct {
		models.ThreadAccess
		ThreadTitle string `json:"thread_title"`
	}

	var rows []accessRow
	h.db.Table("thread_access ta").
		Select("ta.*, t.title AS thread_title").
		Joins("JOIN threads t ON t.id = ta.thread_id").
		Where("ta.user_id = ?", userID).
		Order("ta.created_at ASC").
		Scan(&rows)

	if rows == nil {
		rows = []accessRow{}
	}

	result := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		result = append(result, gin.H{
			"id":           r.ID,
			"thread_id":    r.ThreadID,
			"thread_title": r.ThreadTitle,
			"created_at":   r.CreatedAt,
		})
	}

	respondList(c, result, int64(len(result)))
}
