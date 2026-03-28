package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestHandleMessage_sessionNotFound verifies POST to /message with invalid session returns 404.
func TestHandleMessage_sessionNotFound(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, nil, nil)
	handler := NewSSEHandler(srv, "test-token")

	router := gin.New()
	group := router.Group("/mcp")
	RegisterRoutes(group, handler)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/message?sessionId=invalid", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestNewSSEHandler_notNil verifies NewSSEHandler returns a non-nil handler.
func TestNewSSEHandler_notNil(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, nil, nil)
	handler := NewSSEHandler(srv, "test-token")
	if handler == nil {
		t.Fatal("expected non-nil SSEHandler")
	}
}

// TestRegisterRoutes_endpoints verifies that SSE and message routes are registered.
func TestRegisterRoutes_endpoints(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, nil, nil)
	handler := NewSSEHandler(srv, "test-token")

	router := gin.New()
	group := router.Group("/mcp")
	RegisterRoutes(group, handler)

	routes := router.Routes()

	expectedRoutes := map[string]string{
		"GET":  "/mcp/sse",
		"POST": "/mcp/message",
	}

	for method, path := range expectedRoutes {
		found := false
		for _, route := range routes {
			if route.Method == method && route.Path == path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing route: %s %s", method, path)
		}
	}
}
