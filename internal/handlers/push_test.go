package handlers

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"botka/internal/middleware"
	"botka/internal/models"
	"botka/internal/push"
)

// fakeSender is an in-memory push.Sender used for handler tests.
type fakeSender struct {
	sent atomic.Int64
}

func (f *fakeSender) Send(_ context.Context, _ int64, _ push.PushPayload) error {
	f.sent.Add(1)
	return nil
}
func (f *fakeSender) Broadcast(_ context.Context, _ push.PushPayload) error { return nil }

// pushTestRouter mounts the push handler with optional auth and sender.
// When sender is nil and publicKey is empty the handler is in "disabled"
// mode; tests use this to assert 503 responses.
func pushTestRouter(db *gorm.DB, sender push.Sender, publicKey string, withAuth bool) *gin.Engine {
	r := gin.New()
	if withAuth {
		r.Use(middleware.Auth(db))
	}
	v1 := r.Group("/api/v1")
	h := NewPushHandler(db, sender, publicKey)
	RegisterPushRoutes(v1, h)
	return r
}

func createPushTestUser(t *testing.T, db *gorm.DB, username string) (models.User, string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := models.User{Username: username, PasswordHash: string(hash), Role: models.RoleAdmin}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	sess := models.Session{
		ID:        username + "sessiontoken1234567890abcdef1234567890abcdef1234567890abcdef",
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	// Pad/truncate to 64 chars to satisfy possible varchar constraints.
	if len(sess.ID) > 64 {
		sess.ID = sess.ID[:64]
	}
	for len(sess.ID) < 64 {
		sess.ID += "x"
	}
	if err := db.Create(&sess).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	return u, sess.ID
}

func doRequestAuth(router *gin.Engine, method, path, cookie, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: cookie})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func genHandlerSubKeys(t *testing.T) (string, string) {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	authBytes := make([]byte, 16)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("gen auth: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
		base64.RawURLEncoding.EncodeToString(authBytes)
}

func TestPush_VAPIDDisabled_ReturnsServiceUnavailable(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	_, cookie := createPushTestUser(t, db, "disabled")

	r := pushTestRouter(db, nil, "", true)
	for _, path := range []string{
		"/api/v1/push/vapid-public-key",
		"/api/v1/push/subscriptions",
	} {
		w := doRequestAuth(r, http.MethodGet, path, cookie, "")
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: expected 503, got %d", path, w.Code)
		}
	}
	w := doRequestAuth(r, http.MethodPost, "/api/v1/push/subscriptions", cookie,
		`{"endpoint":"e","keys":{"p256dh":"a","auth":"b"}}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("create: expected 503, got %d", w.Code)
	}
	w = doRequestAuth(r, http.MethodPost, "/api/v1/push/test", cookie, "{}")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("test: expected 503, got %d", w.Code)
	}
}

func TestPush_Unauthenticated_Returns401(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := pushTestRouter(db, &fakeSender{}, "pubkey", true)
	w := doRequest(r, http.MethodGet, "/api/v1/push/vapid-public-key", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	w = doRequest(r, http.MethodGet, "/api/v1/push/subscriptions", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPush_VAPIDPublicKey_ReturnsKey(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	_, cookie := createPushTestUser(t, db, "vapidkey")

	r := pushTestRouter(db, &fakeSender{}, "publickeybase64", true)
	w := doRequestAuth(r, http.MethodGet, "/api/v1/push/vapid-public-key", cookie, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			PublicKey string `json:"public_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.PublicKey != "publickeybase64" {
		t.Errorf("expected publickeybase64, got %q", resp.Data.PublicKey)
	}
}

