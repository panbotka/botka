package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/webauthn"
	"golang.org/x/crypto/bcrypt"

	"botka/internal/middleware"
	"botka/internal/models"
)

func setupPasskeyRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db := setupTestDB(t)
	cleanTables(t, db)

	wan, err := webauthn.New(&webauthn.Config{
		RPID:          "localhost",
		RPDisplayName: "Botka Test",
		RPOrigins:     []string{"http://localhost:5110"},
	})
	if err != nil {
		t.Fatalf("create webauthn: %v", err)
	}

	authHandler := NewAuthHandler(db, 24*time.Hour, false)
	passkeyHandler := NewPasskeyHandler(db, wan, authHandler)

	router := gin.New()
	router.Use(middleware.Auth(db))
	v1 := router.Group("/api/v1")
	RegisterPasskeyRoutes(v1, passkeyHandler)
	RegisterAuthRoutes(v1, authHandler)

	return router
}

func TestLoginBegin_ReturnsValidShape(t *testing.T) {
	router := setupPasskeyRouter(t)

	w := doRequest(router, "POST", "/api/v1/auth/passkey/login/begin", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Parse the response and verify the shape: {"data": {"publicKey": {...}}}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data object in response")
	}

	publicKey, ok := data["publicKey"].(map[string]interface{})
	if !ok {
		t.Fatal("expected publicKey object inside data")
	}

	// Verify required fields in publicKey.
	if _, ok := publicKey["challenge"]; !ok {
		t.Error("publicKey missing challenge field")
	}
	if _, ok := publicKey["rpId"]; !ok {
		t.Error("publicKey missing rpId field")
	}
	if publicKey["rpId"] != "localhost" {
		t.Errorf("expected rpId 'localhost', got %v", publicKey["rpId"])
	}
}

func TestLoginFinish_NoPendingSession(t *testing.T) {
	router := setupPasskeyRouter(t)

	// Attempt to finish login without calling begin first.
	body := `{"id":"test","rawId":"dGVzdA","type":"public-key","response":{"authenticatorData":"dGVzdA","clientDataJSON":"dGVzdA","signature":"dGVzdA","userHandle":"dGVzdA"}}`
	w := doRequest(router, "POST", "/api/v1/auth/passkey/login/finish", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "no pending login" {
		t.Errorf("expected 'no pending login' error, got %v", resp["error"])
	}
}

func TestPasskeyEndpoints_ArePublic(t *testing.T) {
	router := setupPasskeyRouter(t)

	// login/begin should be accessible without authentication.
	w := doRequest(router, "POST", "/api/v1/auth/passkey/login/begin", "")
	if w.Code == http.StatusUnauthorized {
		t.Error("login/begin should be public, got 401")
	}

	// login/finish should be accessible without authentication.
	// With no pending session, we expect 400 "no pending login" — NOT 401 from the auth middleware.
	// Use a fresh router to avoid leftover sessions from login/begin above.
	router2 := setupPasskeyRouter(t)
	w2 := doRequest(router2, "POST", "/api/v1/auth/passkey/login/finish", "")
	if w2.Code == http.StatusUnauthorized {
		// Verify it's not the auth middleware blocking us.
		var resp map[string]interface{}
		json.Unmarshal(w2.Body.Bytes(), &resp)
		if resp["error"] == "unauthorized" {
			t.Error("login/finish should be public, got 401 from auth middleware")
		}
	}
}

func TestCredentialFlagsStoredAndLoaded(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Create a user.
	hash, _ := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.MinCost)
	user := models.User{Username: "flagtest", PasswordHash: string(hash)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Store a credential with backup flags set.
	cred := models.WebAuthnCredential{
		UserID:         user.ID,
		CredentialID:   []byte("test-cred-id"),
		PublicKey:      []byte("test-public-key"),
		AAGUID:         []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		SignCount:      0,
		BackupEligible: true,
		BackupState:    true,
		Name:           "Test Passkey",
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}

	// Load the credential back and verify flags.
	var loaded models.WebAuthnCredential
	if err := db.Where("credential_id = ?", []byte("test-cred-id")).First(&loaded).Error; err != nil {
		t.Fatalf("load credential: %v", err)
	}
	if !loaded.BackupEligible {
		t.Error("expected BackupEligible to be true")
	}
	if !loaded.BackupState {
		t.Error("expected BackupState to be true")
	}

	// Verify loadWebAuthnUser populates the flags.
	wan, _ := webauthn.New(&webauthn.Config{
		RPID:          "localhost",
		RPDisplayName: "Test",
		RPOrigins:     []string{"http://localhost"},
	})
	authHandler := NewAuthHandler(db, 24*time.Hour, false)
	h := NewPasskeyHandler(db, wan, authHandler)

	wanUser := h.loadWebAuthnUser(&user)
	creds := wanUser.WebAuthnCredentials()
	if len(creds) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(creds))
	}
	if !creds[0].Flags.BackupEligible {
		t.Error("expected loaded webauthn credential to have BackupEligible=true")
	}
	if !creds[0].Flags.BackupState {
		t.Error("expected loaded webauthn credential to have BackupState=true")
	}
}

func TestRegisterBegin_RequiresAuth(t *testing.T) {
	router := setupPasskeyRouter(t)

	// Registration endpoints require authentication.
	w := doRequest(router, "POST", "/api/v1/auth/passkey/register/begin", "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated register/begin, got %d", w.Code)
	}
}

func TestRegisterBegin_Authenticated(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	wan, _ := webauthn.New(&webauthn.Config{
		RPID:          "localhost",
		RPDisplayName: "Botka Test",
		RPOrigins:     []string{"http://localhost:5110"},
	})
	authHandler := NewAuthHandler(db, 24*time.Hour, false)
	passkeyHandler := NewPasskeyHandler(db, wan, authHandler)

	router := gin.New()
	router.Use(middleware.Auth(db))
	v1 := router.Group("/api/v1")
	RegisterPasskeyRoutes(v1, passkeyHandler)
	RegisterAuthRoutes(v1, authHandler)

	// Create a user and session.
	hash, _ := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.MinCost)
	user := models.User{Username: "regtest", PasswordHash: string(hash)}
	db.Create(&user)

	session := models.Session{
		ID:        "regtestsession1234567890abcdef1234567890abcdef1234567890abcdef1",
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	db.Create(&session)

	req := httptest.NewRequest("POST", "/api/v1/auth/passkey/register/begin", nil)
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: session.ID})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify response contains publicKey with challenge.
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if _, ok := data["publicKey"]; !ok {
		t.Error("expected publicKey in register/begin response")
	}
}
