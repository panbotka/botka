package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"botka/internal/handlers"
)

// TestHandleRunCommand_noTracker verifies run_command fails without command tracker.
func TestHandleRunCommand_noTracker(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleRunCommand(json.RawMessage(`{"project_name":"test","command":"dev"}`))
	if err == nil {
		t.Fatal("expected error when commands is nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleRunCommand_missingProjectName verifies project_name is required.
func TestHandleRunCommand_missingProjectName(t *testing.T) {
	t.Parallel()
	tracker := handlers.NewCommandTracker()
	srv := NewServer(nil, nil, tracker)

	_, err := srv.handleRunCommand(json.RawMessage(`{"command":"dev"}`))
	if err == nil {
		t.Fatal("expected error for missing project_name")
	}
	if !strings.Contains(err.Error(), "project_name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleRunCommand_invalidCommand verifies command validation.
func TestHandleRunCommand_invalidCommand(t *testing.T) {
	t.Parallel()
	tracker := handlers.NewCommandTracker()
	srv := NewServer(nil, nil, tracker)

	_, err := srv.handleRunCommand(json.RawMessage(`{"project_name":"test","command":"invalid"}`))
	if err == nil {
		t.Fatal("expected error for invalid command")
	}
	if !strings.Contains(err.Error(), "\"dev\" or \"deploy\"") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleRunCommand_invalidJSON verifies bad JSON returns an error.
func TestHandleRunCommand_invalidJSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleRunCommand(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestHandleListCommands_noTracker verifies list_commands fails without command tracker.
func TestHandleListCommands_noTracker(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleListCommands(json.RawMessage(`{"project_name":"test"}`))
	if err == nil {
		t.Fatal("expected error when commands is nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleListCommands_missingProjectName verifies project_name is required.
func TestHandleListCommands_missingProjectName(t *testing.T) {
	t.Parallel()
	tracker := handlers.NewCommandTracker()
	srv := NewServer(nil, nil, tracker)

	_, err := srv.handleListCommands(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing project_name")
	}
	if !strings.Contains(err.Error(), "project_name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleKillCommand_noTracker verifies kill_command fails without command tracker.
func TestHandleKillCommand_noTracker(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleKillCommand(json.RawMessage(`{"project_name":"test","pid":123}`))
	if err == nil {
		t.Fatal("expected error when commands is nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleKillCommand_missingPID verifies pid is required.
func TestHandleKillCommand_missingPID(t *testing.T) {
	t.Parallel()
	tracker := handlers.NewCommandTracker()
	srv := NewServer(nil, nil, tracker)

	_, err := srv.handleKillCommand(json.RawMessage(`{"project_name":"test"}`))
	if err == nil {
		t.Fatal("expected error for missing pid")
	}
	if !strings.Contains(err.Error(), "pid is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleKillCommand_invalidJSON verifies bad JSON returns an error.
func TestHandleKillCommand_invalidJSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleKillCommand(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