func TestPush_CreateSubscription_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	user, cookie := createPushTestUser(t, db, "createsub")
	p256, auth := genHandlerSubKeys(t)

	r := pushTestRouter(db, &fakeSender{}, "pub", true)
	body := fmt.Sprintf(`{"endpoint":"https://push.example/abc","keys":{"p256dh":"%s","auth":"%s"},"user_agent":"Firefox/123"}`, p256, auth)

	w := doRequestAuth(r, http.MethodPost, "/api/v1/push/subscriptions", cookie, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Second POST with same endpoint returns existing row, not a new one.
	w2 := doRequestAuth(r, http.MethodPost, "/api/v1/push/subscriptions", cookie, body)
	if w2.Code != http.StatusCreated {
		t.Fatalf("second create: expected 201, got %d", w2.Code)
	}

	var count int64
	db.Model(&models.PushSubscription{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 subscription after duplicate POST, got %d", count)
	}
}

func TestPush_CreateSubscription_MissingFields(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	_, cookie := createPushTestUser(t, db, "missingfields")

	r := pushTestRouter(db, &fakeSender{}, "pub", true)
	body := `{"endpoint":"","keys":{"p256dh":"","auth":""}}`
	w := doRequestAuth(r, http.MethodPost, "/api/v1/push/subscriptions", cookie, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPush_CreateSubscription_MalformedJSON(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	_, cookie := createPushTestUser(t, db, "malformed")

	r := pushTestRouter(db, &fakeSender{}, "pub", true)
	w := doRequestAuth(r, http.MethodPost, "/api/v1/push/subscriptions", cookie, `not json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPush_ListSubscriptions(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	user, cookie := createPushTestUser(t, db, "listsub")
	other, _ := createPushTestUser(t, db, "othersub")
	p256a, autha := genHandlerSubKeys(t)
	p256b, authb := genHandlerSubKeys(t)
	p256c, authc := genHandlerSubKeys(t)

	for _, e := range []string{"https://push.example/own1", "https://push.example/own2"} {
		s := models.PushSubscription{UserID: user.ID, Endpoint: e, P256dh: p256a, Auth: autha}
		_ = db.Create(&s).Error
		p256a, autha = p256b, authb
	}
	// Other user's subscription should NOT appear in the list.
	otherSub := models.PushSubscription{UserID: other.ID, Endpoint: "https://push.example/other", P256dh: p256c, Auth: authc}
	if err := db.Create(&otherSub).Error; err != nil {
		t.Fatalf("create other sub: %v", err)
	}

	r := pushTestRouter(db, &fakeSender{}, "pub", true)
	w := doRequestAuth(r, http.MethodGet, "/api/v1/push/subscriptions", cookie, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data  []models.PushSubscription `json:"data"`
		Total int64                     `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Data))
	}
	for _, s := range resp.Data {
		if s.UserID != user.ID {
			t.Errorf("returned subscription for wrong user: %d", s.UserID)
		}
	}
}

func TestPush_DeleteSubscription_OwnVsOther(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	user, cookie := createPushTestUser(t, db, "deletesub")
	other, _ := createPushTestUser(t, db, "deletesubother")
	p256, auth := genHandlerSubKeys(t)

	own := models.PushSubscription{UserID: user.ID, Endpoint: "https://push.example/own", P256dh: p256, Auth: auth}
	if err := db.Create(&own).Error; err != nil {
		t.Fatalf("create own: %v", err)
	}
	notOwn := models.PushSubscription{UserID: other.ID, Endpoint: "https://push.example/notown", P256dh: p256, Auth: auth}
	if err := db.Create(&notOwn).Error; err != nil {
		t.Fatalf("create other: %v", err)
	}

	r := pushTestRouter(db, &fakeSender{}, "pub", true)

	// Own subscription deletes successfully -> 204.
	w := doRequestAuth(r, http.MethodDelete,
		fmt.Sprintf("/api/v1/push/subscriptions/%d", own.ID), cookie, "")
	if w.Code != http.StatusNoContent {
		t.Errorf("delete own: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Trying to delete another user's subscription returns 404.
	w = doRequestAuth(r, http.MethodDelete,
		fmt.Sprintf("/api/v1/push/subscriptions/%d", notOwn.ID), cookie, "")
	if w.Code != http.StatusNotFound {
		t.Errorf("delete other: expected 404, got %d", w.Code)
	}

	// The other user's row is still present.
	var stillThere int64
	db.Model(&models.PushSubscription{}).Where("id = ?", notOwn.ID).Count(&stillThere)
	if stillThere != 1 {
		t.Errorf("expected other user's sub to remain, count=%d", stillThere)
	}
}

func TestPush_TestEndpoint_CallsSender(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	user, cookie := createPushTestUser(t, db, "testendpoint")
	p256, auth := genHandlerSubKeys(t)

	for i := 0; i < 2; i++ {
		s := models.PushSubscription{
			UserID: user.ID, Endpoint: fmt.Sprintf("https://push.example/test%d", i),
			P256dh: p256, Auth: auth,
		}
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("create sub: %v", err)
		}
	}

	sender := &fakeSender{}
	r := pushTestRouter(db, sender, "pub", true)
	w := doRequestAuth(r, http.MethodPost, "/api/v1/push/test", cookie,
		`{"title":"Hi","body":"There"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := sender.sent.Load(); got != 1 {
		t.Errorf("expected sender.Send called once, got %d", got)
	}

	var resp struct {
		Data struct {
			Sent int64 `json:"sent"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Sent != 2 {
		t.Errorf("expected sent=2, got %d", resp.Data.Sent)
	}
}
