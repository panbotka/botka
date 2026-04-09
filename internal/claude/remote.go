package claude

import (
	"context"
	"fmt"
	"strings"

	"botka/internal/box"
)

// RemotePrefix identifies a working directory that lives on the remote Box
// build machine. A RunConfig.WorkDir of "box:/home/box/projects/app" tells
// the runner to spawn Claude Code via SSH on Box with /home/box/projects/app
// as the working directory.
const RemotePrefix = "box:"

// RemoteSpec describes how to reach a remote Claude Code host.
// A zero value is not usable — the SSH target and Waker must be set.
type RemoteSpec struct {
	// SSHTarget is the "user@host" SSH destination (e.g. "box@box").
	SSHTarget string

	// Waker ensures the remote host is awake before each command.
	// It may be nil, in which case wake-on-LAN is skipped and the SSH
	// command is issued directly. Nil is useful in tests.
	Waker *box.Waker
}

// IsRemotePath reports whether the given work directory points at the remote
// Box host. It uses the RemotePrefix convention.
func IsRemotePath(workDir string) bool {
	return strings.HasPrefix(workDir, RemotePrefix)
}

// SplitRemotePath strips the RemotePrefix from workDir. If workDir is not a
// remote path, it is returned unchanged and isRemote is false.
func SplitRemotePath(workDir string) (path string, isRemote bool) {
	if !IsRemotePath(workDir) {
		return workDir, false
	}
	return strings.TrimPrefix(workDir, RemotePrefix), true
}

// BuildSSHArgs assembles the argv used to run the given claude invocation
// over SSH on the remote host. The returned slice has "ssh" as argv[0] and
// ends with the fully quoted remote command. Remote exec is a single shell
// string because SSH concatenates extra args with spaces on the remote side,
// and we need to preserve argument boundaries for Claude Code's -p flag.
//
// claudePath is the remote claude binary path (usually just "claude"), args
// are the claude flags (without the prompt), and prompt is the final "-p"
// argument, which may contain arbitrary characters.
func BuildSSHArgs(sshTarget, remoteDir, claudePath string, args []string, prompt string) []string {
	// Build the remote shell command. We cd into the working directory and
	// then exec claude with the supplied args. Every user-supplied value is
	// passed through shellQuote so whitespace and special characters don't
	// break the command.
	var sb strings.Builder
	sb.WriteString("cd ")
	sb.WriteString(shellQuote(remoteDir))
	sb.WriteString(" && exec ")
	sb.WriteString(shellQuote(claudePath))
	for _, a := range args {
		sb.WriteByte(' ')
		sb.WriteString(shellQuote(a))
	}
	sb.WriteByte(' ')
	sb.WriteString(shellQuote(prompt))

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

// shellQuote wraps a value in single quotes for safe execution by a POSIX
// shell on the remote host. Embedded single quotes are escaped using the
// standard '\” sequence. This is deliberately paranoid because user content
// (prompts, project paths) flows into the remote shell unchanged.
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

// ensureRemoteUp is a small helper used by both the one-shot runner and the
// persistent session manager before they attempt an SSH command. It returns
// nil if the waker is nil (useful in tests) or Box is already up.
func ensureRemoteUp(ctx context.Context, remote *RemoteSpec) error {
	if remote == nil || remote.Waker == nil {
		return nil
	}
	if err := remote.Waker.EnsureUp(ctx); err != nil {
		return fmt.Errorf("box ensure up: %w", err)
	}
	return nil
}
