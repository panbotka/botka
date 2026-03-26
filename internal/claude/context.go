package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"botka/internal/models"
)

// ContextConfig holds paths for assembling the system prompt context.
type ContextConfig struct {
	OpenClawWorkspace string // /home/pi/.openclaw/workspace
	ContextDir        string // directory for assembled context files
}

// MemoryFunc retrieves app memories formatted for inclusion in a system prompt.
// Returns concatenated memory content or an error.
type MemoryFunc func(ctx context.Context) (string, error)

// AssembleContext builds a hierarchical system prompt from multiple context layers
// and writes it to a file that Claude Code reads via --append-system-prompt-file.
//
// The layers (in order) provide progressively narrower context:
//  1. SOUL.md     — AI identity/personality (from OpenClaw workspace)
//  2. USER.md     — information about the user
//  3. MEMORY.md   — long-term operational memory
//  4. Daily notes — recent entries (last 3 days) for temporal context
//  5. App memories — user-created memories from the database
//  6. System prompt — thread-specific instructions (from persona or custom)
//  7. Project CLAUDE.md — project-specific coding context
//  8. Conversation history — prior messages so resumed sessions have context
//
// This layering ensures Claude has full context even when starting a new session
// (e.g., after a session reset or server restart).
func AssembleContext(ctx context.Context, cfg ContextConfig, threadID int64, getMemories MemoryFunc, systemPrompt, folderClaudeMD, projectName, projectPath string, sources []SourceInput, messages []models.Message) (string, error) {
	var parts []string

	// Layer 1: SOUL.md (identity)
	if content, err := readFileIfExists(filepath.Join(cfg.OpenClawWorkspace, "SOUL.md")); err == nil && content != "" {
		parts = append(parts, "# Identity\n\n"+content)
	}

	// Layer 2: USER.md (user info)
	if content, err := readFileIfExists(filepath.Join(cfg.OpenClawWorkspace, "USER.md")); err == nil && content != "" {
		parts = append(parts, "# About the User\n\n"+content)
	}

	// Layer 3: MEMORY.md (operational memory)
	if content, err := readFileIfExists(filepath.Join(cfg.OpenClawWorkspace, "MEMORY.md")); err == nil && content != "" {
		parts = append(parts, "# Operational Memory\n\n"+content)
	}

	// Layer 4: Recent daily notes (last 3 days)
	if notes := recentDailyNotes(cfg.OpenClawWorkspace, 3); notes != "" {
		parts = append(parts, "# Recent Notes\n\n"+notes)
	}

	// Layer 5: App memories from database
	if getMemories != nil {
		if memBlock, err := getMemories(ctx); err == nil && memBlock != "" {
			parts = append(parts, "# User Preferences\n\n"+memBlock)
		}
	}

	// Layer 6: Thread system prompt (from persona or custom)
	if systemPrompt != "" {
		parts = append(parts, "# Thread Instructions\n\n"+systemPrompt)
	}

	// Layer 6b: Thread URL sources (fetched fresh)
	if len(sources) > 0 {
		if sourceContent := FetchSources(ctx, sources); sourceContent != "" {
			parts = append(parts, "# Reference Sources\n\n"+sourceContent)
		}
	}

	// Layer 7: Folder/project CLAUDE.md
	if folderClaudeMD != "" {
		parts = append(parts, "# Project Context\n\n"+folderClaudeMD)
	}

	// Layer 7b: Project assignment note
	if projectName != "" {
		note := fmt.Sprintf("# Active Project\n\nThis chat is associated with project %q (%s).\nWhen creating tasks or discussing code, default to this project unless the user specifies otherwise.", projectName, projectPath)
		parts = append(parts, note)
	}

	// Layer 8: Conversation history (so Claude knows what was discussed before a session reset)
	if len(messages) > 0 {
		var historyLines []string
		// Include last 200 messages
		start := 0
		if len(messages) > 200 {
			start = len(messages) - 200
		}
		for _, m := range messages[start:] {
			prefix := "User"
			if m.Role == "assistant" {
				prefix = "Assistant"
			}
			// Truncate very long messages
			content := m.Content
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			historyLines = append(historyLines, fmt.Sprintf("**%s:** %s", prefix, content))
		}
		parts = append(parts, "# Previous Conversation\n\nThis is a continuation of an existing conversation. Here is the recent history:\n\n"+strings.Join(historyLines, "\n\n"))
	}

	assembled := strings.Join(parts, "\n\n---\n\n")

	// Write to file
	if err := os.MkdirAll(cfg.ContextDir, 0755); err != nil {
		return "", fmt.Errorf("create context dir: %w", err)
	}

	filePath := filepath.Join(cfg.ContextDir, fmt.Sprintf("thread-%d.md", threadID))
	if err := os.WriteFile(filePath, []byte(assembled), 0644); err != nil {
		return "", fmt.Errorf("write context file: %w", err)
	}

	// Return absolute path since claude may run in a different working directory
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return filePath, nil
	}
	return absPath, nil
}

func readFileIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// recentDailyNotes reads the N most recent daily memory files from the workspace.
func recentDailyNotes(workspacePath string, n int) string {
	memoryDir := filepath.Join(workspacePath, "memory")
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return ""
	}

	// Filter for date-formatted .md files and sort by name (dates sort naturally)
	var dateFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			// Check if it looks like a date file (YYYY-MM-DD.md)
			name := strings.TrimSuffix(e.Name(), ".md")
			if _, err := time.Parse("2006-01-02", name); err == nil {
				dateFiles = append(dateFiles, e.Name())
			}
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(dateFiles)))

	if len(dateFiles) > n {
		dateFiles = dateFiles[:n]
	}

	var notes []string
	for _, f := range dateFiles {
		content, err := os.ReadFile(filepath.Join(memoryDir, f))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(f, ".md")
		notes = append(notes, fmt.Sprintf("## %s\n\n%s", name, strings.TrimSpace(string(content))))
	}

	return strings.Join(notes, "\n\n")
}
