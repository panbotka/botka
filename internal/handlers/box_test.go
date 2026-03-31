package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// mockCommandRunner records calls and returns canned results.
type mockCommandRunner struct {
	lastCmd  string
	lastArgs []string
	output   []byte
	err      error
}

func (m *mockCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	m.lastCmd = name
	m.lastArgs = args
	return m.output, m.err
}

func boxRouter(h *BoxHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	RegisterBoxRoutes(v1, h)
	return r
}

func newTestBoxHandler(runner *mockCommandRunner) *BoxHandler {
	h := NewBoxHandler("10.0.0.1", "testuser", "/usr/bin/wol")
	h.runner = runner
	return h
}

func TestBoxHandler_Wake_Success(t *testing.T) {
	runner := &mockCommandRunner{output: []byte("ok"), err: nil}
	h := newTestBoxHandler(runner)
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/box/wake", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if runner.lastCmd != "/usr/bin/wol" {
		t.Errorf("expected wol command, got %q", runner.lastCmd)
	}
}

func TestBoxHandler_Wake_Failure(t *testing.T) {
	runner := &mockCommandRunner{output: []byte("error"), err: fmt.Errorf("exit 1")}
	h := newTestBoxHandler(runner)
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/box/wake", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestBoxHandler_StartService_Allowed(t *testing.T) {
	runner := &mockCommandRunner{output: []byte("ok"), err: nil}
	h := newTestBoxHandler(runner)
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/box/services/image-embeddings/start", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify SSH command contains the service name.
	found := false
	for _, arg := range runner.lastArgs {
		if arg == "image-embeddings" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected service name in args, got %v", runner.lastArgs)
	}
}

func TestBoxHandler_StartService_NotAllowed(t *testing.T) {
	runner := &mockCommandRunner{}
	h := newTestBoxHandler(runner)
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/box/services/malicious-service/start", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] == "" {
		t.Error("expected error message")
	}
}

func TestBoxHandler_StopService_Allowed(t *testing.T) {
	runner := &mockCommandRunner{output: []byte("ok"), err: nil}
	h := newTestBoxHandler(runner)
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/box/services/photo-enhancer/stop", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBoxHandler_StopService_NotAllowed(t *testing.T) {
	runner := &mockCommandRunner{}
	h := newTestBoxHandler(runner)
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/box/services/malicious-svc/stop", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for injection attempt, got %d", w.Code)
	}
}

func TestBoxHandler_StopService_LlamaCpp_NotAllowed(t *testing.T) {
	runner := &mockCommandRunner{}
	h := newTestBoxHandler(runner)
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/box/services/llama.cpp/stop", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for manual service, got %d", w.Code)
	}
}

func TestBoxHandler_Shutdown_Success(t *testing.T) {
	runner := &mockCommandRunner{output: []byte(""), err: nil}
	h := newTestBoxHandler(runner)
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/box/shutdown", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if runner.lastCmd != "ssh" {
		t.Errorf("expected ssh command, got %q", runner.lastCmd)
	}
}

func TestBoxHandler_Status_ResponseShape(t *testing.T) {
	runner := &mockCommandRunner{}
	h := newTestBoxHandler(runner)
	// The status endpoint doesn't use the command runner, it does TCP checks.
	// With a non-routable host, it will report offline.
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/box/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Online   bool               `json:"online"`
			Host     string             `json:"host"`
			Services []boxServiceStatus `json:"services"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Data.Host != "10.0.0.1" {
		t.Errorf("expected host 10.0.0.1, got %s", resp.Data.Host)
	}
	if len(resp.Data.Services) != 3 {
		t.Errorf("expected 3 services, got %d", len(resp.Data.Services))
	}
	// Host is unreachable in tests, so all should be stopped.
	for _, svc := range resp.Data.Services {
		if svc.Status != "stopped" {
			t.Errorf("expected service %s to be stopped, got %s", svc.Name, svc.Status)
		}
	}
}

func TestAllowedServices_Whitelist(t *testing.T) {
	expected := map[string]bool{
		"image-embeddings": true,
		"photo-enhancer":   true,
	}
	if len(allowedServices) != len(expected) {
		t.Fatalf("expected %d allowed services, got %d", len(expected), len(allowedServices))
	}
	for name := range expected {
		if !allowedServices[name] {
			t.Errorf("expected %q to be allowed", name)
		}
	}
	// Verify dangerous names are not in the whitelist.
	dangerous := []string{"ssh", "bash", "rm", "llama.cpp", "../etc/passwd"}
	for _, name := range dangerous {
		if allowedServices[name] {
			t.Errorf("%q should not be in allowed services", name)
		}
	}
}
