package box

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// sshTransportExitCode is the status ssh(1) exits with when it fails on its own
// (authentication, connection teardown) rather than relaying the remote
// command's exit status.
const sshTransportExitCode = 255

// shutdownDisconnectMarkers are the messages ssh prints when the remote host
// tears the connection down. Once `shutdown now` takes effect, sshd goes away
// mid-session and ssh reports one of these — the expected happy path.
var shutdownDisconnectMarkers = []string{
	"closed by remote host",
	"connection closed by",
	"connection reset by peer",
	"broken pipe",
}

// sshFailureMarkers indicate ssh never reached the point of running the command,
// or that sudo refused it. ssh reports these with the same exit status as a
// shutdown-induced disconnect, so they must be matched explicitly.
var sshFailureMarkers = []string{
	"permission denied",
	"a password is required",
	"authentication failure",
	"too many authentication failures",
	"host key verification failed",
	"connection refused",
	"connection timed out",
	"operation timed out",
	"no route to host",
	"network is unreachable",
	"could not resolve hostname",
	"not in the sudoers file",
	"is not allowed to run",
}

// exitCoder is satisfied by *exec.ExitError; it lets tests inject a fake error
// that carries a specific process exit status.
type exitCoder interface {
	ExitCode() int
}

// isExpectedShutdownDisconnect reports whether a failed `ssh … sudo shutdown now`
// invocation actually means the box began powering off.
//
// A poweroff kills sshd mid-session, so ssh legitimately exits non-zero even on
// success. The signal that separates the two cases is *why* ssh gave up:
//
//   - An auth/sudo/connection failure names itself in the output ("Permission
//     denied", "sudo: a password is required", …). These happen immediately,
//     before shutdown could ever start, so they are always real failures.
//   - A non-255 exit status is the remote command's own status relayed by ssh,
//     meaning the session survived long enough to report it — the box is not
//     going down, so this is a failure too.
//   - Only a transport-level teardown (exit 255 with a disconnect message and no
//     failure marker) indicates the host went away under us, i.e. success.
//
// Anything unrecognized is treated as a failure: silently reporting success is
// exactly the bug this guards against.
func isExpectedShutdownDisconnect(err error, output string) bool {
	if err == nil {
		return true
	}

	lower := strings.ToLower(output)
	for _, marker := range sshFailureMarkers {
		if strings.Contains(lower, marker) {
			return false
		}
	}

	var ec exitCoder
	if errors.As(err, &ec) && ec.ExitCode() >= 0 && ec.ExitCode() != sshTransportExitCode {
		return false
	}

	for _, marker := range shutdownDisconnectMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	return false
}

// RunFunc runs a command and returns its combined output. It matches the
// signature of the command runners used by the box handler and the auto-off
// monitor, so both can pass their own (mockable) implementation.
type RunFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// Shutdown powers the Box off over SSH.
//
// A failing ssh invocation is reported as an error unless the failure is the
// connection teardown caused by the machine actually powering off — see
// isExpectedShutdownDisconnect. A cancelled or timed-out context is always a
// failure: the command never disconnected cleanly, so nothing can be inferred
// from its output.
func Shutdown(ctx context.Context, run RunFunc, sshTarget string) error {
	output, err := run(ctx, "ssh", "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10", sshTarget, "sudo", "shutdown", "now")
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return shutdownError(fmt.Errorf("%w: %w", ctxErr, err), output)
	}
	if !isExpectedShutdownDisconnect(err, string(output)) {
		return shutdownError(err, output)
	}
	return nil
}

// shutdownError builds an error carrying both the cause and whatever SSH
// printed, so callers can show why the shutdown failed.
func shutdownError(err error, output []byte) error {
	if out := strings.TrimSpace(string(output)); out != "" {
		return fmt.Errorf("shutdown failed: %w: %s", err, out)
	}
	return fmt.Errorf("shutdown failed: %w", err)
}
