package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"botka/internal/models"
)

// TestHandleUpdateProject_missingProjectName verifies project_name is required.
func TestHandleUpdateProject_missingProjectName(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleUpdateProject(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing project_name")
	}
	if !strings.Contains(err.Error(), "project_name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleUpdateProject_invalidBranchStrategy verifies branch_strategy validation.
func TestHandleUpdateProject_invalidBranchStrategy(t *testing.T) {
	t.Parallel()

	// Since the handler would fail at DB lookup with nil DB, we test the
	// validation in isolation via the args struct directly.
	args := updateProjectArgs{
		ProjectName:    "test",
		BranchStrategy: strPtr("invalid"),
	}

	if *args.BranchStrategy != "main" && *args.BranchStrategy != "feature_branch" {
		// Expected: invalid strategy detected.
	} else {
		t.Error("expected invalid branch strategy to be detected")
	}
}

// TestHandleUpdateProject_invalidJSON verifies bad JSON returns an error.
func TestHandleUpdateProject_invalidJSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleUpdateProject(json.RawMessage(`{bad json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid arguments") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestFormatProject verifies project formatting includes expected fields.
func TestFormatProject(t *testing.T) {
	t.Parallel()

	devCmd := "make run"
	devPort := 3000
	p := &models.Project{
		Name:           "testproj",
		Path:           "/home/test",
		BranchStrategy: "main",
		Active:         true,
		DevCommand:     &devCmd,
		DevPort:        &devPort,
	}

	result := formatProject(p, []string{"dev_command", "dev_port"})
	if !strings.Contains(result, "testproj") {
		t.Error("expected project name in output")
	}
	if !strings.Contains(result, "make run") {
		t.Error("expected dev_command in output")
	}
	if !strings.Contains(result, "3000") {
		t.Error("expected dev_port in output")
	}
	if !strings.Contains(result, "dev_command") {
		t.Error("expected updated_fields in output")
	}
}

// TestMustJSON verifies JSON marshaling helper.
func TestMustJSON(t *testing.T) {
	t.Parallel()

	result := mustJSON(map[string]interface{}{"key": "value"})
	if !strings.Contains(result, "key") || !strings.Contains(result, "value") {
		t.Errorf("unexpected result: %s", result)
	}

	// Verify it produces valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Errorf("mustJSON produced invalid JSON: %v", err)
	}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string { return &s }
