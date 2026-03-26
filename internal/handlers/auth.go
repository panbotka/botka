package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"botka/internal/middleware"
	"botka/internal/models"
)

// AuthHandler handles authentication endpoints (login, logout, me, change password).
type AuthHandler struct {
	db            *gorm.DB
	sessionMaxAge time.Duration
	secure        bool // set Secure flag on cookies (for HTTPS)
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db *gorm.DB, sessionMaxAge time.Duration, secure bool) *AuthHandler {
	return &AuthHandler{db: db, sessionMaxAge: sessionMaxAge, secure: secure}
}

// RegisterAuthRoutes attaches auth endpoints to the given router group.
func RegisterAuthRoutes(rg *gin.RouterGroup, h *AuthHandler) {
	auth := rg.Group("/auth")
	auth.POST("/login", h.Login)
	auth.POST("/logout", h.Logout)
	auth.GET("/me", h.Me)
	auth.PUT("/password", h.ChangePassword)
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login authenticates a user with username and password.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "username and password required")
		return
	}

	var user models.User
	if err := h.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		respondError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		respondError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.createSession(user.ID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create session")
		return
	}

	h.setSessionCookie(c, token)
	respondOK(c, gin.H{"username": user.Username, "id": user.ID, "role": user.Role})
}

// Logout clears the session cookie and deletes the session.
func (h *AuthHandler) Logout(c *gin.Context) {
	cookie, err := c.Cookie(middleware.SessionCookieName)
	if err == nil && cookie != "" {
		h.db.Delete(&models.Session{}, "id = ?", cookie)
	}
	h.clearSessionCookie(c)
	c.JSON(http.StatusNoContent, nil)
}

// Me returns the current authenticated user, or 401 if not authenticated.
func (h *AuthHandler) Me(c *gin.Context) {
	cookie, err := c.Cookie(middleware.SessionCookieName)
	if err != nil || cookie == "" {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var session models.Session
	if err := h.db.Where("id = ? AND expires_at > ?", cookie, time.Now()).First(&session).Error; err != nil {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var user models.User
	if err := h.db.First(&user, session.UserID).Error; err != nil {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Count registered passkeys.
	var passkeyCount int64
	h.db.Model(&models.WebAuthnCredential{}).Where("user_id = ?", user.ID).Count(&passkeyCount)

	respondOK(c, gin.H{
		"id":            user.ID,
		"username":      user.Username,
		"role":          user.Role,
		"passkey_count": passkeyCount,
	})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// ChangePassword updates the user's password after verifying the current one.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	user, ok := c.Get(middleware.ContextKeyUser)
	if !ok {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	u := user.(*models.User)

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "current_password and new_password required")
		return
	}

	if len(req.NewPassword) < 8 {
		respondError(c, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		respondError(c, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := h.db.Model(u).Update("password_hash", string(hash)).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update password")
		return
	}

	respondOK(c, gin.H{"message": "password updated"})
}

// createSession generates a random session token and stores it in the database.
func (h *AuthHandler) createSession(userID int64) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}

	session := models.Session{
		ID:        token,
		UserID:    userID,
		ExpiresAt: time.Now().Add(h.sessionMaxAge),
	}
	if err := h.db.Create(&session).Error; err != nil {
		return "", err
	}
	return token, nil
}

// CreateSessionForUser creates a session for a user and sets the cookie.
// Exported for use by the WebAuthn handler.
func (h *AuthHandler) CreateSessionForUser(c *gin.Context, userID int64) error {
	token, err := h.createSession(userID)
	if err != nil {
		return err
	}
	h.setSessionCookie(c, token)
	return nil
}

func (h *AuthHandler) setSessionCookie(c *gin.Context, token string) {
	maxAge := int(h.sessionMaxAge.Seconds())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.SessionCookieName, token, maxAge, "/", "", h.secure, true)
}

func (h *AuthHandler) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.SessionCookieName, "", -1, "/", "", h.secure, true)
}

// generateToken creates a cryptographically random 64-char hex string.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
