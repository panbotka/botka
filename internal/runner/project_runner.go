package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"botka/internal/box"
	"botka/internal/claude"
	"botka/internal/models"
)

// projectRunner abstracts filesystem and command execution for a project
// regardless of whether the project lives locally or on the remote Box host.
// The executor constructs one per task and routes all git, spec-sync, and
// verification work through it.
//
// For local projects, projectRunner is a thin wrapper over exec.Cmd and
// os.WriteFile. For remote projects (project.Path starting with "box:"),
// it issues ssh commands that cd into the remote working directory before
// running the requested command.
type projectRunner struct {
	project    *models.Project
	remote     *claude.RemoteSpec
	remoteDir  string // stripped remote path (empty for local projects)
	claudePath string // claude binary path used for chat/task spawns
}

// newProjectRunner returns a projectRunner configured for the given project.
// waker may be nil, in which case the returned runner never calls wake-on-LAN
// (useful when running tests or when the executor was built without a waker).
func newProjectRunner(project *models.Project, waker *box.Waker, sshTarget, claudePath string) *projectRunner {
	pr := &projectRunner{
		project:    project,
		claudePath: claudePath,
	}
	if claude.IsRemotePath(project.Path) {
		pr.remoteDir, _ = claude.SplitRemotePath(project.Path)
		pr.remote = &claude.RemoteSpec{
			SSHTarget: sshTarget,
			Waker:     waker,
		}
	}
	return pr
}

// isRemote reports whether the project runs on the Box host.
func (p *projectRunner) isRemote() bool {
	return p.remote != nil
}

// exists checks that the project's working directory exists. For local
// projects it uses os.Stat; for remote projects it runs "test -d" over SSH.
// Errors are only returned for confirmed-missing directories; transient
// SSH failures are treated as success to avoid blocking task execution.
func (p *projectRunner) exists(ctx context.Context) error {
	if !p.isRemote() {
		if _, err := os.Stat(p.project.Path); os.IsNotExist(err) {
			return fmt.Errorf("project directory does not exist: %s", p.project.Path)
		}
		return nil
	}
	if err := p.ensureWake(ctx); err != nil {
		return err
	}
	_, err := p.runShell(ctx, fmt.Sprintf("test -d %s", shellQuote(p.remoteDir)))
	if err != nil {
		return fmt.Errorf("project directory does not exist on remote: %s", p.project.Path)
	}
	return nil
}

// writeFile writes data to a file under the project's working directory,
// creating parent directories as needed. relPath is relative to the project
// root and must not contain "..".
func (p *projectRunner) writeFile(ctx context.Context, relPath string, data []byte) error {
	if strings.Contains(relPath, "..") {
		return errors.New("relPath must not contain parent-dir segments")
	}
	if !p.isRemote() {
		full := filepath.Join(p.project.Path, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			return err
		}
		return os.WriteFile(full, data, 0o600) //nolint:gosec // spec file in project dir
	}

	if err := p.ensureWake(ctx); err != nil {
		return err
	}
	full := filepath.Join(p.remoteDir, relPath)
	// Ensure parent dir exists on remote, then pipe data to the target file.
	mkdirCmd := fmt.Sprintf("mkdir -p %s", shellQuote(filepath.Dir(full)))
	if _, err := p.runShell(ctx, mkdirCmd); err != nil {
		return fmt.Errorf("remote mkdir: %w", err)
	}
	writeCmd := fmt.Sprintf("cat > %s", shellQuote(full))
	cmd := exec.CommandContext(ctx, "ssh", p.sshOptions(writeCmd)...) //nolint:gosec // args are controlled
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remote write %s: %w: %s", full, err, stderr.String())
	}
	return nil
}

// runGit runs a git command inside the project's working directory and
// returns its combined output. The command runs locally for local projects
// or via SSH for remote ones.
func (p *projectRunner) runGit(ctx context.Context, args ...string) ([]byte, error) {
	return p.runInProject(ctx, "git", args...)
}

// runInProject runs an arbitrary command inside the project's working
// directory. It is used for git, gh, and verification commands.
func (p *projectRunner) runInProject(ctx context.Context, name string, args ...string) ([]byte, error) {
	if !p.isRemote() {
		cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // caller passes trusted name
		cmd.Dir = p.project.Path
		return cmd.CombinedOutput()
	}

	if err := p.ensureWake(ctx); err != nil {
		return nil, err
	}
	var sb strings.Builder
	sb.WriteString("cd ")
	sb.WriteString(shellQuote(p.remoteDir))
	sb.WriteString(" && ")
	sb.WriteString(shellQuote(name))
	for _, a := range args {
		sb.WriteByte(' ')
		sb.WriteString(shellQuote(a))
	}
	return p.runShell(ctx, sb.String())
}

// runShell executes a raw shell command on the remote host and returns its
// combined output. It is only used for remote projects.
func (p *projectRunner) runShell(ctx context.Context, shellCmd string) ([]byte, error) {
	if !p.isRemote() {
		return nil, errors.New("runShell is only valid for remote projects")
	}
	cmd := exec.CommandContext(ctx, "ssh", p.sshOptions(shellCmd)...) //nolint:gosec // args are controlled
	return cmd.CombinedOutput()
}

// sshOptions returns the argv tail for an ssh invocation that runs the given
// shell command on the remote host. argv[0] is not included ("ssh" is fixed).
func (p *projectRunner) sshOptions(shellCmd string) []string {
	return []string{
		"-o", "BatchMode=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "StrictHostKeyChecking=no",
		p.remote.SSHTarget,
		shellCmd,
	}
}

// ensureWake calls the Waker if one is configured. It is a no-op for local
// projects or when the waker is nil.
func (p *projectRunner) ensureWake(ctx context.Context) error {
	if p.remote == nil || p.remote.Waker == nil {
		return nil
	}
	if err := p.remote.Waker.EnsureUp(ctx); err != nil {
		return fmt.Errorf("box ensure up: %w", err)
	}
	return nil
}

// shellQuote wraps a value in single quotes for safe use in a POSIX shell
// command. This is a duplicate of the claude-package helper so the runner
// package does not depend on unexported symbols.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	var sb strings.Builder
	sb.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			sb.WriteString(`'\''`)
			continue
		}
		sb.WriteByte(s[i])
	}
	sb.WriteByte('\'')
	return sb.String()
}
