package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"botka/internal/box"
	"botka/internal/claude"
	"botka/internal/models"
)

// Executor manages the lifecycle of executing a single task by spawning a Claude Code process.
// It supports both local and remote (SSH-over-Box) projects. The same binary path is used
// for local and remote spawns; for local projects it is resolved via exec.LookPath, while
// for remote projects the path is passed through to ssh as-is so it resolves on the remote host.
type Executor struct {
	localClaudePath  string // resolved local claude binary path
	remoteClaudePath string // unresolved claude binary path to use on the remote host
	waker            *box.Waker
	sshTarget        string
}

// NewExecutor creates a new Executor with the given claude binary path.
// If claudePath is empty or "claude", it will be resolved via exec.LookPath.
// waker and sshTarget may be nil/empty if the deployment has no Box host;
// remote projects will then fail fast when execution is attempted.
func NewExecutor(claudePath string, waker *box.Waker, sshTarget string) (*Executor, error) {
	resolved, err := exec.LookPath(claudePath)
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found at %q: %w", claudePath, err)
	}
	return &Executor{
		localClaudePath:  resolved,
		remoteClaudePath: claudePath,
		waker:            waker,
		sshTarget:        sshTarget,
	}, nil
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
	killed     bool
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
	pr := newProjectRunner(project, e.waker, e.sshTarget, e.remoteClaudePath)
	if pr.isRemote() && e.sshTarget == "" {
		return nil, fmt.Errorf("remote project %q has no SSH target configured", project.Path)
	}

	if err := pr.exists(ctx); err != nil {
		return nil, err
	}
	if err := e.syncSpec(ctx, pr, task); err != nil {
		return nil, fmt.Errorf("spec sync failed: %w", err)
	}
	if project.BranchStrategy == "feature_branch" {
		if err := e.setupBranch(ctx, pr, task); err != nil {
			slog.Warn("branch setup failed", "error", err, "task_id", task.ID)
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	out, err := e.spawnClaude(execCtx, pr, task, buffer)
	if err != nil {
		return nil, err
	}

	// Detect user-initiated kill (parent context cancelled, not timeout).
	if ctx.Err() != nil && execCtx.Err() == nil {
		out.killed = true
	}
	// Also detect kill when parent cancelled even if execCtx also cancelled
	// but not from timeout (timeout sets timedOut).
	if ctx.Err() != nil && !out.timedOut {
		out.killed = true
	}

	result := classifyOutcome(out, task)

	if result.Status == models.TaskStatusDone {
		e.maybeVerify(ctx, pr, result)
	}
	if isSuccessful(result.Status) && project.BranchStrategy == "feature_branch" {
		e.pushAndCreatePR(ctx, pr, task)
	}
	return result, nil
}

// CaptureGitHEAD returns the current git HEAD SHA for the given project.
// For remote projects it dispatches via SSH; for local projects it runs git
// in-process. Returns an empty string on any error.
func CaptureGitHEAD(project *models.Project, waker *box.Waker, sshTarget string) string {
	pr := newProjectRunner(project, waker, sshTarget, "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) //nolint:mnd // capture-head timeout
	defer cancel()
	out, err := pr.runGit(ctx, "rev-parse", "HEAD")
	if err != nil {
		slog.Warn("failed to capture git HEAD", "path", project.Path, "error", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GitRevert resets the project to the given HEAD SHA and cleans untracked files.
// If the project uses feature_branch strategy, it also checks out the default branch
// and deletes the feature branch.
func GitRevert(
	project *models.Project, waker *box.Waker, sshTarget, headSHA string, task *models.Task,
) {
	if headSHA == "" {
		slog.Info("no git HEAD SHA stored, skipping revert", "task_id", task.ID)
		return
	}
	slog.Info("reverting git changes", "task_id", task.ID, "head_sha", headSHA)

	pr := newProjectRunner(project, waker, sshTarget, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute) //nolint:mnd // revert budget
	defer cancel()

	if out, err := pr.runGit(ctx, "reset", "--hard", headSHA); err != nil {
		slog.Error("git reset failed", "task_id", task.ID, "error", err, "output", string(out))
	}
	if out, err := pr.runGit(ctx, "clean", "-fd"); err != nil {
		slog.Error("git clean failed", "task_id", task.ID, "error", err, "output", string(out))
	}

	if project.BranchStrategy == "feature_branch" {
		branchName := fmt.Sprintf("botka/task-%s", task.ID)

		defaultBranch := "main"
		if out, err := pr.runGit(ctx, "symbolic-ref", "refs/remotes/origin/HEAD", "--short"); err == nil {
			parts := strings.SplitN(strings.TrimSpace(string(out)), "/", 2) //nolint:mnd
			if len(parts) == 2 {                                            //nolint:mnd
				defaultBranch = parts[1]
			}
		}

		if out, err := pr.runGit(ctx, "checkout", defaultBranch); err != nil {
			slog.Error("git checkout default branch failed",
				"task_id", task.ID, "error", err, "output", string(out))
		}
		if out, err := pr.runGit(ctx, "branch", "-D", branchName); err != nil {
			slog.Warn("git branch delete failed", "task_id", task.ID, "error", err, "output", string(out))
		}
	}

	slog.Info("git revert completed", "task_id", task.ID)
}

func (e *Executor) syncSpec(ctx context.Context, pr *projectRunner, task *models.Task) error {
	relPath := fmt.Sprintf("docs/specs/task-%s.md", task.ID)
	return pr.writeFile(ctx, relPath, []byte(task.Spec))
}

func (e *Executor) setupBranch(ctx context.Context, pr *projectRunner, task *models.Task) error {
	branchName := fmt.Sprintf("botka/task-%s", task.ID)
	if _, err := pr.runGit(ctx, "checkout", "-b", branchName); err == nil {
		return nil
	}
	_, err := pr.runGit(ctx, "checkout", branchName)
	return err
}

func (e *Executor) buildPrompt(task *models.Task) string {
	prompt := fmt.Sprintf(
		"You are working on task: %s. Read the full specification at docs/specs/task-%s.md "+
			"and implement it completely. When done, commit your changes with a descriptive commit message."+
			" Include the spec file docs/specs/task-%s.md in your commit."+
			" IMPORTANT: NEVER run deploy, restart, or service management commands (make deploy, systemctl restart, etc.)"+
			" — you are running inside the application and would kill yourself.",
		task.Title, task.ID, task.ID,
	)
	if task.RetryCount > 0 && task.FailureReason != nil {
		prompt += fmt.Sprintf(
			" Previous attempt failed with: %s. Fix the issues and complete the task.",
			*task.FailureReason,
		)
	}
	return prompt
}

// nonInteractivePrompt is always appended as a system prompt for task executions.
// Task agents run without a user present, so interactive tools like AskUserQuestion
// will fail. This prompt tells Claude to make reasonable assumptions instead.
const nonInteractivePrompt = `You are running as an autonomous task agent in non-interactive mode. ` +
	`There is no user present to answer questions. The AskUserQuestion tool is NOT available and will fail ` +
	`if you try to use it. Do NOT call AskUserQuestion or any tool that requires interactive user input. ` +
	`Instead, make reasonable assumptions based on the task specification and codebase context. ` +
	`If a decision is ambiguous, choose the most conventional option and document your reasoning in a code comment or commit message.`

// botkaSafetyPrompt is appended as a system prompt when executing tasks on the
// botka project itself, to prevent task agents from running commands that would
// restart the service and kill the agent's own process.
const botkaSafetyPrompt = `CRITICAL SAFETY RULE: You are running as an autonomous task agent inside the Botka process. ` +
	`Running 'make deploy', 'make install-service', 'systemctl restart botka', or 'systemctl stop botka' ` +
	`will kill your own process immediately. NEVER run these commands. If deployment is needed, ` +
	`just commit your changes and note it in the task output.`

// isBotkaProject returns true if the given project is the botka application itself.
func isBotkaProject(project *models.Project) bool {
	name := strings.ToLower(project.Name)
	return name == "botka" || strings.HasSuffix(project.Path, "/botka")
}

// buildSpawnCmd returns an exec.Cmd that runs claude with the given args in
// the project's working directory. For local projects it runs the resolved
// local claude binary with cmd.Dir set; for remote projects it wraps the
// invocation in an SSH call to Box. Remote projects call EnsureUp first.
func (e *Executor) buildSpawnCmd(
	ctx context.Context, pr *projectRunner, claudeArgs []string,
) (*exec.Cmd, error) {
	if pr.isRemote() {
		if err := pr.ensureWake(ctx); err != nil {
			return nil, err
		}
		sshArgs := buildTaskSSHArgs(pr.remote.SSHTarget, pr.remoteDir, e.remoteClaudePath, claudeArgs)
		return exec.CommandContext(ctx, sshArgs[0], sshArgs[1:]...), nil //nolint:gosec // args are controlled
	}
	cmd := exec.CommandContext(ctx, e.localClaudePath, claudeArgs...) //nolint:gosec // args are controlled
	cmd.Dir = pr.project.Path
	return cmd, nil
}

// buildTaskSSHArgs assembles an ssh argv that cd's into remoteDir and exec's
// claude with the given args. Task execution's claude invocation has the
// prompt already baked into claudeArgs (as a -p value), so this helper does
// not accept a separate prompt like the chat runner's BuildSSHArgs.
func buildTaskSSHArgs(sshTarget, remoteDir, claudePath string, claudeArgs []string) []string {
	var sb strings.Builder
	sb.WriteString("cd ")
	sb.WriteString(shellQuote(remoteDir))
	sb.WriteString(" && exec ")
	sb.WriteString(shellQuote(claudePath))
	for _, a := range claudeArgs {
		sb.WriteByte(' ')
		sb.WriteString(shellQuote(a))
	}
	return []string{
		"ssh",
		"-o", "BatchMode=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "StrictHostKeyChecking=no",
		sshTarget,
		sb.String(),
	}
}

func (e *Executor) spawnClaude(
	ctx context.Context, pr *projectRunner, task *models.Task, buffer *Buffer,
) (*spawnOutput, error) {
	claudeArgs := []string{
		"--dangerously-skip-permissions", "--verbose",
		"--output-format", "stream-json",
	}
	systemPrompt := nonInteractivePrompt
	if isBotkaProject(pr.project) {
		systemPrompt += " " + botkaSafetyPrompt
	}
	claudeArgs = append(claudeArgs, "--append-system-prompt", systemPrompt)
	claudeArgs = append(claudeArgs, "-p", e.buildPrompt(task))

	cmd, err := e.buildSpawnCmd(ctx, pr, claudeArgs)
	if err != nil {
		return nil, err
	}
	cmd.Env = append(claude.SanitizedEnv(), "BOTKA_TASK_AGENT=1")
	// Use a process group so we can kill the entire tree (claude + child processes,
	// or the ssh client + its children) on timeout or cancellation.
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
	if out.killed {
		return &ExecutionResult{
			Status:       models.TaskStatusFailed,
			ErrorMessage: "Killed by user",
		}
	}
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

func (e *Executor) maybeVerify(ctx context.Context, pr *projectRunner, result *ExecutionResult) {
	if pr.project.VerificationCommand == nil || *pr.project.VerificationCommand == "" {
		return
	}
	verCtx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	output, err := pr.runInProject(verCtx, "bash", "-c", *pr.project.VerificationCommand)
	if err != nil {
		result.Status = models.TaskStatusNeedsReview
		result.Summary += fmt.Sprintf("\n\nVerification failed:\n%s", string(output))
		slog.Warn("verification failed", "project", pr.project.Name, "error", err)
	}
}

func (e *Executor) pushAndCreatePR(ctx context.Context, pr *projectRunner, task *models.Task) {
	branchName := fmt.Sprintf("botka/task-%s", task.ID)
	if _, err := pr.runGit(ctx, "push", "-u", "origin", branchName); err != nil {
		slog.Warn("git push failed", "error", err, "task_id", task.ID)
		return
	}
	// gh is expected to be installed on whichever host runs the project.
	title := fmt.Sprintf("botka: %s", task.Title)
	body := fmt.Sprintf("Automated task implementation\n\nTask ID: %s", task.ID)
	if _, err := pr.runInProject(ctx, "gh", "pr", "create", "--title", title, "--body", body); err != nil {
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
