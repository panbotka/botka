package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/claude"
	"botka/internal/models"
)

const (
	// failureSummaryTimeout caps how long the haiku call may run before being
	// killed. The summary is best-effort, so a short timeout is acceptable.
	failureSummaryTimeout = 90 * time.Second
	// failureSummaryTailLines is the number of trailing log lines fed to the
	// summarizer alongside the spec.
	failureSummaryTailLines = 200
	// failureSummaryMaxOutput caps the total size of the output excerpt sent
	// to the model so a runaway log can't blow up the prompt.
	failureSummaryMaxOutput = 32 * 1024
	// failureSummarySpecCap caps how much of the spec is included in the prompt.
	failureSummarySpecCap = 8 * 1024
)

// generateFailureSummary runs the configured Claude model in non-interactive
// mode to produce a short Czech root-cause summary for a failed task.
// On any error (timeout, non-zero exit, parse error, empty result) it returns
// an error and leaves the database column untouched. The function is safe to
// call concurrently for different tasks.
func generateFailureSummary(
	ctx context.Context, claudePath, model, spec, output string,
) (string, error) {
	if model == "" {
		model = "haiku"
	}

	prompt := buildFailureSummaryPrompt(spec, output)

	ctx, cancel := context.WithTimeout(ctx, failureSummaryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, claudePath, //nolint:gosec // args are controlled
		"-p", prompt,
		"--output-format", "json",
		"--model", model,
	)
	cmd.Env = claude.SanitizedEnv()

	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		var exitErr *exec.ExitError
		if e, ok := err.(*exec.ExitError); ok { //nolint:errorlint // direct check is sufficient here
			stderr = string(e.Stderr)
			exitErr = e
		}
		if exitErr != nil {
			return "", fmt.Errorf("claude exited with %d: %s", exitErr.ExitCode(), truncate(stderr, maxErrLen))
		}
		return "", fmt.Errorf("claude failed: %w", err)
	}

	var parsed struct {
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return "", fmt.Errorf("parse claude json: %w", err)
	}
	if parsed.IsError {
		return "", fmt.Errorf("claude returned error result")
	}
	summary := strings.TrimSpace(parsed.Result)
	if summary == "" {
		return "", fmt.Errorf("claude returned empty result")
	}
	return summary, nil
}

// buildFailureSummaryPrompt assembles the prompt sent to the summarizer.
// The structure deliberately separates the spec (what should have happened)
// from the log tail (what actually happened) and instructs the model to
// answer in Czech with 2-4 sentences.
func buildFailureSummaryPrompt(spec, output string) string {
	tail := tailLines(output, failureSummaryTailLines)
	if len(tail) > failureSummaryMaxOutput {
		tail = tail[len(tail)-failureSummaryMaxOutput:]
	}
	specExcerpt := spec
	if len(specExcerpt) > failureSummarySpecCap {
		specExcerpt = specExcerpt[:failureSummarySpecCap] + "\n[…spec truncated…]"
	}
	var sb strings.Builder
	sb.WriteString("Úloha (task) selhala. Tvým úkolem je vytvořit stručné shrnutí příčiny ")
	sb.WriteString("chyby ve 2 až 4 větách v češtině. Odpověz pouze samotným shrnutím, ")
	sb.WriteString("bez uvozujících frází jako „Úloha selhala“ či „Shrnutí:“. Soustřeď se ")
	sb.WriteString("na hlavní (root-cause) důvod selhání, ne na drobné varovné hlášky.\n\n")
	sb.WriteString("=== Specifikace úlohy ===\n")
	sb.WriteString(specExcerpt)
	sb.WriteString("\n\n=== Posledních ~200 řádek výstupu ===\n")
	sb.WriteString(tail)
	return sb.String()
}

