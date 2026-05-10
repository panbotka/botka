package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestHandleSearchMessages_invalidJSON verifies bad JSON returns an error.
func TestHandleSearchMessages_invalidJSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleSearchMessages(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid arguments") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleSearchMessages_missingQuery verifies query is required.
func TestHandleSearchMessages_missingQuery(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleSearchMessages(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing query")
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSearchMessagesInToolDefinitions verifies the tool is registered in
// the tools/list response.
func TestSearchMessagesInToolDefinitions(t *testing.T) {
	t.Parallel()
	defs := toolDefinitions()
	for _, d := range defs {
		if d.Name == "search_messages" {
			return
		}
	}
	t.Errorf("search_messages tool not found in toolDefinitions()")
}
