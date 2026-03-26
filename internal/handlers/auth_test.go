package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"botka/internal/middleware"
	"botka/internal/models"
)

func setupAuthRouter(t *testing.T) (*gin.Engine, *AuthHandler) {
	t.Helper()
	db := setupTestDB(t)
	cleanTables(t, db)

	router := gin.New()
	router.Use(middleware.Auth(db))

	h := NewAuthHandler(db, 24*time.Hour, false)
	v1 := router.Group("/api/v1")
	RegisterAuthRoutes(v1, h)

	return router, h
}

func createTestUser(t *testing.T, password string) {
	t.Helper()
	db := setupTestDB(t)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := models.User{
		Username:     "testuser",
		PasswordHash: string(hash),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}
}

func TestLogin_Success(t *testing.T) {
	router, _ := setupAuthRouter(t)
	createTestUser(t, "testpass123")

	w := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"testuser","password":"testpass123"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Check response contains username.
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data object")
	}
	if data["username"] != "testuser" {
		t.Errorf("expected username testuser, got %v", data["username"])
	}

	// Check session cookie was set.
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == middleware.SessionCookieName {
			found = true
			if c.HttpOnly != true {
				t.Error("cookie should be HttpOnly")
			}
			if len(c.Value) != 64 {
				t.Errorf("expected 64-char token, got %d", len(c.Value))
			}
		}
	}
	if !found {
		t.Error("session cookie not set")
	}
}

func TestLogin_InvalidPassword(t *testing.T) {
	router, _ := setupAuthRouter(t)
	createTestUser(t, "testpass123")

	w := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"testuser","password":"wrongpass"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogin_InvalidUsername(t *testing.T) {
	router, _ := setupAuthRouter(t)
	createTestUser(t, "testpass123")

	w := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"nonexistent","password":"testpass123"}`)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogin_MissingFields(t *testing.T) {
	router, _ := setupAuthRouter(t)

	w := doRequest(router, "POST", "/api/v1/auth/login", `{}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMe_Authenticated(t *testing.T) {
	router, h := setupAuthRouter(t)
	createTestUser(t, "testpass123")

	// Login first to get a session.
	loginW := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"testuser","password":"testpass123"}`)
	if loginW.Code != http.StatusOK {
		t.Fatalf("login failed: %d", loginW.Code)
	}

	var sessionCookie string
	for _, c := range loginW.Result().Cookies() {
		if c.Name == middleware.SessionCookieName {
			sessionCookie = c.Value
		}
	}
	_ = h // used indirectly

	// Call /me with the session cookie.
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: sessionCookie})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["username"] != "testuser" {
		t.Errorf("expected testuser, got %v", data["username"])
	}
}

func TestMe_Unauthenticated(t *testing.T) {
	router, _ := setupAuthRouter(t)

	w := doRequest(router, "GET", "/api/v1/auth/me", "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogout(t *testing.T) {
	router, _ := setupAuthRouter(t)
	createTestUser(t, "testpass123")

	// Login.
	loginW := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"testuser","password":"testpass123"}`)
	var sessionCookie string
	for _, c := range loginW.Result().Cookies() {
		if c.Name == middleware.SessionCookieName {
			sessionCookie = c.Value
		}
	}

	// Logout.
	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: sessionCookie})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify session is cleared: /me should fail.
	req2 := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	req2.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: sessionCookie})
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", w2.Code)
	}
}

func TestAuthMiddleware_ProtectedRoute(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	router := gin.New()
	router.Use(middleware.Auth(db))
	router.GET("/api/v1/threads", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})

	// Without auth: should get 401.
	w := doRequest(router, "GET", "/api/v1/threads", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// With valid session: should get 200.
	hash, _ := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.MinCost)
	user := models.User{Username: "authtest", PasswordHash: string(hash)}
	db.Create(&user)

	session := models.Session{
		ID:        "testsessiontoken1234567890abcdef1234567890abcdef1234567890abcdef",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	db.Create(&session)

	req := httptest.NewRequest("GET", "/api/v1/threads", nil)
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: session.ID})
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid session, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestAuthMiddleware_ExpiredSession(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	router := gin.New()
	router.Use(middleware.Auth(db))
	router.GET("/api/v1/threads", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})

	hash, _ := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.MinCost)
	user := models.User{Username: "expiredtest", PasswordHash: string(hash)}
	db.Create(&user)

	session := models.Session{
		ID:        "expiredsession1234567890abcdef1234567890abcdef1234567890abcdef1",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(-time.Hour), // expired
	}
	db.Create(&session)

	req := httptest.NewRequest("GET", "/api/v1/threads", nil)
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: session.ID})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired session, got %d", w.Code)
	}
}

func TestAuthMiddleware_PublicEndpoints(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	router := gin.New()
	router.Use(middleware.Auth(db))

	// Auth endpoints should be public.
	router.POST("/api/v1/auth/login", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	router.GET("/api/v1/auth/me", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/me"} {
		method := "GET"
		if path == "/api/v1/auth/login" {
			method = "POST"
		}
		w := doRequest(router, method, path, "")
		if w.Code != http.StatusOK {
			t.Errorf("expected %s to be public (200), got %d", path, w.Code)
		}
	}
}

func TestChangePassword(t *testing.T) {
	router, _ := setupAuthRouter(t)
	createTestUser(t, "oldpass123")

	// Login first.
	loginW := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"testuser","password":"oldpass123"}`)
	var sessionCookie string
	for _, c := range loginW.Result().Cookies() {
		if c.Name == middleware.SessionCookieName {
			sessionCookie = c.Value
		}
	}

	// Change password.
	body := `{"current_password":"oldpass123","new_password":"newpass456"}`
	req := httptest.NewRequest("PUT", "/api/v1/auth/password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: sessionCookie})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Login with new password.
	w2 := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"testuser","password":"newpass456"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected login with new password to succeed, got %d", w2.Code)
	}

	// Login with old password should fail.
	w3 := doRequest(router, "POST", "/api/v1/auth/login", `{"username":"testuser","password":"oldpass123"}`)
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password to fail, got %d", w3.Code)
	}
}
