package claude

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"botka/internal/models"
)

// mcpConfigEntry represents a single MCP server in Claude Code's .mcp.json format.
type mcpConfigEntry struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// mcpConfigFile represents the top-level .mcp.json structure expected by Claude Code.
type mcpConfigFile struct {
	MCPServers map[string]mcpConfigEntry `json:"mcpServers"`
}

// stdioConfig holds the JSON fields for a stdio-type MCP server.
type stdioConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// sseConfig holds the JSON fields for an SSE-type MCP server.
type sseConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// GenerateMCPConfig builds a Claude Code .mcp.json config file from the given
// MCP servers and writes it to a temp file in contextDir. Returns the absolute
// path to the generated file, or empty string if no servers are provided.
// The identifier is used in the filename (e.g. thread ID or task ID).
func GenerateMCPConfig(servers []models.MCPServer, contextDir, identifier string) (string, error) {
	if len(servers) == 0 {
		return "", nil
	}

	cfg := mcpConfigFile{
		MCPServers: make(map[string]mcpConfigEntry, len(servers)),
	}

	for _, s := range servers {
		entry, err := buildMCPEntry(s)
		if err != nil {
			return "", fmt.Errorf("build entry for %q: %w", s.Name, err)
		}
		cfg.MCPServers[s.Name] = entry
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}

	if err := os.MkdirAll(contextDir, 0755); err != nil {
		return "", fmt.Errorf("create context dir: %w", err)
	}

	filePath := filepath.Join(contextDir, fmt.Sprintf("mcp-%s.json", identifier))
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("write mcp config: %w", err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return filePath, nil
	}
	return absPath, nil
}

// buildMCPEntry converts an MCPServer model into a .mcp.json entry.
func buildMCPEntry(s models.MCPServer) (mcpConfigEntry, error) {
	switch s.ServerType {
	case models.MCPServerTypeStdio:
		var sc stdioConfig
		if err := json.Unmarshal(s.Config, &sc); err != nil {
			return mcpConfigEntry{}, fmt.Errorf("unmarshal stdio config: %w", err)
		}
		return mcpConfigEntry{
			Command: sc.Command,
			Args:    sc.Args,
			Env:     sc.Env,
		}, nil

	case models.MCPServerTypeSSE:
		var sc sseConfig
		if err := json.Unmarshal(s.Config, &sc); err != nil {
			return mcpConfigEntry{}, fmt.Errorf("unmarshal sse config: %w", err)
		}
		return mcpConfigEntry{
			Type:    "sse",
			URL:     sc.URL,
			Headers: sc.Headers,
		}, nil

	default:
		return mcpConfigEntry{}, fmt.Errorf("unknown server type %q", s.ServerType)
	}
}

// MCPServerHash returns a deterministic hash of the given MCP server list.
// Used as a cache key to detect when the set of MCP servers has changed
// and a session needs to be invalidated.
func MCPServerHash(servers []models.MCPServer) string {
	if len(servers) == 0 {
		return ""
	}
	ids := make([]int64, len(servers))
	for i, s := range servers {
		ids[i] = s.ID
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	data, _ := json.Marshal(ids)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}

// CleanupMCPConfigs removes all mcp-*.json files from the context directory.
// Called on startup to clean stale files from previous runs.
func CleanupMCPConfigs(contextDir string) {
	pattern := filepath.Join(contextDir, "mcp-*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, f := range matches {
		_ = os.Remove(f)
	}
}

// RemoveMCPConfig removes a specific MCP config file. Safe to call with empty path.
func RemoveMCPConfig(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}
