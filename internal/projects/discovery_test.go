package projects

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan_FindsGitRepos(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "myrepo", ".git"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 project, got %d", len(discovered))
	}
	if discovered[0].Name != "myrepo" {
		t.Errorf("Name = %q, want %q", discovered[0].Name, "myrepo")
	}
}

func TestScan_SkipsHiddenDirectories(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hidden", ".git"), 0o755)
	os.MkdirAll(filepath.Join(dir, "visible", ".git"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 project, got %d", len(discovered))
	}
	if discovered[0].Name != "visible" {
		t.Errorf("Name = %q, want %q", discovered[0].Name, "visible")
	}
}

func TestScan_SkipsRegularFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notadir.txt"), []byte("hello"), 0o644)
	os.MkdirAll(filepath.Join(dir, "repo", ".git"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 project, got %d", len(discovered))
	}
	if discovered[0].Name != "repo" {
		t.Errorf("Name = %q, want %q", discovered[0].Name, "repo")
	}
}

func TestScan_SkipsDirsWithoutGit(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "norepo"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 0 {
		t.Errorf("expected 0 projects, got %d", len(discovered))
	}
}

func TestScan_SkipsDotGitFile(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "worktree")
	os.MkdirAll(projectDir, 0o755)
	// .git is a regular file (e.g. git worktree), not a directory
	os.WriteFile(filepath.Join(projectDir, ".git"), []byte("gitdir: /somewhere"), 0o644)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 0 {
		t.Errorf("expected 0 projects when .git is a file, got %d", len(discovered))
	}
}

func TestScan_EmptyDirReturnsEmptySlice(t *testing.T) {
	dir := t.TempDir()

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 0 {
		t.Errorf("expected 0 projects, got %d", len(discovered))
	}
}

func TestScan_NonexistentDirReturnsError(t *testing.T) {
	_, err := Scan("/tmp/nonexistent-dir-botka-test-zzz")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}

func TestScan_ReturnsAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "proj", ".git"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 project, got %d", len(discovered))
	}
	if !filepath.IsAbs(discovered[0].Path) {
		t.Errorf("Path %q is not absolute", discovered[0].Path)
	}
	wantPath := filepath.Join(dir, "proj")
	if discovered[0].Path != wantPath {
		t.Errorf("Path = %q, want %q", discovered[0].Path, wantPath)
	}
}
