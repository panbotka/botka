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
	// onPhase, when set, is called immediately before the executor enters each
	// step of the pipeline. The Runner wires it to a persist-and-broadcast
	// callback. It must never fail the run — implementations swallow their own
	// errors — and it is nil in unit tests that exercise the steps directly.
	onPhase func(task *models.Task, phase models.RunPhase)
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
	Status              models.TaskStatus
	CostUSD             float64
	DurationMs          int64
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	Model               string
	Summary             string
	ErrorMessage        string
	ShouldRetry         bool
	RetryAfter          time.Duration
}

// spawnOutput collects raw output data from a claude process.
type spawnOutput struct {
	exitCode   int
	stderr     string
	lastResult *Event
	lastText   string
	model      string
	timedOut   bool
	killed     bool
}

const (
	execTimeout         = 30 * time.Minute
	verifyTimeout       = 5 * time.Minute
	gracefulStopTimeout = 10 * time.Second
	maxRetries          = 1
	maxErrLen           = 500
	// leftoverCommitBudget bounds the git work of preserving a task's
	// uncommitted changes; it must comfortably cover an add + commit + push
	// over the network.
	leftoverCommitBudget = 3 * time.Minute
)

// Execute runs a single task against a project, managing the full lifecycle.
// mcpConfigPath is the path to a generated .mcp.json file; empty means no MCP servers.
func (e *Executor) Execute(
	ctx context.Context, task *models.Task, project *models.Project, buffer *Buffer,
	mcpConfigPath string,
) (*ExecutionResult, error) {
	pr := newProjectRunner(project, e.waker, e.sshTarget, e.remoteClaudePath)
	if pr.isRemote() && e.sshTarget == "" {
		return nil, fmt.Errorf("remote project %q has no SSH target configured", project.Path)
	}

	e.recordPhase(task, models.RunPhasePreparing)
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

	e.recordPhase(task, models.RunPhaseAgent)
	out, err := e.spawnClaude(execCtx, pr, task, buffer, mcpConfigPath)
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
		if pr.hasVerification() {
			e.recordPhase(task, models.RunPhaseVerifying)
		}
		e.maybeVerify(ctx, pr, result)
	}
	if isSuccessful(result.Status) && project.BranchStrategy == "feature_branch" {
		e.recordPhase(task, models.RunPhasePublishing)
		e.pushAndCreatePR(ctx, pr, task)
	}
	return result, nil
}

