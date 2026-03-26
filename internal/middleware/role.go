package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// AdminOnly returns middleware that restricts access to admin users.
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetUser(c)
		if user == nil || !user.IsAdmin() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

// ExternalAccess returns middleware that enforces thread-level access for
// external users. Admin users pass through unchanged. External users may
// only access thread endpoints for threads they have been granted access to.
func ExternalAccess(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip paths that don't require authentication.
		if isPublicPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		user := GetUser(c)
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		// Admins have full access.
		if user.IsAdmin() {
			c.Next()
			return
		}

		path := c.Request.URL.Path

		// External users can only access specific thread endpoints.
		if !isAllowedExternalPath(path, c.Request.Method) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}

		// For thread-specific endpoints, verify access.
		threadID := extractThreadID(path)
		if threadID > 0 {
			if !HasThreadAccess(db, user.ID, threadID) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
				return
			}
		}

		c.Next()
	}
}

// GetUser extracts the authenticated user from the Gin context.
func GetUser(c *gin.Context) *models.User {
	val, ok := c.Get(ContextKeyUser)
	if !ok {
		return nil
	}
	user, ok := val.(*models.User)
	if !ok {
		return nil
	}
	return user
}

// HasThreadAccess checks if a user has access to a specific thread.
func HasThreadAccess(db *gorm.DB, userID, threadID int64) bool {
	var count int64
	db.Model(&models.ThreadAccess{}).
		Where("user_id = ? AND thread_id = ?", userID, threadID).
		Count(&count)
	return count > 0
}

// isAllowedExternalPath checks if a path is accessible to external users.
func isAllowedExternalPath(path, method string) bool {
	// Thread list (filtered by access in handler).
	if path == "/api/v1/threads" && method == "GET" {
		return true
	}

	// Thread-specific endpoints that external users can access.
	if strings.HasPrefix(path, "/api/v1/threads/") {
		rest := strings.TrimPrefix(path, "/api/v1/threads/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 0 {
			return false
		}

		// Must start with a numeric thread ID.
		if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
			return false
		}

		// GET /api/v1/threads/:id — view thread
		if len(parts) == 1 && method == "GET" {
			return true
		}

		// Allowed sub-paths
		if len(parts) == 2 {
			sub := parts[1]
			switch {
			case sub == "messages" && (method == "POST" || method == "GET"):
				return true
			case sub == "stream/subscribe" && method == "GET":
				return true
			case sub == "regenerate" && method == "POST":
				return true
			case sub == "branch" && method == "POST":
				return true
			case sub == "session-health" && method == "GET":
				return true
			case sub == "interrupt" && method == "POST":
				return true
			case strings.HasPrefix(sub, "messages/") && strings.HasSuffix(sub, "/edit") && method == "POST":
				return true
			}
		}
	}

	// Auth endpoints (already public via auth middleware, but be explicit).
	if strings.HasPrefix(path, "/api/v1/auth/") {
		return true
	}

	// Transcribe (voice input).
	if path == "/api/v1/transcribe" && method == "POST" {
		return true
	}
	if path == "/api/v1/transcribe/status" && method == "GET" {
		return true
	}

	// Status endpoint.
	if path == "/api/v1/status" && method == "GET" {
		return true
	}

	return false
}

// extractThreadID extracts a thread ID from a path like /api/v1/threads/123/...
func extractThreadID(path string) int64 {
	if !strings.HasPrefix(path, "/api/v1/threads/") {
		return 0
	}
	rest := strings.TrimPrefix(path, "/api/v1/threads/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 {
		return 0
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0
	}
	return id
}
