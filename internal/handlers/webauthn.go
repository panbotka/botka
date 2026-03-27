package handlers

import (
	"encoding/binary"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"

	"botka/internal/middleware"
	"botka/internal/models"
)

// webAuthnUser adapts models.User to the webauthn.User interface.
type webAuthnUser struct {
	user  *models.User
	creds []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(u.user.ID))
	return b
}

func (u *webAuthnUser) WebAuthnName() string                       { return u.user.Username }
func (u *webAuthnUser) WebAuthnDisplayName() string                { return u.user.Username }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// PasskeyHandler handles WebAuthn passkey registration and authentication.
type PasskeyHandler struct {
	db          *gorm.DB
	wan         *webauthn.WebAuthn
	authHandler *AuthHandler

	mu       sync.Mutex
	sessions map[string]*sessionEntry // challenge -> session data
}

type sessionEntry struct {
	data      *webauthn.SessionData
	expiresAt time.Time
}

// NewPasskeyHandler creates a new PasskeyHandler.
func NewPasskeyHandler(db *gorm.DB, wan *webauthn.WebAuthn, authHandler *AuthHandler) *PasskeyHandler {
	h := &PasskeyHandler{
		db:          db,
		wan:         wan,
		authHandler: authHandler,
		sessions:    make(map[string]*sessionEntry),
	}
	go h.cleanupLoop()
	return h
}

// RegisterPasskeyRoutes attaches passkey endpoints to the given router group.
func RegisterPasskeyRoutes(rg *gin.RouterGroup, h *PasskeyHandler) {
	pk := rg.Group("/auth/passkey")
	pk.POST("/register/begin", h.RegisterBegin)
	pk.POST("/register/finish", h.RegisterFinish)
	pk.POST("/login/begin", h.LoginBegin)
	pk.POST("/login/finish", h.LoginFinish)

	rg.GET("/auth/passkeys", h.List)
	rg.DELETE("/auth/passkeys/:id", h.Delete)
}

// RegisterBegin starts passkey registration for the authenticated user.
func (h *PasskeyHandler) RegisterBegin(c *gin.Context) {
	userVal, ok := c.Get(middleware.ContextKeyUser)
	if !ok {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	u := userVal.(*models.User)

	wanUser := h.loadWebAuthnUser(u)

	creation, session, err := h.wan.BeginRegistration(wanUser)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to begin registration")
		return
	}

	h.storeSession(session.Challenge, session)
	respondOK(c, creation)
}

// RegisterFinish completes passkey registration and stores the credential.
func (h *PasskeyHandler) RegisterFinish(c *gin.Context) {
	userVal, ok := c.Get(middleware.ContextKeyUser)
	if !ok {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	u := userVal.(*models.User)

	// Parse the name from query param (the body is the WebAuthn response).
	name := c.Query("name")
	if name == "" {
		name = "Passkey"
	}

	wanUser := h.loadWebAuthnUser(u)

	// Find the session by looking through stored sessions for this user.
	session := h.findSessionForUser(wanUser.WebAuthnID())
	if session == nil {
		respondError(c, http.StatusBadRequest, "no pending registration")
		return
	}

	credential, err := h.wan.FinishRegistration(wanUser, *session, c.Request)
	if err != nil {
		respondError(c, http.StatusBadRequest, "registration failed")
		return
	}

	// Store the credential in the database.
	cred := models.WebAuthnCredential{
		UserID:       u.ID,
		CredentialID: credential.ID,
		PublicKey:    credential.PublicKey,
		AAGUID:       credential.Authenticator.AAGUID,
		SignCount:    credential.Authenticator.SignCount,
		Name:         name,
	}
	if err := h.db.Create(&cred).Error; err != nil {
		slog.Error("webauthn: failed to store credential", "error", err, "user", u.Username)
		respondError(c, http.StatusInternalServerError, "failed to store credential")
		return
	}

	h.removeSession(session.Challenge)

	respondOK(c, gin.H{"id": cred.ID, "name": cred.Name})
}

// LoginBegin starts passkey login (discoverable flow).
func (h *PasskeyHandler) LoginBegin(c *gin.Context) {
	assertion, session, err := h.wan.BeginDiscoverableLogin()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to begin login")
		return
	}

	h.storeSession(session.Challenge, session)
	respondOK(c, assertion)
}

