package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"botka/internal/models"
)

// Executor manages the lifecycle of executing a single task by spawning a Claude Code process.
type Executor struct {
	claudePath string
}

// NewExecutor creates a new Executor with the given claude binary path.
// If claudePath is empty or "claude", it will be resolved via exec.LookPath.
func NewExecutor(claudePath string) (*Executor, error) {
	resolved, err := exec.LookPath(claudePath)
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found at %q: %w", claudePath, err)
	}
	return &Executor{claudePath: resolved}, nil
}

// ExecutionResult holds the outcome of a task execution attempt.
type ExecutionResult struct {
	Status       models.TaskStatus
	CostUSD      float64
	DurationMs   int64
	Summary      string
	ErrorMessage string
	ShouldRetry  bool
	RetryAfter   time.Duration
}

// spawnOutput collects raw output data from a claude process.
type spawnOutput struct {
	exitCode   int
	stderr     string
	lastResult *Event
	lastText   string
	timedOut   bool
}

const (
	execTimeout         = 30 * time.Minute
	verifyTimeout       = 5 * time.Minute
	gracefulStopTimeout = 10 * time.Second
	maxRetries          = 1
	maxErrLen           = 500
)

// Execute runs a single task against a project, managing the full lifecycle.
func (e *Executor) Execute(
	ctx context.Context, task *models.Task, project *models.Project, buffer *Buffer,
) (*ExecutionResult, error) {
	if _, err := os.Stat(project.Path); os.IsNotExist(err) {
		return nil, fmt.Errorf("project directory does not exist: %s", project.Path)
	}
	if err := e.syncSpec(task, project); err != nil {
		return nil, fmt.Errorf("spec sync failed: %w", err)
	}
	if project.BranchStrategy == "feature_branch" {
		if err := e.setupBranch(ctx, task, project); err != nil {
			slog.Warn("branch setup failed", "error", err, "task_id", task.ID)
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	out, err := e.spawnClaude(execCtx, e.claudePath, task, project, buffer)
	if err != nil {
		return nil, err
	}
	result := classifyOutcome(out, task)

	if result.Status == models.TaskStatusDone {
		e.maybeVerify(ctx, project, result)
	}
	if isSuccessful(result.Status) && project.BranchStrategy == "feature_branch" {
		e.pushAndCreatePR(ctx, task, project)
	}
	return result, nil
}

func (e *Executor) syncSpec(task *models.Task, project *models.Project) error {
	specDir := filepath.Join(project.Path, "docs", "specs")
	if err := os.MkdirAll(specDir, 0o750); err != nil {
		return err
	}
	specFile := filepath.Join(specDir, fmt.Sprintf("task-%s.md", task.ID))
	return os.WriteFile(specFile, []byte(task.Spec), 0o600) //nolint:gosec // spec file in project dir
}

func (e *Executor) setupBranch(ctx context.Context, task *models.Task, project *models.Project) error {
	branchName := fmt.Sprintf("botka/task-%s", task.ID)
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName) //nolint:gosec // UUID branch name
	cmd.Dir = project.Path
	if err := cmd.Run(); err != nil {
		cmd = exec.CommandContext(ctx, "git", "checkout", branchName) //nolint:gosec // UUID branch name
		cmd.Dir = project.Path
		return cmd.Run()
	}
	return nil
}

func (e *Executor) buildPrompt(task *models.Task) string {
	prompt := fmt.Sprintf(
		"You are working on task: %s. Read the full specification at docs/specs/task-%s.md "+
			"and implement it completely. When done, commit your changes with a descriptive commit message.",
		task.Title, task.ID,
	)
	if task.RetryCount > 0 && task.FailureReason != nil {
		prompt += fmt.Sprintf(
			" Previous attempt failed with: %s. Fix the issues and complete the task.",
			*task.FailureReason,
		)
	}
	return prompt
}

func (e *Executor) spawnClaude(
	ctx context.Context, claudePath string, task *models.Task,
	project *models.Project, buffer *Buffer,
) (*spawnOutput, error) {
	cmd := exec.CommandContext(ctx, claudePath, //nolint:gosec // args are controlled
		"--dangerously-skip-permissions", "--verbose",
		"--output-format", "stream-json",
		"-p", e.buildPrompt(task),
	)
	cmd.Dir = project.Path
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = gracefulStopTimeout

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	out := &spawnOutput{}
	parseErr := ParseStream(io.TeeReader(stdout, buffer), func(ev Event) {
		switch ev.Type {
		case EventResult:
			evCopy := ev
			out.lastResult = &evCopy
		case EventAssistantText:
			out.lastText = ev.Text
		}
	})

	waitErr := cmd.Wait()
	out.stderr = stderrBuf.String()
	if ctx.Err() != nil {
		out.timedOut = true
		return out, nil //nolint:nilerr // timeout classified via spawnOutput.timedOut, not Go error
	}
	if parseErr != nil {
		slog.Warn("stream parse error", "error", parseErr)
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		out.exitCode = exitErr.ExitCode()
	} else if waitErr != nil {
		return nil, fmt.Errorf("wait for claude: %w", waitErr)
	}
	return out, nil
}

func classifyOutcome(out *spawnOutput, task *models.Task) *ExecutionResult {
	if out.timedOut {
		return &ExecutionResult{
			Status:       models.TaskStatusFailed,
			ErrorMessage: "execution timed out",
			ShouldRetry:  task.RetryCount < maxRetries,
		}
	}
	allOutput := out.stderr + out.lastText
	if out.lastResult == nil {
		return classifyCrash(out.exitCode, allOutput, task)
	}
	if out.exitCode != 0 && isAPIError(allOutput) {
		return &ExecutionResult{
			Status:       models.TaskStatusFailed,
			CostUSD:      out.lastResult.CostUSD,
			DurationMs:   out.lastResult.DurationMs,
			ErrorMessage: fmt.Sprintf("API error (exit code %d): %s", out.exitCode, truncate(out.stderr, maxErrLen)),
			RetryAfter:   time.Hour,
		}
	}
	if out.exitCode != 0 || out.lastResult.IsError {
		return buildFailureResult(out, task)
	}
	return &ExecutionResult{
		Status:     models.TaskStatusDone,
		CostUSD:    out.lastResult.CostUSD,
		DurationMs: out.lastResult.DurationMs,
		Summary:    out.lastText,
	}
}

func buildFailureResult(out *spawnOutput, task *models.Task) *ExecutionResult {
	errMsg := truncate(out.stderr, maxErrLen)
	if errMsg == "" {
		errMsg = "claude process exited with error"
	}
	return &ExecutionResult{
		Status:       models.TaskStatusFailed,
		CostUSD:      out.lastResult.CostUSD,
		DurationMs:   out.lastResult.DurationMs,
		Summary:      out.lastText,
		ErrorMessage: errMsg,
		ShouldRetry:  task.RetryCount < maxRetries,
	}
}

func classifyCrash(exitCode int, output string, task *models.Task) *ExecutionResult {
	if isAPIError(output) {
		return &ExecutionResult{
			Status:       models.TaskStatusFailed,
			ErrorMessage: fmt.Sprintf("API error (exit code %d)", exitCode),
			RetryAfter:   time.Hour,
		}
	}
	return &ExecutionResult{
		Status:       models.TaskStatusFailed,
		ErrorMessage: fmt.Sprintf("claude process crashed (exit code %d)", exitCode),
		ShouldRetry:  task.RetryCount < maxRetries,
	}
}

var apiErrorPatterns = []string{"500", "502", "503", "529", "overloaded", "rate_limit", "capacity"}

func isAPIError(output string) bool {
	lower := strings.ToLower(output)
	for _, p := range apiErrorPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (e *Executor) maybeVerify(ctx context.Context, project *models.Project, result *ExecutionResult) {
	if project.VerificationCommand == nil || *project.VerificationCommand == "" {
		return
	}
	verCtx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	cmd := exec.CommandContext(verCtx, "bash", "-c", *project.VerificationCommand) //nolint:gosec // user-configured
	cmd.Dir = project.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = models.TaskStatusNeedsReview
		result.Summary += fmt.Sprintf("\n\nVerification failed:\n%s", string(output))
		slog.Warn("verification failed", "project", project.Name, "error", err)
	}
}

func (e *Executor) pushAndCreatePR(ctx context.Context, task *models.Task, project *models.Project) {
	branchName := fmt.Sprintf("botka/task-%s", task.ID)
	cmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", branchName) //nolint:gosec // UUID branch
	cmd.Dir = project.Path
	if err := cmd.Run(); err != nil {
		slog.Warn("git push failed", "error", err, "task_id", task.ID)
		return
	}
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		slog.Info("gh CLI not available, skipping PR creation")
		return
	}
	title := fmt.Sprintf("botka: %s", task.Title)
	body := fmt.Sprintf("Automated task implementation\n\nTask ID: %s", task.ID)
	prCmd := exec.CommandContext(ctx, ghPath, "pr", "create", "--title", title, "--body", body) //nolint:gosec
	prCmd.Dir = project.Path
	if err := prCmd.Run(); err != nil {
		slog.Warn("PR creation failed", "error", err, "task_id", task.ID)
	}
}

func isSuccessful(status models.TaskStatus) bool {
	return status == models.TaskStatusDone || status == models.TaskStatusNeedsReview
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
