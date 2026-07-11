package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"botka/internal/models"
)

// gitRepoWithRemote creates a working repo wired to a bare `origin` remote, with
// one commit on main already pushed. It returns the working directory and the
// base commit SHA. Both repos are cleaned up with the test. It skips the test
// when git is unavailable.
func gitRepoWithRemote(t *testing.T) (workDir, baseSHA string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	bareDir := t.TempDir()
	workDir = t.TempDir()

	runGitIn(t, bareDir, "init", "--bare", "-b", "main")
	setup := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
		{"remote", "add", "origin", bareDir},
		{"commit", "--allow-empty", "-m", "init"},
		{"push", "-u", "origin", "main"},
	}
	for _, args := range setup {
		runGitIn(t, workDir, args...)
	}
	return workDir, runGitIn(t, workDir, "rev-parse", "HEAD")
}

// writeFileT writes name (relative to dir) with the given contents, failing the
// test on error.
func writeFileT(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestTreeIsDirty_CleanAndDirty(t *testing.T) {
	t.Parallel()
	workDir, _ := gitRepoWithRemote(t)
	pr := newProjectRunner(&models.Project{Path: workDir}, nil, "", "")

	dirty, err := treeIsDirty(context.Background(), pr)
	if err != nil {
		t.Fatalf("treeIsDirty(clean) error = %v", err)
	}
	if dirty {
		t.Error("treeIsDirty = true on a clean tree, want false")
	}

	writeFileT(t, workDir, "new.txt", "leftover")
	dirty, err = treeIsDirty(context.Background(), pr)
	if err != nil {
		t.Fatalf("treeIsDirty(dirty) error = %v", err)
	}
	if !dirty {
		t.Error("treeIsDirty = false with an untracked file, want true")
	}
}

func TestCommitLeftovers_CommitsAndPushesDirtyTree(t *testing.T) {
	t.Parallel()
	workDir, baseSHA := gitRepoWithRemote(t)
	writeFileT(t, workDir, "leftover.txt", "work the agent forgot to commit")

	task := &models.Task{ID: uuid.New(), Title: "some task"}
	CommitLeftovers(&models.Project{Path: workDir}, nil, "", task, "botka: auto-commit leftover")

	if got := runGitIn(t, workDir, "status", "--porcelain"); got != "" {
		t.Errorf("working tree not clean after CommitLeftovers: %q", got)
	}
	head := runGitIn(t, workDir, "rev-parse", "HEAD")
	if head == baseSHA {
		t.Fatal("HEAD did not advance; nothing was committed")
	}
	if remote := runGitIn(t, workDir, "rev-parse", "origin/main"); remote != head {
		t.Errorf("origin/main = %s, want pushed HEAD %s", remote, head)
	}
}

func TestCommitLeftovers_NoopOnCleanTree(t *testing.T) {
	t.Parallel()
	workDir, baseSHA := gitRepoWithRemote(t)

	task := &models.Task{ID: uuid.New(), Title: "clean task"}
	CommitLeftovers(&models.Project{Path: workDir}, nil, "", task, "botka: auto-commit leftover")

	if head := runGitIn(t, workDir, "rev-parse", "HEAD"); head != baseSHA {
		t.Errorf("HEAD = %s, want unchanged base %s (nothing to commit)", head, baseSHA)
	}
}

func TestParkLeftoversAndReset_PreservesAndResets(t *testing.T) {
	t.Parallel()
	workDir, baseSHA := gitRepoWithRemote(t)
	writeFileT(t, workDir, "attempt.txt", "half-finished attempt")

	task := &models.Task{ID: uuid.New(), Title: "timed-out task"}
	ParkLeftoversAndReset(&models.Project{Path: workDir}, nil, "", baseSHA, task, 1)

	// Working tree is back at base and clean, ready for a retry.
	if head := runGitIn(t, workDir, "rev-parse", "HEAD"); head != baseSHA {
		t.Errorf("HEAD = %s, want reset to base %s", head, baseSHA)
	}
	if status := runGitIn(t, workDir, "status", "--porcelain"); status != "" {
		t.Errorf("working tree not clean after reset: %q", status)
	}
	if _, err := os.Stat(filepath.Join(workDir, "attempt.txt")); !os.IsNotExist(err) {
		t.Error("attempt.txt survived the reset; clean did not remove it")
	}

	// The abandoned attempt is preserved on the pushed wip branch.
	branch := "wip/task-" + task.ID.String() + "-attempt-1"
	remoteBranch := runGitIn(t, workDir, "rev-parse", "origin/"+branch)
	if remoteBranch == baseSHA || remoteBranch == "" {
		t.Fatalf("wip branch %s not pushed with the parked work (sha=%q)", branch, remoteBranch)
	}
	tree := runGitIn(t, workDir, "ls-tree", "--name-only", "origin/"+branch)
	if !strings.Contains(tree, "attempt.txt") {
		t.Errorf("wip branch does not contain the leftover file; tree=%q", tree)
	}
}

func TestParkLeftoversAndReset_NoopWhenNothingToPreserve(t *testing.T) {
	t.Parallel()
	workDir, baseSHA := gitRepoWithRemote(t)

	task := &models.Task{ID: uuid.New(), Title: "clean retry"}
	ParkLeftoversAndReset(&models.Project{Path: workDir}, nil, "", baseSHA, task, 1)

	if head := runGitIn(t, workDir, "rev-parse", "HEAD"); head != baseSHA {
		t.Errorf("HEAD = %s, want unchanged base %s", head, baseSHA)
	}
	branch := "wip/task-" + task.ID.String() + "-attempt-1"
	cmd := exec.Command("git", "rev-parse", "--verify", "origin/"+branch)
	cmd.Dir = workDir
	if err := cmd.Run(); err == nil {
		t.Errorf("wip branch %s was created for a clean tree with nothing to preserve", branch)
	}
}
