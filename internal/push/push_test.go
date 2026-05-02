package push

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"botka/internal/models"
)

// genSubscriberKeys produces a fresh P-256 keypair encoded as base64url,
// matching what a browser would send. webpush-go validates that the public
// key is a real point on the curve, so we cannot use a hard-coded value.
func genSubscriberKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	curve := ecdh.P256()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	p256dh = base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
	authBytes := make([]byte, 16)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("generate auth: %v", err)
	}
	auth = base64.RawURLEncoding.EncodeToString(authBytes)
	return p256dh, auth
}

func setupPushTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_TEST_URL")
	if dsn == "" {
		dsn = "postgres://botka:botka@localhost:5432/botka_test?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("test database unavailable: %v", err)
	}
	// Recreate the user/push tables fresh for each test run.
	db.Exec("DROP TABLE IF EXISTS push_subscriptions, users CASCADE")
	if err := db.AutoMigrate(&models.User{}, &models.PushSubscription{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func createPushUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	u := models.User{Username: "pushuser", PasswordHash: "x", Role: models.RoleAdmin}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

// stubClient is a webpush HTTPClient that records each request and replies
// with a fixed status. It also lets a test fail one specific subscription.
type stubClient struct {
	mu       sync.Mutex
	requests []*http.Request
	status   int
	// per-call statuses keyed by Endpoint URL; falls back to status.
	perEndpoint map[string]int
	calls       int32
}

func (s *stubClient) Do(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&s.calls, 1)
	s.mu.Lock()
	s.requests = append(s.requests, req)
	st := s.status
	if v, ok := s.perEndpoint[req.URL.String()]; ok {
		st = v
	}
	s.mu.Unlock()
	return &http.Response{
		StatusCode: st,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func newSenderForTest(db *gorm.DB, client *stubClient) *webPushSender {
	priv, pub, err := generateTestKeys()
	if err != nil {
		panic(err)
	}
	return &webPushSender{
		db:         db,
		publicKey:  pub,
		privateKey: priv,
		subject:    "mailto:test@example.com",
		httpClient: client,
	}
}

// generateTestKeys uses the package's own dependency to produce keys that
// pass the same validation the production sender will run.
func generateTestKeys() (string, string, error) {
	// Imported via webpush-go; reuse here to avoid hard-coded magic values.
	return webpushGenerate()
}

func TestSend_NoSubscriptions_NoOp(t *testing.T) {
	db := setupPushTestDB(t)
	user := createPushUser(t, db)

	stub := &stubClient{status: http.StatusCreated}
	s := newSenderForTest(db, stub)

	if err := s.Send(context.Background(), user.ID, PushPayload{Title: "x", Body: "y"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := atomic.LoadInt32(&stub.calls); got != 0 {
		t.Errorf("expected 0 push requests, got %d", got)
	}
}

func TestSend_SuccessUpdatesLastUsedAt(t *testing.T) {
	db := setupPushTestDB(t)
	user := createPushUser(t, db)
	p256, auth := genSubscriberKeys(t)

	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: "https://stub.example/push/abc",
		P256dh:   p256,
		Auth:     auth,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create sub: %v", err)
	}

	stub := &stubClient{status: http.StatusCreated}
	s := newSenderForTest(db, stub)

	if err := s.Send(context.Background(), user.ID, PushPayload{Title: "hello", Body: "world"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if got := atomic.LoadInt32(&stub.calls); got != 1 {
		t.Fatalf("expected 1 push request, got %d", got)
	}

	var updated models.PushSubscription
	if err := db.First(&updated, sub.ID).Error; err != nil {
		t.Fatalf("reload sub: %v", err)
	}
	if updated.LastUsedAt == nil {
		t.Errorf("expected last_used_at to be set after success")
	}
}

func TestSend_DeletesDeadSubscriptionOn410(t *testing.T) {
	db := setupPushTestDB(t)
	user := createPushUser(t, db)
	p256, auth := genSubscriberKeys(t)

	deadEndpoint := "https://stub.example/push/dead"
	live := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: "https://stub.example/push/live",
		P256dh:   p256,
		Auth:     auth,
	}
	dead := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: deadEndpoint,
		P256dh:   p256,
		Auth:     auth,
	}
	if err := db.Create(&live).Error; err != nil {
		t.Fatalf("create live: %v", err)
	}
	if err := db.Create(&dead).Error; err != nil {
		t.Fatalf("create dead: %v", err)
	}

	stub := &stubClient{
		status:      http.StatusCreated,
		perEndpoint: map[string]int{deadEndpoint: http.StatusGone},
	}
	s := newSenderForTest(db, stub)

	if err := s.Send(context.Background(), user.ID, PushPayload{Title: "t", Body: "b"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var count int64
	db.Model(&models.PushSubscription{}).Where("id = ?", dead.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected dead subscription to be deleted, count=%d", count)
	}
	db.Model(&models.PushSubscription{}).Where("id = ?", live.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected live subscription to remain, count=%d", count)
	}
}

func TestSend_404AlsoDeletes(t *testing.T) {
	db := setupPushTestDB(t)
	user := createPushUser(t, db)
	p256, auth := genSubscriberKeys(t)

	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: "https://stub.example/push/missing",
		P256dh:   p256,
		Auth:     auth,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create sub: %v", err)
	}

	stub := &stubClient{status: http.StatusNotFound}
	s := newSenderForTest(db, stub)

	if err := s.Send(context.Background(), user.ID, PushPayload{Title: "t", Body: "b"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var count int64
	db.Model(&models.PushSubscription{}).Where("id = ?", sub.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected sub deleted on 404, count=%d", count)
	}
}

func TestSend_PayloadIsJSONInRequestBody(t *testing.T) {
	db := setupPushTestDB(t)
	user := createPushUser(t, db)
	p256, auth := genSubscriberKeys(t)

	sub := models.PushSubscription{
		UserID:   user.ID,
		Endpoint: "https://stub.example/push/payload",
		P256dh:   p256,
		Auth:     auth,
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("create sub: %v", err)
	}

	stub := &stubClient{status: http.StatusCreated}
	s := newSenderForTest(db, stub)

	if err := s.Send(context.Background(), user.ID, PushPayload{
		Title: "Hello", Body: "world", URL: "/threads/1", Tag: "thread-1",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(stub.requests))
	}
	req := stub.requests[0]
	// The body is encrypted by webpush-go, so we don't decode it. We only
	// verify the request was made to the right endpoint with a non-empty
	// encrypted body and the expected headers.
	if req.URL.String() != sub.Endpoint {
		t.Errorf("expected request to %s, got %s", sub.Endpoint, req.URL.String())
	}
	var buf bytes.Buffer
	if req.Body != nil {
		if _, err := buf.ReadFrom(req.Body); err != nil {
			t.Fatalf("read body: %v", err)
		}
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty encrypted body")
	}
	if req.Header.Get("Authorization") == "" {
		t.Error("expected Authorization (VAPID) header")
	}
}

func TestBroadcast_HitsAllUsers(t *testing.T) {
	db := setupPushTestDB(t)
	u1 := createPushUser(t, db)
	u2 := models.User{Username: "second", PasswordHash: "x", Role: models.RoleAdmin}
	if err := db.Create(&u2).Error; err != nil {
		t.Fatalf("create user2: %v", err)
	}

	for i, uid := range []int64{u1.ID, u2.ID} {
		p256, auth := genSubscriberKeys(t)
		s := models.PushSubscription{
			UserID:   uid,
			Endpoint: "https://stub.example/push/" + string(rune('a'+i)),
			P256dh:   p256,
			Auth:     auth,
		}
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("create sub: %v", err)
		}
	}

	stub := &stubClient{status: http.StatusCreated}
	s := newSenderForTest(db, stub)

	if err := s.Broadcast(context.Background(), PushPayload{Title: "all", Body: "users"}); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if got := atomic.LoadInt32(&stub.calls); got != 2 {
		t.Errorf("expected 2 broadcast requests, got %d", got)
	}
}
