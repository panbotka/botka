package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"botka/internal/models"
)

func TestIsAllowedExternalPath(t *testing.T) {
	tests := []struct {
		path    string
		method  string
		allowed bool
	}{
		// Thread list — allowed
		{"/api/v1/threads", "GET", true},
		// Thread list — POST not allowed (no thread creation)
		{"/api/v1/threads", "POST", false},
		// Thread detail — GET allowed
		{"/api/v1/threads/42", "GET", true},
		// Thread detail — DELETE not allowed
		{"/api/v1/threads/42", "DELETE", false},
		// Thread detail — PUT not allowed (rename)
		{"/api/v1/threads/42", "PUT", false},
		// Messages — POST and GET allowed
		{"/api/v1/threads/42/messages", "POST", true},
		{"/api/v1/threads/42/messages", "GET", true},
		{"/api/v1/threads/42/messages", "DELETE", false},
		// Stream subscribe — allowed
		{"/api/v1/threads/42/stream/subscribe", "GET", true},
		// Regenerate — allowed
		{"/api/v1/threads/42/regenerate", "POST", true},
		// Branch — allowed
		{"/api/v1/threads/42/branch", "POST", true},
		// Session health — allowed
		{"/api/v1/threads/42/session-health", "GET", true},
		// Interrupt — allowed
		{"/api/v1/threads/42/interrupt", "POST", true},
		// Edit message — allowed
		{"/api/v1/threads/42/messages/123/edit", "POST", true},
		// Thread operations not allowed for external
		{"/api/v1/threads/42/pin", "PUT", false},
		{"/api/v1/threads/42/archive", "PUT", false},
		{"/api/v1/threads/42/model", "PUT", false},
		{"/api/v1/threads/42/project", "PUT", false},
		{"/api/v1/threads/42/tags", "PUT", false},
		{"/api/v1/threads/42/usage", "GET", false},
		// Auth endpoints — allowed
		{"/api/v1/auth/login", "POST", true},
		{"/api/v1/auth/me", "GET", true},
		{"/api/v1/auth/logout", "POST", true},
		// Transcribe — allowed
		{"/api/v1/transcribe", "POST", true},
		{"/api/v1/transcribe/status", "GET", true},
		// Status — allowed
		{"/api/v1/status", "GET", true},
		// Admin-only endpoints — forbidden
		{"/api/v1/tasks", "GET", false},
		{"/api/v1/projects", "GET", false},
		{"/api/v1/settings", "GET", false},
		{"/api/v1/personas", "GET", false},
		{"/api/v1/tags", "GET", false},
		{"/api/v1/memories", "GET", false},
		{"/api/v1/analytics/cost", "GET", false},
		{"/api/v1/users", "GET", false},
		{"/api/v1/runner/status", "GET", false},
		{"/api/v1/search", "GET", false},
		// Non-numeric thread ID
		{"/api/v1/threads/abc", "GET", false},
	}

	for _, tt := range tests {
		got := isAllowedExternalPath(tt.path, tt.method)
		if got != tt.allowed {
			t.Errorf("isAllowedExternalPath(%q, %q) = %v, want %v", tt.path, tt.method, got, tt.allowed)
		}
	}
}

func TestExtractThreadID(t *testing.T) {
	tests := []struct {
		path string
		want int64
	}{
		{"/api/v1/threads/42", 42},
		{"/api/v1/threads/42/messages", 42},
		{"/api/v1/threads/100/stream/subscribe", 100},
		{"/api/v1/threads/abc", 0},
		{"/api/v1/tasks", 0},
		{"/api/v1/threads", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := extractThreadID(tt.path)
		if got != tt.want {
			t.Errorf("extractThreadID(%q) = %d, want %d", tt.path, got, tt.want)
		}
	}
}

// TestExternalAccess_AdminPassesThrough verifies that admin users bypass
// external access restrictions entirely.
func TestExternalAccess_AdminPassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUser, &models.User{ID: 1, Role: models.RoleAdmin})
		c.Next()
	})
	r.Use(ExternalAccess(nil))
	r.GET("/api/v1/tasks", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/tasks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin user, got %d", w.Code)
	}
}

// TestExternalAccess_NoUserReturns401 verifies that requests without a user
// in the context get a 401 on protected paths.
func TestExternalAccess_NoUserReturns401(t *testing.T) {
	r := gin.New()
	r.Use(ExternalAccess(nil))
	r.GET("/api/v1/tasks", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/tasks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing user, got %d", w.Code)
	}
}

// TestExternalAccess_ExternalUserForbiddenOnAdminPath verifies that external
// users are blocked from admin-only paths (e.g., /api/v1/tasks).
func TestExternalAccess_ExternalUserForbiddenOnAdminPath(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUser, &models.User{ID: 2, Role: models.RoleExternal})
		c.Next()
	})
	r.Use(ExternalAccess(nil))
	r.GET("/api/v1/tasks", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/tasks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for external user on admin path, got %d", w.Code)
	}
}

// TestExternalAccess_PublicPathPassesThrough verifies that public paths
// pass through the ExternalAccess middleware without user checks.
func TestExternalAccess_PublicPathPassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(ExternalAccess(nil))
	r.POST("/api/v1/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for public path, got %d", w.Code)
	}
}

// TestExternalAccess_ExternalUserAllowedThreadList verifies that external
// users can access the thread list endpoint (GET /api/v1/threads).
func TestExternalAccess_ExternalUserAllowedThreadList(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUser, &models.User{ID: 2, Role: models.RoleExternal})
		c.Next()
	})
	r.Use(ExternalAccess(nil))
	r.GET("/api/v1/threads", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/threads", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Thread list is an allowed external path and has no thread ID to check,
	// so it should pass through.
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for external user on thread list, got %d", w.Code)
	}
}
