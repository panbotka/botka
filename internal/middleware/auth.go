package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "botka_session"

	// ContextKeyUser is the Gin context key for the authenticated user.
	ContextKeyUser = "user"
)

// Auth returns Gin middleware that checks the session cookie against the
// sessions table and sets the user in the context. Unauthenticated requests
// to protected routes receive a 401 JSON response.
func Auth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isPublicPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		cookie, err := c.Cookie(SessionCookieName)
		if err != nil || cookie == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		var session models.Session
		if err := db.Where("id = ? AND expires_at > ?", cookie, time.Now()).First(&session).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		var user models.User
		if err := db.First(&user, session.UserID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		c.Set(ContextKeyUser, &user)
		c.Next()
	}
}

// isPublicPath returns true for paths that do not require authentication.
func isPublicPath(path string) bool {
	// Auth endpoints that must be public.
	switch path {
	case "/api/v1/auth/login",
		"/api/v1/auth/me",
		"/api/v1/auth/passkey/login/begin",
		"/api/v1/auth/passkey/login/finish":
		return true
	}

	// MCP uses its own auth.
	if strings.HasPrefix(path, "/mcp/") || strings.HasPrefix(path, "/mcp") {
		return true
	}

	// Non-API paths (static files, health check, etc.) are public.
	if !strings.HasPrefix(path, "/api/") {
		return true
	}

	return false
}