// LoginFinish completes passkey login and creates a session.
func (h *PasskeyHandler) LoginFinish(c *gin.Context) {
	// Find any valid login session.
	session := h.findAnyLoginSession()
	if session == nil {
		respondError(c, http.StatusBadRequest, "no pending login")
		return
	}

	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		userID := binary.BigEndian.Uint64(userHandle)
		var user models.User
		if err := h.db.First(&user, userID).Error; err != nil {
			return nil, err
		}
		return h.loadWebAuthnUser(&user), nil
	}

	user, credential, err := h.wan.FinishPasskeyLogin(handler, *session, c.Request)
	if err != nil {
		slog.Error("webauthn: login failed", "error", err)
		respondError(c, http.StatusUnauthorized, "login failed")
		return
	}

	// Update sign count.
	h.db.Model(&models.WebAuthnCredential{}).
		Where("credential_id = ?", credential.ID).
		Update("sign_count", credential.Authenticator.SignCount)

	h.removeSession(session.Challenge)

	// Create a session for the user.
	wanUser := user.(*webAuthnUser)
	if err := h.authHandler.CreateSessionForUser(c, wanUser.user.ID); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create session")
		return
	}

	respondOK(c, gin.H{"username": wanUser.user.Username, "id": wanUser.user.ID})
}

// List returns all registered passkeys for the authenticated user.
func (h *PasskeyHandler) List(c *gin.Context) {
	userVal, ok := c.Get(middleware.ContextKeyUser)
	if !ok {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	u := userVal.(*models.User)

	var creds []models.WebAuthnCredential
	if err := h.db.Where("user_id = ?", u.ID).Order("created_at ASC").Find(&creds).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list passkeys")
		return
	}

	type passkeyItem struct {
		ID        int64     `json:"id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
	}

	items := make([]passkeyItem, len(creds))
	for i, cred := range creds {
		items[i] = passkeyItem{ID: cred.ID, Name: cred.Name, CreatedAt: cred.CreatedAt}
	}

	respondOK(c, items)
}

// Delete removes a passkey by ID.
func (h *PasskeyHandler) Delete(c *gin.Context) {
	userVal, ok := c.Get(middleware.ContextKeyUser)
	if !ok {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	u := userVal.(*models.User)

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid passkey id")
		return
	}

	result := h.db.Where("id = ? AND user_id = ?", id, u.ID).Delete(&models.WebAuthnCredential{})
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete passkey")
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "passkey not found")
		return
	}

	c.Status(http.StatusNoContent)
}

// loadWebAuthnUser loads a User and their credentials into a webAuthnUser.
func (h *PasskeyHandler) loadWebAuthnUser(u *models.User) *webAuthnUser {
	var dbCreds []models.WebAuthnCredential
	h.db.Where("user_id = ?", u.ID).Find(&dbCreds)

	creds := make([]webauthn.Credential, len(dbCreds))
	for i, c := range dbCreds {
		creds[i] = webauthn.Credential{
			ID:        c.CredentialID,
			PublicKey: c.PublicKey,
			Authenticator: webauthn.Authenticator{
				AAGUID:    c.AAGUID,
				SignCount: c.SignCount,
			},
		}
	}

	return &webAuthnUser{user: u, creds: creds}
}

// Session store management (in-memory with TTL).

func (h *PasskeyHandler) storeSession(challenge string, data *webauthn.SessionData) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[challenge] = &sessionEntry{
		data:      data,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
}

func (h *PasskeyHandler) findSessionForUser(userID []byte) *webauthn.SessionData {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for _, entry := range h.sessions {
		if entry.expiresAt.Before(now) {
			continue
		}
		if entry.data.UserID != nil && bytesEqual(entry.data.UserID, userID) {
			return entry.data
		}
	}
	return nil
}

func (h *PasskeyHandler) findAnyLoginSession() *webauthn.SessionData {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for _, entry := range h.sessions {
		if entry.expiresAt.Before(now) {
			continue
		}
		// Login sessions have nil UserID (discoverable flow).
		if len(entry.data.UserID) == 0 {
			return entry.data
		}
	}
	return nil
}

func (h *PasskeyHandler) removeSession(challenge string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, challenge)
}

func (h *PasskeyHandler) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		h.mu.Lock()
		now := time.Now()
		for k, entry := range h.sessions {
			if entry.expiresAt.Before(now) {
				delete(h.sessions, k)
			}
		}
		h.mu.Unlock()
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