// recordPhase announces that the task is about to enter the given phase.
// It is a no-op when no recorder is wired.
func (e *Executor) recordPhase(task *models.Task, phase models.RunPhase) {
	if e.onPhase == nil {
		return
	}
	e.onPhase(task, phase)
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

// leftoverCommitArgs builds the git argv for an automated safety-net commit with
// message. It pins a stable identity via `-c` so the runner can commit
// regardless of the service environment's git config, and so these commits are
// clearly attributable to botka rather than to an agent. The override applies to
// this one commit only.
func leftoverCommitArgs(message string) []string {
	return []string{
		"-c", "user.name=botka (task runner)",
		"-c", "user.email=botka@panbotka.cz",
		"commit", "-m", message,
	}
}

// treeIsDirty reports whether the project's working tree has any uncommitted
// changes — staged, unstaged, or untracked. It runs `git status --porcelain`
// and treats any non-empty output as dirty. It returns an error only when the
// status command itself fails.
func treeIsDirty(ctx context.Context, pr *projectRunner) (bool, error) {
	out, err := pr.runGit(ctx, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// CommitLeftovers commits and pushes any uncommitted work left in the project's
// working tree so a task that finished — or was killed by its execution timeout
// — before the agent committed never silently loses its work. It stages
// everything, commits on the current branch with commitMsg, and pushes HEAD to
// origin. It is a no-op when the tree is already clean. Errors are logged, never
// returned: finalizing a task must not be blocked by a git or network hiccup.
func CommitLeftovers(
	project *models.Project, waker *box.Waker, sshTarget string, task *models.Task, commitMsg string,
) {
	pr := newProjectRunner(project, waker, sshTarget, "")
	ctx, cancel := context.WithTimeout(context.Background(), leftoverCommitBudget)
	defer cancel()

	dirty, err := treeIsDirty(ctx, pr)
	if err != nil {
		slog.Error("leftover-commit: status check failed", "task_id", task.ID, "error", err)
		return
	}
	if !dirty {
		return
	}
	slog.Info("committing leftover work", "task_id", task.ID)

	steps := [][]string{
		{"add", "-A"},
		leftoverCommitArgs(commitMsg),
		{"push", "origin", "HEAD"},
	}
	for _, args := range steps {
		if out, err := pr.runGit(ctx, args...); err != nil {
			slog.Error("leftover-commit: git step failed",
				"task_id", task.ID, "args", args, "error", err, "output", string(out))
			return
		}
	}
	slog.Info("leftover work committed and pushed", "task_id", task.ID)
}

// ParkLeftoversAndReset preserves whatever a failed attempt produced — commits
// made past baseSHA and/or an uncommitted working tree — on a pushed
// wip/task-<id>-attempt-<n> branch, then hard-resets the working branch back to
// baseSHA and removes untracked files so the retry starts from a pristine tree.
// It replaces an earlier discard-on-retry behavior: a retry still begins from
// baseSHA, but the abandoned attempt is never lost. attempt is the 1-based
// number of the attempt being parked. Errors are logged, never returned: a
// retry proceeds regardless, and a failed push still leaves the work on a local
// wip branch and in the reflog.
func ParkLeftoversAndReset(
	project *models.Project, waker *box.Waker, sshTarget, baseSHA string, task *models.Task, attempt int,
) {
	if baseSHA == "" {
		slog.Warn("no base commit SHA, skipping pre-retry park+reset", "task_id", task.ID)
		return
	}
	slog.Info("parking attempt and resetting working tree for retry",
		"task_id", task.ID, "base_sha", baseSHA, "attempt", attempt)

	pr := newProjectRunner(project, waker, sshTarget, "")
	ctx, cancel := context.WithTimeout(context.Background(), leftoverCommitBudget)
	defer cancel()

	branch := fmt.Sprintf("wip/task-%s-attempt-%d", task.ID, attempt)
	parkAttempt(ctx, pr, task, baseSHA, branch)

	if out, err := pr.runGit(ctx, "reset", "--hard", baseSHA); err != nil {
		slog.Error("pre-retry git reset failed", "task_id", task.ID, "error", err, "output", string(out))
	}
	if out, err := pr.runGit(ctx, "clean", "-fd"); err != nil {
		slog.Error("pre-retry git clean failed", "task_id", task.ID, "error", err, "output", string(out))
	}
	slog.Info("pre-retry park+reset completed", "task_id", task.ID, "wip_branch", branch)
}

// parkAttempt captures a failed attempt's work — staging and committing a dirty
// tree first when needed — onto the local wip branch and pushes it to origin,
// but only when the attempt actually diverged from baseSHA. It leaves HEAD in
// place for the caller to reset. Errors are logged, never returned.
func parkAttempt(ctx context.Context, pr *projectRunner, task *models.Task, baseSHA, branch string) {
	dirty, err := treeIsDirty(ctx, pr)
	if err != nil {
		slog.Error("park: status check failed", "task_id", task.ID, "error", err)
		return
	}
	if dirty {
		msg := fmt.Sprintf("botka: parked leftovers from timed-out attempt of task %s", task.ID)
		if out, err := pr.runGit(ctx, "add", "-A"); err != nil {
			slog.Error("park: git add failed", "task_id", task.ID, "error", err, "output", string(out))
			return
		}
		if out, err := pr.runGit(ctx, leftoverCommitArgs(msg)...); err != nil {
			slog.Error("park: git commit failed", "task_id", task.ID, "error", err, "output", string(out))
			return
		}
	}
	head, err := pr.runGit(ctx, "rev-parse", "HEAD")
	if err != nil {
		slog.Error("park: rev-parse HEAD failed", "task_id", task.ID, "error", err)
		return
	}
	if strings.TrimSpace(string(head)) == baseSHA {
		return // the attempt produced nothing to preserve
	}
	if out, err := pr.runGit(ctx, "branch", "-f", branch, "HEAD"); err != nil {
		slog.Error("park: create wip branch failed", "task_id", task.ID, "error", err, "output", string(out))
		return
	}
	if out, err := pr.runGit(ctx, "push", "origin", branch); err != nil {
		slog.Warn("park: push wip branch failed (kept on local branch + reflog)",
			"task_id", task.ID, "branch", branch, "error", err, "output", string(out))
	}
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
			" Previous attempt failed with: %s. The working tree has been reset to the task's"+
				" starting commit, so any partial work from that attempt is gone — start fresh"+
				" rather than resuming it, and complete the task.",
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
	mcpConfigPath string,
) (*spawnOutput, error) {
	claudeArgs := []string{
		"--dangerously-skip-permissions", "--verbose",
		"--output-format", "stream-json",
		// Autonomous tasks are latency-insensitive, so buy quality with effort.
		"--effort", "xhigh",
	}
	if mcpConfigPath != "" {
		claudeArgs = append(claudeArgs, "--mcp-config", mcpConfigPath)
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
		case EventSystemInit:
			if ev.Model != "" {
				out.model = ev.Model
			}
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
		result := newResultFromSpawn(out)
		result.Status = models.TaskStatusFailed
		result.ErrorMessage = fmt.Sprintf("API error (exit code %d): %s", out.exitCode, truncate(out.stderr, maxErrLen))
		result.RetryAfter = time.Hour
		return result
	}
	if out.exitCode != 0 || out.lastResult.IsError {
		return buildFailureResult(out, task)
	}
	result := newResultFromSpawn(out)
	result.Status = models.TaskStatusDone
	result.Summary = out.lastText
	return result
}

func buildFailureResult(out *spawnOutput, task *models.Task) *ExecutionResult {
	errMsg := truncate(out.stderr, maxErrLen)
	if errMsg == "" {
		errMsg = "claude process exited with error"
	}
	result := newResultFromSpawn(out)
	result.Status = models.TaskStatusFailed
	result.Summary = out.lastText
	result.ErrorMessage = errMsg
	result.ShouldRetry = task.RetryCount < maxRetries
	return result
}

// newResultFromSpawn builds a baseline ExecutionResult populated with the
// token counts, model, duration, and computed cost from the parsed result
// event. The cost is recomputed locally via the pricing table — Claude Code's
// emitted cost_usd is intentionally ignored so all numbers in the database
// come from a single, version-controlled source of truth.
func newResultFromSpawn(out *spawnOutput) *ExecutionResult {
	r := &ExecutionResult{
		Model: out.model,
	}
	if out.lastResult != nil {
		r.DurationMs = out.lastResult.DurationMs
		r.InputTokens = out.lastResult.InputTokens
		r.OutputTokens = out.lastResult.OutputTokens
		r.CacheReadTokens = out.lastResult.CacheReadTokens
		r.CacheCreationTokens = out.lastResult.CacheCreationTokens
		r.CostUSD = computeCost(
			out.model,
			r.InputTokens, r.OutputTokens,
			r.CacheReadTokens, r.CacheCreationTokens,
		)
	}
	return r
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
	if !pr.hasVerification() {
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
