package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"botka/internal/models"
)

func TestAssembleContext_AllLayers(t *testing.T) {
	// Set up a mock workspace
	workspace := t.TempDir()
	contextDir := t.TempDir()

	// Layer 1: SOUL.md
	os.WriteFile(filepath.Join(workspace, "SOUL.md"), []byte("I am a helpful assistant."), 0644)

	// Layer 2: USER.md
	os.WriteFile(filepath.Join(workspace, "USER.md"), []byte("User is a developer."), 0644)

	// Layer 3: MEMORY.md
	os.WriteFile(filepath.Join(workspace, "MEMORY.md"), []byte("Remember to be concise."), 0644)

	// Layer 4: Daily notes
	memDir := filepath.Join(workspace, "memory")
	os.MkdirAll(memDir, 0755)
	os.WriteFile(filepath.Join(memDir, "2025-01-15.md"), []byte("Worked on tests today."), 0644)

	// Layer 5: App memories via callback
	memFn := func(_ context.Context) (string, error) {
		return "User prefers dark mode.", nil
	}

	// Layer 6: System prompt
	systemPrompt := "You are a coding assistant."

	// Layer 7: Folder CLAUDE.md
	folderMD := "This project uses Go and React."

	// Layer 8: Messages
	messages := []models.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	cfg := ContextConfig{
		OpenClawWorkspace: workspace,
		ContextDir:        contextDir,
	}

	path, err := AssembleContext(context.Background(), cfg, 42, memFn, systemPrompt, folderMD, "myproject", "/home/pi/projects/myproject", messages)
	if err != nil {
		t.Fatalf("AssembleContext error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read assembled file: %v", err)
	}

	assembled := string(content)

	checks := []struct {
		name     string
		contains string
	}{
		{"identity", "I am a helpful assistant."},
		{"user info", "User is a developer."},
		{"memory", "Remember to be concise."},
		{"daily notes", "Worked on tests today."},
		{"app memories", "User prefers dark mode."},
		{"system prompt", "You are a coding assistant."},
		{"folder context", "This project uses Go and React."},
		{"conversation history", "**User:** Hello"},
		{"conversation history", "**Assistant:** Hi there!"},
		{"section header identity", "# Identity"},
		{"section header user", "# About the User"},
		{"section header memory", "# Operational Memory"},
		{"section header notes", "# Recent Notes"},
		{"section header prefs", "# User Preferences"},
		{"section header thread", "# Thread Instructions"},
		{"section header project", "# Project Context"},
		{"section header conversation", "# Previous Conversation"},
		{"active project", `project "myproject"`},
		{"active project path", "/home/pi/projects/myproject"},
		{"section header active project", "# Active Project"},
	}

	for _, c := range checks {
		if !strings.Contains(assembled, c.contains) {
			t.Errorf("assembled context missing %s: expected to contain %q", c.name, c.contains)
		}
	}
}

func TestAssembleContext_EmptyWorkspace(t *testing.T) {
	workspace := t.TempDir()
	contextDir := t.TempDir()

	cfg := ContextConfig{
		OpenClawWorkspace: workspace,
		ContextDir:        contextDir,
	}

	path, err := AssembleContext(context.Background(), cfg, 1, nil, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AssembleContext error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read assembled file: %v", err)
	}

	// With no layers, the file should be empty
	if len(strings.TrimSpace(string(content))) != 0 {
		t.Errorf("expected empty output for empty workspace, got %q", string(content))
	}
}

func TestAssembleContext_MessageTruncation(t *testing.T) {
	workspace := t.TempDir()
	contextDir := t.TempDir()

	longContent := strings.Repeat("x", 600)
	messages := []models.Message{
		{Role: "user", Content: longContent},
	}

	cfg := ContextConfig{
		OpenClawWorkspace: workspace,
		ContextDir:        contextDir,
	}

	path, err := AssembleContext(context.Background(), cfg, 1, nil, "", "", "", "", messages)
	if err != nil {
		t.Fatalf("AssembleContext error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read assembled file: %v", err)
	}

	assembled := string(content)
	if strings.Contains(assembled, longContent) {
		t.Error("expected long message to be truncated")
	}
	if !strings.Contains(assembled, "...") {
		t.Error("expected truncation marker '...'")
	}
}

func TestAssembleContext_MessageLimit(t *testing.T) {
	workspace := t.TempDir()
	contextDir := t.TempDir()

	// Generate 210 messages — only last 200 should appear
	var messages []models.Message
	for i := range 210 {
		messages = append(messages, models.Message{
			Role:    "user",
			Content: strings.Repeat("a", 10) + string(rune('0'+i%10)),
		})
	}

	cfg := ContextConfig{
		OpenClawWorkspace: workspace,
		ContextDir:        contextDir,
	}

	path, err := AssembleContext(context.Background(), cfg, 1, nil, "", "", "", "", messages)
	if err != nil {
		t.Fatalf("AssembleContext error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read assembled file: %v", err)
	}

	// Verify it contains history section
	if !strings.Contains(string(content), "# Previous Conversation") {
		t.Error("expected conversation history section")
	}
}

func TestAssembleContext_MemoryFuncError(t *testing.T) {
	workspace := t.TempDir()
	contextDir := t.TempDir()

	// Memory function that returns an error — should be silently skipped
	memFn := func(_ context.Context) (string, error) {
		return "", os.ErrNotExist
	}

	cfg := ContextConfig{
		OpenClawWorkspace: workspace,
		ContextDir:        contextDir,
	}

	_, err := AssembleContext(context.Background(), cfg, 1, memFn, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AssembleContext should not fail on memory error: %v", err)
	}
}

func TestReadFileIfExists_Missing(t *testing.T) {
	content, err := readFileIfExists("/nonexistent/path/file.md")
	if err != nil {
		t.Errorf("expected nil error for missing file, got %v", err)
	}
	if content != "" {
		t.Errorf("expected empty content, got %q", content)
	}
}

func TestReadFileIfExists_Exists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("  hello world  \n"), 0644)

	content, err := readFileIfExists(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "hello world" {
		t.Errorf("expected trimmed content 'hello world', got %q", content)
	}
}

func TestRecentDailyNotes(t *testing.T) {
	workspace := t.TempDir()
	memDir := filepath.Join(workspace, "memory")
	os.MkdirAll(memDir, 0755)

	// Create 5 daily note files
	dates := []string{"2025-01-10", "2025-01-11", "2025-01-12", "2025-01-13", "2025-01-14"}
	for _, d := range dates {
		os.WriteFile(filepath.Join(memDir, d+".md"), []byte("Notes for "+d), 0644)
	}

	// Also add a non-date file that should be ignored
	os.WriteFile(filepath.Join(memDir, "README.md"), []byte("not a note"), 0644)

	notes := recentDailyNotes(workspace, 3)

	// Should contain the 3 most recent
	if !strings.Contains(notes, "2025-01-14") {
		t.Error("expected most recent note 2025-01-14")
	}
	if !strings.Contains(notes, "2025-01-13") {
		t.Error("expected note 2025-01-13")
	}
	if !strings.Contains(notes, "2025-01-12") {
		t.Error("expected note 2025-01-12")
	}
	// Should NOT contain older ones
	if strings.Contains(notes, "2025-01-10") {
		t.Error("should not contain old note 2025-01-10")
	}
}

func TestRecentDailyNotes_EmptyDir(t *testing.T) {
	workspace := t.TempDir()
	notes := recentDailyNotes(workspace, 3)
	if notes != "" {
		t.Errorf("expected empty notes, got %q", notes)
	}
}
