package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// MessageHandler handles individual message operations.
type MessageHandler struct {
	db *gorm.DB
}

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(db *gorm.DB) *MessageHandler {
	return &MessageHandler{db: db}
}

// RegisterMessageRoutes attaches message endpoints to the given router group.
func RegisterMessageRoutes(rg *gin.RouterGroup, h *MessageHandler) {
	rg.PATCH("/messages/:id/hide", h.ToggleHide)
}

// ToggleHide toggles the hidden field on a message.
func (h *MessageHandler) ToggleHide(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid message id")
		return
	}

	var msg models.Message
	if err := h.db.First(&msg, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "message not found")
		return
	}

	msg.Hidden = !msg.Hidden
	if err := h.db.Save(&msg).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update message")
		return
	}

	respondOK(c, msg)
}
