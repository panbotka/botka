package mcp

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SSEHandler manages SSE-based MCP sessions over Gin.
type SSEHandler struct {
	server   *Server
	sessions sync.Map // sessionID (string) → *sseSession
}

// sseSession holds the response channel for a single SSE connection.
type sseSession struct {
	messages chan []byte
}

// NewSSEHandler creates an SSE handler backed by the given MCP server.
func NewSSEHandler(server *Server) *SSEHandler {
	return &SSEHandler{server: server}
}

// RegisterRoutes registers the SSE and message endpoints on a Gin router group.
func RegisterRoutes(rg *gin.RouterGroup, handler *SSEHandler) {
	rg.GET("/sse", handler.HandleSSE)
	rg.POST("/message", handler.HandleMessage)
}

// HandleSSE opens an SSE connection, sends an endpoint event with the
// message URL, then streams responses for this session until the client
// disconnects.
func (h *SSEHandler) HandleSSE(c *gin.Context) {
	sessionID := uuid.New().String()
	sess := &sseSession{
		messages: make(chan []byte, 64),
	}
	h.sessions.Store(sessionID, sess)
	defer h.sessions.Delete(sessionID)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// Send the endpoint event so the client knows where to POST messages.
	_, _ = fmt.Fprintf(c.Writer, "event: endpoint\ndata: /mcp/message?sessionId=%s\n\n", sessionID)
	c.Writer.Flush()

	slog.Info("mcp sse session started", "session", sessionID)

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			slog.Info("mcp sse session closed", "session", sessionID)
			return
		case msg, ok := <-sess.messages:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(c.Writer, "event: message\ndata: %s\n\n", msg)
			c.Writer.Flush()
		}
	}
}

// HandleMessage receives a JSON-RPC message via POST, processes it through
// the MCP server, and sends the response back on the SSE connection.
func (h *SSEHandler) HandleMessage(c *gin.Context) {
	sessionID := c.Query("sessionId")
	val, ok := h.sessions.Load(sessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	sess := val.(*sseSession)

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	resp := h.server.HandleMessage(body)
	if resp != nil {
		select {
		case sess.messages <- resp:
		default:
			slog.Warn("mcp sse session buffer full, dropping response", "session", sessionID)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session buffer full"})
			return
		}
	}

	c.Status(http.StatusAccepted)
}
