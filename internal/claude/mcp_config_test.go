package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"botka/internal/models"
)

func TestGenerateMCPConfig_empty(t *testing.T) {
	t.Parallel()
	path, err := GenerateMCPConfig(nil, t.TempDir(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path for empty servers, got %q", path)
	}
}

func TestGenerateMCPConfig_stdio(t *testing.T) {
	t.Parallel()
	servers := []models.MCPServer{
		{
			ID:         1,
			Name:       "my-tool",
			ServerType: models.MCPServerTypeStdio,
			Config:     json.RawMessage(`{"command":"npx","args":["-y","my-mcp"],"env":{"KEY":"val"}}`),
		},
	}

	dir := t.TempDir()
	path, err := GenerateMCPConfig(servers, dir, "thread-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal generated config: %v", err)
	}

	entry, ok := cfg.MCPServers["my-tool"]
	if !ok {
		t.Fatal("missing my-tool entry")
	}
	if entry.Command != "npx" {
		t.Errorf("command = %q, want %q", entry.Command, "npx")
	}
	if len(entry.Args) != 2 || entry.Args[0] != "-y" || entry.Args[1] != "my-mcp" {
		t.Errorf("args = %v, want [-y my-mcp]", entry.Args)
	}
	if entry.Env["KEY"] != "val" {
		t.Errorf("env KEY = %q, want %q", entry.Env["KEY"], "val")
	}
	if entry.Type != "" {
		t.Errorf("type should be empty for stdio, got %q", entry.Type)
	}
}

func TestGenerateMCPConfig_sse(t *testing.T) {
	t.Parallel()
	servers := []models.MCPServer{
		{
			ID:         2,
			Name:       "remote-server",
			ServerType: models.MCPServerTypeSSE,
			Config:     json.RawMessage(`{"url":"https://example.com/mcp","headers":{"Authorization":"Bearer tok"}}`),
		},
	}

	dir := t.TempDir()
	path, err := GenerateMCPConfig(servers, dir, "task-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	entry, ok := cfg.MCPServers["remote-server"]
	if !ok {
		t.Fatal("missing remote-server entry")
	}
	if entry.Type != "sse" {
		t.Errorf("type = %q, want %q", entry.Type, "sse")
	}
	if entry.URL != "https://example.com/mcp" {
		t.Errorf("url = %q, want https://example.com/mcp", entry.URL)
	}
	if entry.Headers["Authorization"] != "Bearer tok" {
		t.Errorf("Authorization header = %q, want %q", entry.Headers["Authorization"], "Bearer tok")
	}
	if entry.Command != "" {
		t.Errorf("command should be empty for sse, got %q", entry.Command)
	}
}

func TestGenerateMCPConfig_mixed(t *testing.T) {
	t.Parallel()
	servers := []models.MCPServer{
		{
			ID:         1,
			Name:       "local",
			ServerType: models.MCPServerTypeStdio,
			Config:     json.RawMessage(`{"command":"node","args":["server.js"],"env":{}}`),
		},
		{
			ID:         2,
			Name:       "remote",
			ServerType: models.MCPServerTypeSSE,
			Config:     json.RawMessage(`{"url":"https://mcp.example.com","headers":{}}`),
		},
	}

	dir := t.TempDir()
	path, err := GenerateMCPConfig(servers, dir, "mixed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(cfg.MCPServers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(cfg.MCPServers))
	}
	if _, ok := cfg.MCPServers["local"]; !ok {
		t.Error("missing local entry")
	}
	if _, ok := cfg.MCPServers["remote"]; !ok {
		t.Error("missing remote entry")
	}
}

func TestGenerateMCPConfig_filename(t *testing.T) {
	t.Parallel()
	servers := []models.MCPServer{
		{
			ID:         1,
			Name:       "s",
			ServerType: models.MCPServerTypeStdio,
			Config:     json.RawMessage(`{"command":"echo"}`),
		},
	}

	dir := t.TempDir()
	path, err := GenerateMCPConfig(servers, dir, "thread-99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	base := filepath.Base(path)
	if base != "mcp-thread-99.json" {
		t.Errorf("filename = %q, want %q", base, "mcp-thread-99.json")
	}
}

func TestMCPServerHash_empty(t *testing.T) {
	t.Parallel()
	h := MCPServerHash(nil)
	if h != "" {
		t.Errorf("expected empty hash for nil servers, got %q", h)
	}
}

func TestMCPServerHash_deterministic(t *testing.T) {
	t.Parallel()
	servers := []models.MCPServer{
		{ID: 3}, {ID: 1}, {ID: 2},
	}
	h1 := MCPServerHash(servers)
	// Reversed input order should produce the same hash (sorted by ID).
	servers2 := []models.MCPServer{
		{ID: 1}, {ID: 2}, {ID: 3},
	}
	h2 := MCPServerHash(servers2)
	if h1 != h2 {
		t.Errorf("hash should be order-independent: %q != %q", h1, h2)
	}
}

func TestMCPServerHash_differs(t *testing.T) {
	t.Parallel()
	s1 := []models.MCPServer{{ID: 1}}
	s2 := []models.MCPServer{{ID: 2}}
	if MCPServerHash(s1) == MCPServerHash(s2) {
		t.Error("different server sets should produce different hashes")
	}
}

func TestCleanupMCPConfigs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create some MCP config files and a non-MCP file.
	os.WriteFile(filepath.Join(dir, "mcp-thread-1.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "mcp-task-abc.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "thread-5.md"), []byte("keep"), 0644)

	CleanupMCPConfigs(dir)

	// MCP files should be gone.
	if _, err := os.Stat(filepath.Join(dir, "mcp-thread-1.json")); !os.IsNotExist(err) {
		t.Error("mcp-thread-1.json should have been removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "mcp-task-abc.json")); !os.IsNotExist(err) {
		t.Error("mcp-task-abc.json should have been removed")
	}
	// Non-MCP file should remain.
	if _, err := os.Stat(filepath.Join(dir, "thread-5.md")); err != nil {
		t.Error("thread-5.md should still exist")
	}
}

func TestRemoveMCPConfig_emptyPath(t *testing.T) {
	t.Parallel()
	RemoveMCPConfig("")
}

func TestRemoveMCPConfig_removes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp-test.json")
	os.WriteFile(path, []byte("{}"), 0644)

	RemoveMCPConfig(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestBuildMCPEntry_invalidType(t *testing.T) {
	t.Parallel()
	s := models.MCPServer{
		Name:       "bad",
		ServerType: "grpc",
		Config:     json.RawMessage(`{}`),
	}
	_, err := buildMCPEntry(s)
	if err == nil {
		t.Error("expected error for unknown server type")
	}
}
