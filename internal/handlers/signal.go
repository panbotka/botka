package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/signal"
)

// SignalGroupLister abstracts the subset of signal.Client used by the handler
// so tests can substitute a mock implementation without talking to signal-cli.
type SignalGroupLister interface {
	// ListGroups returns every Signal group the account is a member of.
	ListGroups(ctx context.Context) ([]signal.SignalGroup, error)
}

// SignalHandler handles HTTP requests for Signal bridges and group discovery.
type SignalHandler struct {
	db     *gorm.DB
	client SignalGroupLister
}

// NewSignalHandler creates a new SignalHandler backed by the given database
// and signal client. client may be nil, in which case ListGroups will return
// a 503.
func NewSignalHandler(db *gorm.DB, client SignalGroupLister) *SignalHandler {
	return &SignalHandler{db: db, client: client}
}

// RegisterSignalRoutes attaches Signal endpoints to the given router group.
// Discovery lives under /signal and per-thread bridge CRUD lives under
// /threads/:id/signal-bridge.
func RegisterSignalRoutes(rg *gin.RouterGroup, h *SignalHandler) {
	rg.GET("/signal/groups", h.ListGroups)
	rg.GET("/threads/:id/signal-bridge", h.GetBridge)
	rg.PUT("/threads/:id/signal-bridge", h.PutBridge)
	rg.DELETE("/threads/:id/signal-bridge", h.DeleteBridge)
}

// signalGroupResponse is the JSON shape returned for each discovered group.
type signalGroupResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
}

// ListGroups returns the Signal groups the account is a member of. It
// responds with 503 Service Unavailable if no client is configured or the
// signal-cli daemon is unreachable.
func (h *SignalHandler) ListGroups(c *gin.Context) {
	if h.client == nil {
		respondError(c, http.StatusServiceUnavailable, "signal client not configured")
		return
	}

	groups, err := h.client.ListGroups(c.Request.Context())
	if err != nil {
		if errors.Is(err, signal.ErrDaemonUnreachable) {
			respondError(c, http.StatusServiceUnavailable, "signal-cli daemon unreachable")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to list signal groups")
		return
	}

	out := make([]signalGroupResponse, 0, len(groups))
	for _, g := range groups {
		out = append(out, signalGroupResponse{
			ID:          g.ID,
			Name:        g.Name,
			MemberCount: g.MemberCount(),
		})
	}
	respondOK(c, out)
}

// GetBridge returns the Signal bridge record for the given thread, or 404 if
// no bridge has been configured.
func (h *SignalHandler) GetBridge(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var bridge models.SignalBridge
	if err := h.db.Where("thread_id = ?", threadID).First(&bridge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "signal bridge not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to load signal bridge")
		return
	}
	respondOK(c, bridge)
}

// putBridgeRequest is the JSON body accepted by PutBridge.
type putBridgeRequest struct {
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`
	Active    *bool  `json:"active,omitempty"`
}

// PutBridge creates or updates the Signal bridge for the given thread. The
// group_id field is required; group_name and active are optional. Responds
// with 200 OK and the persisted record on success.
func (h *SignalHandler) PutBridge(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req putBridgeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.GroupID == "" {
		respondError(c, http.StatusBadRequest, "group_id is required")
		return
	}

	// Verify the thread exists so we return a clean 404 rather than a
	// foreign key violation.
	var thread models.Thread
	if err := h.db.Select("id").Where("id = ?", threadID).First(&thread).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "thread not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to load thread")
		return
	}

	var bridge models.SignalBridge
	err = h.db.Where("thread_id = ?", threadID).First(&bridge).Error
	switch {
	case err == nil:
		bridge.GroupID = req.GroupID
		bridge.GroupName = req.GroupName
		if req.Active != nil {
			bridge.Active = *req.Active
		}
		if err := h.db.Save(&bridge).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to update signal bridge")
			return
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		bridge = models.SignalBridge{
			ThreadID:  threadID,
			GroupID:   req.GroupID,
			GroupName: req.GroupName,
			Active:    true,
		}
		if req.Active != nil {
			bridge.Active = *req.Active
		}
		if err := h.db.Create(&bridge).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to create signal bridge")
			return
		}
	default:
		respondError(c, http.StatusInternalServerError, "failed to load signal bridge")
		return
	}

	respondOK(c, bridge)
}

// DeleteBridge removes the Signal bridge for the given thread. Returns
// 204 No Content on success, 404 if no bridge exists.
func (h *SignalHandler) DeleteBridge(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	result := h.db.Where("thread_id = ?", threadID).Delete(&models.SignalBridge{})
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete signal bridge")
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "signal bridge not found")
		return
	}

	c.Status(http.StatusNoContent)
}