// tailLines returns at most n trailing newline-separated lines from s.
// If s contains fewer lines than n, the entire string is returned.
// A trailing partial line (no terminating newline) counts as a line.
func tailLines(s string, n int) string {
	if s == "" || n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	// strings.Split on a trailing newline produces a final empty element,
	// which represents the empty partial line after the last \n. Drop it so
	// "a\nb\n" is treated as 2 lines, not 3.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
		if len(lines) <= n {
			return s
		}
		return strings.Join(lines[len(lines)-n:], "\n") + "\n"
	}
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// scheduleFailureSummary launches an asynchronous summary generation for the
// given task if the feature is enabled. The summary is written back to the
// tasks.failure_summary column on success. Failures are logged at warn level
// and leave the column untouched. The goroutine uses context.Background so
// it survives runner shutdown — the timeout in generateFailureSummary bounds
// its lifetime.
func (r *Runner) scheduleFailureSummary(taskID uuid.UUID, rawOutput string) {
	if r.config == nil || !r.config.FailureSummaryEnabled {
		return
	}

	go func() {
		if err := r.runFailureSummary(context.Background(), taskID, rawOutput); err != nil {
			slog.Warn("failure summary generation failed",
				"task_id", taskID, "error", err)
		}
	}()
}

// RegenerateFailureSummary re-runs summary generation for a failed task. It
// always runs regardless of FailureSummaryEnabled — the user explicitly asked
// for a regeneration via the UI, so the toggle only governs the automatic
// path. Returns the generated summary on success.
func (r *Runner) RegenerateFailureSummary(ctx context.Context, taskID uuid.UUID) (string, error) {
	if r.db == nil || r.config == nil {
		return "", fmt.Errorf("runner not initialized")
	}

	var task models.Task
	if err := r.db.First(&task, "id = ?", taskID).Error; err != nil {
		return "", fmt.Errorf("load task: %w", err)
	}
	if task.Status != models.TaskStatusFailed {
		return "", fmt.Errorf("task is not failed (status=%s)", task.Status)
	}

	var lastExec models.TaskExecution
	output := ""
	err := r.db.Where("task_id = ?", taskID).
		Order("attempt DESC").
		First(&lastExec).Error
	if err != nil && err != gorm.ErrRecordNotFound { //nolint:errorlint // gorm sentinel
		return "", fmt.Errorf("load execution: %w", err)
	}
	if lastExec.RawOutput != nil {
		output = *lastExec.RawOutput
	}

	summary, err := generateFailureSummary(
		ctx, r.config.ClaudePath, r.config.FailureSummaryModel, task.Spec, output,
	)
	if err != nil {
		return "", err
	}
	if err := r.db.Model(&models.Task{}).
		Where("id = ?", taskID).
		Update("failure_summary", summary).Error; err != nil {
		return "", fmt.Errorf("save summary: %w", err)
	}
	return summary, nil
}

// runFailureSummary performs a single summarization pass and persists the
// result. It loads the task fresh from the database so it sees any recent
// updates (e.g. spec edits) and so callers don't need to pass in the spec.
// rawOutput, when non-empty, is used as the log tail; otherwise the most
// recent execution's raw_output is loaded from the database.
func (r *Runner) runFailureSummary(
	ctx context.Context, taskID uuid.UUID, rawOutput string,
) error {
	if r.db == nil {
		return fmt.Errorf("no database")
	}

	var task models.Task
	if err := r.db.First(&task, "id = ?", taskID).Error; err != nil {
		return fmt.Errorf("load task: %w", err)
	}

	output := rawOutput
	if output == "" {
		var lastExec models.TaskExecution
		err := r.db.Where("task_id = ?", taskID).
			Order("attempt DESC").
			First(&lastExec).Error
		if err != nil && err != gorm.ErrRecordNotFound { //nolint:errorlint // gorm sentinel
			return fmt.Errorf("load execution: %w", err)
		}
		if lastExec.RawOutput != nil {
			output = *lastExec.RawOutput
		}
	}

	summary, err := generateFailureSummary(
		ctx, r.config.ClaudePath, r.config.FailureSummaryModel, task.Spec, output,
	)
	if err != nil {
		return err
	}

	if err := r.db.Model(&models.Task{}).
		Where("id = ?", taskID).
		Update("failure_summary", summary).Error; err != nil {
		return fmt.Errorf("save summary: %w", err)
	}
	slog.Info("failure summary generated", "task_id", taskID, "length", len(summary))
	return nil
}
