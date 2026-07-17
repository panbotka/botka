package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestKillGroupOnCancel_KillsStubbornGrandchild verifies that cancelling a
// command whose process tree ignores SIGTERM still tears down the entire group.
// The leader and a grandchild both trap-and-ignore SIGTERM; only a group-wide
// SIGKILL reaches the grandchild. Go's os/exec WaitDelay would SIGKILL just the
// leader PID, leaving the grandchild running as an orphan — the bug this guards.
func TestKillGroupOnCancel_KillsStubbornGrandchild(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	// Leader ignores TERM and spawns a child sh that also ignores TERM, records
	// its own PID, and sleeps. The child is not the process-group leader, so a
	// leader-only SIGKILL would leave it running.
	script := "trap '' TERM\n" +
		"sh -c 'trap \"\" TERM; echo $$ > \"" + pidFile + "\"; sleep 60' &\n" +
		"wait\n"
	scriptPath := filepath.Join(dir, "tree.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = gracefulStopTimeout + groupKillGrace
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pgid := cmd.Process.Pid

	grandchild := waitForPIDFile(t, pidFile)

	reaped := make(chan struct{})
	// Short grace so the test stays fast.
	go killGroupOnCancel(ctx, pgid, 200*time.Millisecond, reaped)

	cancel()
	_ = cmd.Wait()
	close(reaped)

	if !waitProcessGone(grandchild, 3*time.Second) {
		// Best-effort cleanup so a failing test doesn't leak the process tree.
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		_ = syscall.Kill(grandchild, syscall.SIGKILL)
		t.Fatalf("grandchild pid %d still alive after cancel — group SIGKILL did not reach it", grandchild)
	}
}

// waitForPIDFile polls for pidFile to appear and returns the PID it holds.
func waitForPIDFile(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("grandchild pid file never appeared: %s", pidFile)
	return 0
}

// waitProcessGone reports whether pid stops existing within timeout. It probes
// with signal 0, which returns an error (ESRCH) once the process is gone.
func waitProcessGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
