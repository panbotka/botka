package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"botka/internal/models"
)

func init() { gin.SetMode(gin.TestMode) }

func TestIsPublicPath(t *testing.T) {
	tests := []struct {
		path   string
		public bool
	}{
		{"/api/v1/auth/login", true},
		{"/api/v1/auth/me", true},
		{"/api/v1/auth/passkey/login/begin", true},
		{"/api/v1/auth/passkey/login/finish", true},
		{"/api/v1/threads", false},
		{"/api/v1/tasks", false},
		{"/api/v1/auth/logout", false},
		{"/api/v1/auth/password", false},
		{"/api/v1/auth/passkeys", false},
		{"/mcp/sse", true},
		{"/", true},
		{"/login", true},
		{"/settings", true},
	}

	for _, tt := range tests {
		got := isPublicPath(tt.path)
		if got != tt.public {
			t.Errorf("isPublicPath(%q) = %v, want %v", tt.path, got, tt.public)
		}
	}
}

func TestAuth_PublicPathPassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(Auth(nil))
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

func TestAuth_ProtectedPathWithoutCookie(t *testing.T) {
	r := gin.New()
	r.Use(Auth(nil))
	r.GET("/api/v1/threads", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/api/v1/threads", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing cookie, got %d", w.Code)
	}
}

func TestGetUser_NilWhenNotSet(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if user := GetUser(c); user != nil {
		t.Errorf("expected nil user from empty context, got %v", user)
	}
}

func TestGetUser_ReturnsUser(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	want := &models.User{ID: 42, Username: "test", Role: models.RoleAdmin}
	c.Set(ContextKeyUser, want)

	got := GetUser(c)
	if got == nil {
		t.Fatal("expected non-nil user")
	}
	if got.ID != want.ID {
		t.Errorf("user ID = %d, want %d", got.ID, want.ID)
	}
	if got.Username != want.Username {
		t.Errorf("username = %q, want %q", got.Username, want.Username)
	}
}

func TestGetUser_NilWhenWrongType(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ContextKeyUser, "not-a-user")

	if user := GetUser(c); user != nil {
		t.Errorf("expected nil for wrong type, got %v", user)
	}
}

func TestAdminOnly_Blocks_NoUser(t *testing.T) {
	r := gin.New()
	r.Use(AdminOnly())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 with no user, got %d", w.Code)
	}
}

func TestAdminOnly_Blocks_ExternalUser(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUser, &models.User{ID: 1, Role: models.RoleExternal})
		c.Next()
	})
	r.Use(AdminOnly())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for external user, got %d", w.Code)
	}
}

func TestAdminOnly_Passes_AdminUser(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUser, &models.User{ID: 1, Role: models.RoleAdmin})
		c.Next()
	})
	r.Use(AdminOnly())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin user, got %d", w.Code)
	}
}
