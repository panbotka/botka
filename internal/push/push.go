// Package push delivers Web Push notifications to subscribed browsers.
//
// The package exposes a Sender interface so trigger code (task events, chat
// replies) can call it without knowing about the HTTP details, and tests can
// inject a fake. NewSender returns nil + error if VAPID keys are not
// configured; callers should treat a nil Sender as "push disabled" and
// short-circuit accordingly.
package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"gorm.io/gorm"

	"botka/internal/config"
	"botka/internal/models"
)

// PushPayload is the payload delivered to the browser. It is marshaled to
// JSON and sent in the encrypted push message body; the service worker on
// the receiving end is responsible for displaying it as a notification.
type PushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// Sender delivers push notifications to subscribed browsers.
type Sender interface {
	// Send delivers the payload to every subscription belonging to userID.
	// Per-subscription failures are logged and dead subscriptions (404/410)
	// are deleted; the caller only sees an error for programming faults
	// (e.g. payload marshal failure).
	Send(ctx context.Context, userID int64, payload PushPayload) error

	// Broadcast delivers the payload to every subscription system-wide.
	Broadcast(ctx context.Context, payload PushPayload) error
}

// maxConcurrentSends bounds the number of in-flight push requests per Send
// or Broadcast call. The push services tolerate parallelism but we don't
// want a single broadcast to open hundreds of TCP connections.
const maxConcurrentSends = 10

// defaultPushTTL is the maximum time the push service will retain an
// undelivered message, in seconds.
const defaultPushTTL = 60 * 60 * 24

// httpClient is the interface used to send push requests; matches the
// webpush-go HTTPClient interface so we can swap in a stub during tests.
type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type webPushSender struct {
	db         *gorm.DB
	publicKey  string
	privateKey string
	subject    string
	httpClient httpClient
}

// webpushGenerate is a thin wrapper around webpush.GenerateVAPIDKeys so
// tests in this package can construct working keys without re-importing
// the dependency. The function returns (privateKey, publicKey).
func webpushGenerate() (string, string, error) {
	return webpush.GenerateVAPIDKeys()
}

// NewSender constructs a Sender from configuration. Returns (nil, error)
// when VAPID keys are missing or invalid; callers should log a warning and
// proceed with push disabled.
func NewSender(cfg *config.Config, db *gorm.DB) (Sender, error) {
	if cfg.VAPIDPublicKey == "" || cfg.VAPIDPrivateKey == "" {
		return nil, errors.New("VAPID keys not configured")
	}
	subject := cfg.VAPIDSubject
	if subject == "" {
		subject = "mailto:admin@example.com"
	}
	return &webPushSender{
		db:         db,
		publicKey:  cfg.VAPIDPublicKey,
		privateKey: cfg.VAPIDPrivateKey,
		subject:    subject,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Send fetches all subscriptions for the user and delivers the payload to
// each. See Sender.Send for error semantics.
func (s *webPushSender) Send(ctx context.Context, userID int64, payload PushPayload) error {
	var subs []models.PushSubscription
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&subs).Error; err != nil {
		return fmt.Errorf("fetch subscriptions: %w", err)
	}
	return s.sendToSubscriptions(ctx, subs, payload)
}

// Broadcast fetches every subscription and delivers the payload.
func (s *webPushSender) Broadcast(ctx context.Context, payload PushPayload) error {
	var subs []models.PushSubscription
	if err := s.db.WithContext(ctx).Find(&subs).Error; err != nil {
		return fmt.Errorf("fetch subscriptions: %w", err)
	}
	return s.sendToSubscriptions(ctx, subs, payload)
}

func (s *webPushSender) sendToSubscriptions(ctx context.Context, subs []models.PushSubscription, payload PushPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	if len(subs) == 0 {
		return nil
	}

	sem := make(chan struct{}, maxConcurrentSends)
	var wg sync.WaitGroup
	for _, sub := range subs {
		wg.Add(1)
		sem <- struct{}{}
		go func(sub models.PushSubscription) {
			defer wg.Done()
			defer func() { <-sem }()
			s.sendOne(ctx, sub, body)
		}(sub)
	}
	wg.Wait()
	return nil
}

// sendOne delivers a single payload to a single subscription. Errors are
// logged; dead subscriptions (404/410) are deleted from the database.
func (s *webPushSender) sendOne(ctx context.Context, sub models.PushSubscription, body []byte) {
	wpSub := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			Auth:   sub.Auth,
			P256dh: sub.P256dh,
		},
	}
	opts := &webpush.Options{
		HTTPClient:      s.httpClient,
		Subscriber:      s.subject,
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
		TTL:             defaultPushTTL,
	}

	// webpush-go pads the input slice in place when generating the encrypted
	// record, so we copy per call to keep parallel sends race-free.
	bodyCopy := append([]byte(nil), body...)
	resp, err := webpush.SendNotificationWithContext(ctx, bodyCopy, wpSub, opts)
	if err != nil {
		slog.Warn("push send failed", "subscription_id", sub.ID, "error", err)
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		if err := s.db.WithContext(ctx).Delete(&models.PushSubscription{}, sub.ID).Error; err != nil {
			slog.Warn("delete dead subscription", "subscription_id", sub.ID, "error", err)
		} else {
			slog.Info("deleted dead push subscription", "subscription_id", sub.ID, "status", resp.StatusCode)
		}
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		now := time.Now()
		if err := s.db.WithContext(ctx).Model(&models.PushSubscription{}).
			Where("id = ?", sub.ID).
			Update("last_used_at", now).Error; err != nil {
			slog.Warn("update last_used_at", "subscription_id", sub.ID, "error", err)
		}
	default:
		slog.Warn("push service rejected delivery",
			"subscription_id", sub.ID,
			"status", resp.StatusCode,
		)
	}
}
