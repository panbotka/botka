package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"botka/internal/models"
)

func TestProjectRunner_LocalIsNotRemote(t *testing.T) {
	t.Parallel()
	proj := &models.Project{Path: "/home/pi/projects/app"}
	pr := newProjectRunner(proj, nil, "", "claude")
	if pr.isRemote() {
		t.Error("local project reported as remote")
	}
}

func TestProjectRunner_RemoteStripsPrefix(t *testing.T) {
	t.Parallel()
	proj := &models.Project{Path: "box:/home/box/projects/app"}
	pr := newProjectRunner(proj, nil, "box@box", "claude")
	if !pr.isRemote() {
		t.Fatal("remote project not detected")
	}
	if pr.remoteDir != "/home/box/projects/app" {
		t.Errorf("remoteDir = %q, want /home/box/projects/app", pr.remoteDir)
	}
	if pr.remote == nil || pr.remote.SSHTarget != "box@box" {
		t.Errorf("remote spec = %+v, want SSHTarget box@box", pr.remote)
	}
}

func TestProjectRunner_LocalWriteFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	proj := &models.Project{Path: dir}
	pr := newProjectRunner(proj, nil, "", "claude")
	if err := pr.writeFile(context.Background(), "nested/spec.md", []byte("body")); err != nil {
		t.Fatalf("writeFile() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "nested", "spec.md"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "body" {
		t.Errorf("content = %q, want body", string(got))
	}
}

func TestProjectRunner_WriteFileRejectsParentDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	proj := &models.Project{Path: dir}
	pr := newProjectRunner(proj, nil, "", "claude")
	err := pr.writeFile(context.Background(), "../escape.md", []byte("nope"))
	if err == nil {
		t.Error("writeFile with .. did not return error")
	}
}

func TestProjectRunner_LocalExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	proj := &models.Project{Path: dir}
	pr := newProjectRunner(proj, nil, "", "claude")
	if err := pr.exists(context.Background()); err != nil {
		t.Errorf("exists(local) error = %v", err)
	}
}

func TestProjectRunner_LocalExistsMissing(t *testing.T) {
	t.Parallel()
	proj := &models.Project{Path: "/definitely/does/not/exist/anywhere/xyz"}
	pr := newProjectRunner(proj, nil, "", "claude")
	err := pr.exists(context.Background())
	if err == nil {
		t.Error("exists(missing) error = nil, want error")
	}
}

func TestProjectRunner_LocalRunGit(t *testing.T) {
	t.Parallel()
	// Use the project's own repo root as a safe read-only target.
	proj := &models.Project{Path: "/home/pi/projects/botka"}
	pr := newProjectRunner(proj, nil, "", "claude")
	out, err := pr.runGit(context.Background(), "rev-parse", "--is-inside-work-tree")
	if err != nil {
		t.Skipf("git not available: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "true" {
		t.Errorf("git output = %q, want true", got)
	}
}

func TestBuildTaskSSHArgs_Structure(t *testing.T) {
	t.Parallel()
	args := buildTaskSSHArgs("box@box", "/home/box/app", "claude",
		[]string{"-p", "hello world"})
	if args[0] != "ssh" {
		t.Errorf("argv[0] = %q, want ssh", args[0])
	}
	last := args[len(args)-1]
	if !strings.HasPrefix(last, "cd '/home/box/app'") {
		t.Errorf("remote command should cd first, got %q", last)
	}
	if !strings.Contains(last, "exec 'claude'") {
		t.Errorf("remote command should exec claude, got %q", last)
	}
	if !strings.Contains(last, "'hello world'") {
		t.Errorf("remote command should quote the prompt value, got %q", last)
	}
	// SSH target should be the penultimate arg.
	if args[len(args)-2] != "box@box" {
		t.Errorf("penultimate arg = %q, want box@box", args[len(args)-2])
	}
}

func TestShellQuote_RunnerMatchesBehavior(t *testing.T) {
	t.Parallel()
	// Runner copy of shellQuote should produce the same output as the
	// one in internal/claude. We re-derive a couple of known cases here.
	tests := map[string]string{
		"":           "''",
		"plain":      "'plain'",
		"with space": "'with space'",
		"quote'me":   `'quote'\''me'`,
	}
	for in, want := range tests {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
