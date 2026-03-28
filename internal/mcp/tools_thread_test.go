package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestHandleListThreads_invalidJSON verifies bad JSON returns an error.
func TestHandleListThreads_invalidJSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleListThreads(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid arguments") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleListThreadSources_missingThreadID verifies thread_id is required.
func TestHandleListThreadSources_missingThreadID(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleListThreadSources(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing thread_id")
	}
	if !strings.Contains(err.Error(), "thread_id is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleListThreadSources_invalidJSON verifies bad JSON returns an error.
func TestHandleListThreadSources_invalidJSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleListThreadSources(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestHandleAddThreadSource_missingURL verifies url is required.
func TestHandleAddThreadSource_missingURL(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleAddThreadSource(json.RawMessage(`{"thread_id":1}`))
	if err == nil {
		t.Fatal("expected error for missing url")
	}
	if !strings.Contains(err.Error(), "url is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleAddThreadSource_missingThreadID verifies thread_id is required.
func TestHandleAddThreadSource_missingThreadID(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleAddThreadSource(json.RawMessage(`{"url":"https://example.com"}`))
	if err == nil {
		t.Fatal("expected error for missing thread_id")
	}
	if !strings.Contains(err.Error(), "thread_id is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleAddThreadSource_invalidJSON verifies bad JSON returns an error.
func TestHandleAddThreadSource_invalidJSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleAddThreadSource(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestHandleRemoveThreadSource_missingFields verifies required fields.
func TestHandleRemoveThreadSource_missingFields(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"missing thread_id", `{"source_id":1}`, "thread_id is required"},
		{"missing source_id", `{"thread_id":1}`, "source_id is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := srv.handleRemoveThreadSource(json.RawMessage(tt.input))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestHandleUpdateThreadSource_missingFields verifies required fields.
func TestHandleUpdateThreadSource_missingFields(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"missing thread_id", `{"source_id":1}`, "thread_id is required"},
		{"missing source_id", `{"thread_id":1}`, "source_id is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := srv.handleUpdateThreadSource(json.RawMessage(tt.input))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestHandleUpdateThreadSource_invalidJSON verifies bad JSON returns an error.
func TestHandleUpdateThreadSource_invalidJSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleUpdateThreadSource(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestHandleRemoveThreadSource_invalidJSON verifies bad JSON returns an error.
func TestHandleRemoveThreadSource_invalidJSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleRemoveThreadSource(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestToolDefinitions_newToolsPresent verifies all new tools are registered.
func TestToolDefinitions_newToolsPresent(t *testing.T) {
	t.Parallel()
	defs := toolDefinitions()

	expected := []string{
		"update_project",
		"run_command",
		"list_commands",
		"kill_command",
		"list_threads",
		"list_thread_sources",
		"add_thread_source",
		"remove_thread_source",
		"update_thread_source",
	}

	names := make(map[string]bool)
	for _, def := range defs {
		names[def.Name] = true
	}

	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool definition: %s", name)
		}
	}
}

// TestToolHandlers_newToolsRegistered verifies all new tools have handlers.
func TestToolHandlers_newToolsRegistered(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)
	handlers := srv.toolHandlers()

	expected := []string{
		"update_project",
		"run_command",
		"list_commands",
		"kill_command",
		"list_threads",
		"list_thread_sources",
		"add_thread_source",
		"remove_thread_source",
		"update_thread_source",
	}

	for _, name := range expected {
		if _, ok := handlers[name]; !ok {
			t.Errorf("missing handler for tool: %s", name)
		}
	}
}
