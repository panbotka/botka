package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/middleware"
	"botka/internal/models"
	"botka/internal/push"
)

// PushHandler handles HTTP requests for Web Push subscriptions and the
// push test endpoint. The Sender may be nil when VAPID keys are not
// configured, in which case all endpoints return 503.
type PushHandler struct {
	db        *gorm.DB
	sender    push.Sender
	publicKey string
}

// NewPushHandler creates a new PushHandler. Pass a nil sender (and empty
// publicKey) to disable push entirely; the endpoints will return 503.
func NewPushHandler(db *gorm.DB, sender push.Sender, publicKey string) *PushHandler {
	return &PushHandler{db: db, sender: sender, publicKey: publicKey}
}

// RegisterPushRoutes attaches push endpoints to the given router group.
func RegisterPushRoutes(rg *gin.RouterGroup, h *PushHandler) {
	rg.GET("/push/vapid-public-key", h.VAPIDPublicKey)
	rg.GET("/push/subscriptions", h.List)
	rg.POST("/push/subscriptions", h.Create)
	rg.DELETE("/push/subscriptions/:id", h.Delete)
	rg.POST("/push/test", h.Test)
}

// pushDisabled returns true when VAPID is not configured. All push
// endpoints short-circuit with a 503 in that case.
func (h *PushHandler) pushDisabled() bool {
	return h.publicKey == "" || h.sender == nil
}

func (h *PushHandler) requireEnabled(c *gin.Context) bool {
	if h.pushDisabled() {
		respondError(c, http.StatusServiceUnavailable, "push notifications not configured")
		return false
	}
	return true
}

func currentUser(c *gin.Context) (*models.User, bool) {
	val, ok := c.Get(middleware.ContextKeyUser)
	if !ok {
		return nil, false
	}
	user, ok := val.(*models.User)
	return user, ok
}

// VAPIDPublicKey returns the server's VAPID public key, which the browser
// needs to subscribe.
func (h *PushHandler) VAPIDPublicKey(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	respondOK(c, gin.H{"public_key": h.publicKey})
}

type subscriptionKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

type createSubscriptionRequest struct {
	Endpoint  string           `json:"endpoint"`
	Keys      subscriptionKeys `json:"keys"`
	UserAgent string           `json:"user_agent"`
}

// Create registers a new push subscription for the current user. The
// (user_id, endpoint) pair is unique; re-posting the same endpoint returns
// the existing row rather than failing.
func (h *PushHandler) Create(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	user, ok := currentUser(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		respondError(c, http.StatusBadRequest, "endpoint, keys.p256dh and keys.auth are required")
		return
	}
	if msg := firstError(
		validateMaxLength("endpoint", req.Endpoint, maxURLLength),
		validateMaxLength("p256dh", req.Keys.P256dh, maxLabelLength),
		validateMaxLength("auth", req.Keys.Auth, maxLabelLength),
		validateMaxLength("user_agent", req.UserAgent, maxLabelLength),
	); msg != "" {
		respondError(c, http.StatusBadRequest, msg)
		return
	}

	var existing models.PushSubscription
	err := h.db.Where("user_id = ? AND endpoint = ?", user.ID, req.Endpoint).First(&existing).Error
	if err == nil {
		// Update keys/UA in case they rotated.
		existing.P256dh = req.Keys.P256dh
		existing.Auth = req.Keys.Auth
		existing.UserAgent = req.UserAgent
		if err := h.db.Save(&existing).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to update subscription")
			return
		}
		c.JSON(http.StatusCreated, gin.H{"data": existing})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		respondError(c, http.StatusInternalServerError, "failed to query subscription")
		return
	}

	sub := models.PushSubscription{
		UserID:    user.ID,
		Endpoint:  req.Endpoint,
		P256dh:    req.Keys.P256dh,
		Auth:      req.Keys.Auth,
		UserAgent: req.UserAgent,
	}
	if err := h.db.Create(&sub).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create subscription")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": sub})
}

// List returns all push subscriptions registered by the current user.
func (h *PushHandler) List(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	user, ok := currentUser(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var subs []models.PushSubscription
	if err := h.db.Where("user_id = ?", user.ID).Order("id ASC").Find(&subs).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}
	if subs == nil {
		subs = []models.PushSubscription{}
	}
	respondList(c, subs, int64(len(subs)))
}

// Delete removes a push subscription if it belongs to the current user.
// Returns 404 if the subscription doesn't exist or belongs to someone else.
func (h *PushHandler) Delete(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	user, ok := currentUser(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid subscription id")
		return
	}

	res := h.db.Where("id = ? AND user_id = ?", id, user.ID).Delete(&models.PushSubscription{})
	if res.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete subscription")
		return
	}
	if res.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "subscription not found")
		return
	}
	c.Status(http.StatusNoContent)
}

type testRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Test sends a test push notification to every subscription owned by the
// current user. Useful for verifying browser permission and the subscription
// flow during development.
func (h *PushHandler) Test(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	user, ok := currentUser(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req testRequest
	_ = c.ShouldBindJSON(&req) // empty body is fine, defaults below

	payload := push.PushPayload{
		Title: req.Title,
		Body:  req.Body,
		URL:   "/",
	}
	if payload.Title == "" {
		payload.Title = "Test from Botka"
	}
	if payload.Body == "" {
		payload.Body = "Push notifications are working"
	}

	var count int64
	if err := h.db.Model(&models.PushSubscription{}).
		Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to count subscriptions")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	if err := h.sender.Send(ctx, user.ID, payload); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to send test push")
		return
	}
	respondOK(c, gin.H{"sent": count})
}
