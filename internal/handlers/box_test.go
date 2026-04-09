package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	h := NewBoxHandler(nil, "10.0.0.1", "testuser", "/usr/bin/wol")
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

func TestParseBoxProjectNames(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name: "directories only",
			input: `botka/
saiduler/
README.md
notes.txt
`,
			expect: []string{"botka", "saiduler"},
		},
		{
			name: "sorted output",
			input: `zeta/
alpha/
middle/
`,
			expect: []string{"alpha", "middle", "zeta"},
		},
		{
			name:   "empty",
			input:  "",
			expect: nil,
		},
		{
			name: "hidden directories skipped",
			input: `.git/
.cache/
visible/
`,
			expect: []string{"visible"},
		},
		{
			name: "blank lines tolerated",
			input: `

foo/

bar/
`,
			expect: []string{"bar", "foo"},
		},
		{
			name: "non-directory entries skipped",
			input: `file.txt
dir/
another_file
`,
			expect: []string{"dir"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBoxProjectNames(tc.input)
			if len(got) != len(tc.expect) {
				t.Fatalf("expected %v, got %v", tc.expect, got)
			}
			for i, v := range got {
				if v != tc.expect[i] {
					t.Errorf("index %d: expected %q, got %q", i, tc.expect[i], v)
				}
			}
		})
	}
}

func TestBoxHandler_ListProjects_Offline(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewBoxHandler(db, "10.0.0.1", "testuser", "/usr/bin/wol")
	// runner intentionally not set — host is unreachable so we return before SSH.

	r := boxRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/box/projects", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Data   []boxProjectEntry `json:"data"`
			Online bool              `json:"online"`
			Note   string            `json:"note"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Online {
		t.Error("expected online=false for unreachable host")
	}
	if resp.Data.Note == "" {
		t.Error("expected note when box is offline")
	}
	if len(resp.Data.Data) != 0 {
		t.Errorf("expected empty data, got %v", resp.Data.Data)
	}
}

func TestBoxHandler_UpsertBoxProjects_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewBoxHandler(db, "10.0.0.1", "testuser", "/usr/bin/wol")

	names := []string{"appone", "apptwo"}
	entries, err := h.upsertBoxProjects(names)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.ID == "" {
			t.Errorf("expected non-empty id, got %+v", e)
		}
		if !strings.HasPrefix(e.Path, "box:") {
			t.Errorf("expected box: prefix, got %q", e.Path)
		}
	}

	// Second call must return the same IDs (reactivation path).
	entries2, err := h.upsertBoxProjects(names)
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if len(entries2) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries2))
	}
	for i := range entries {
		if entries[i].ID != entries2[i].ID {
			t.Errorf("entry %d id changed: %q vs %q", i, entries[i].ID, entries2[i].ID)
		}
	}
}

func TestBoxHandler_ListProjects_NoDatabase(t *testing.T) {
	// Without a database, the endpoint must fail cleanly instead of panicking.
	h := NewBoxHandler(nil, "10.0.0.1", "testuser", "/usr/bin/wol")
	r := boxRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/box/projects", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestBoxHandler_ListProjects_Cached(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	h := NewBoxHandler(db, "10.0.0.1", "testuser", "/usr/bin/wol")
	// Seed cache directly so we don't rely on SSH.
	h.storeCachedProjects([]boxProjectEntry{{ID: "abc", Name: "cached-app", Path: "box:/home/box/projects/cached-app"}})

	r := boxRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/box/projects", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Data   []boxProjectEntry `json:"data"`
			Online bool              `json:"online"`
			Cached bool              `json:"cached"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Data.Cached {
		t.Error("expected cached=true")
	}
	if len(resp.Data.Data) != 1 || resp.Data.Data[0].Name != "cached-app" {
		t.Errorf("unexpected data: %+v", resp.Data.Data)
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
