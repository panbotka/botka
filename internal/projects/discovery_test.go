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

func TestScan_MultipleRepos(t *testing.T) {
	dir := t.TempDir()
	repos := []string{"alpha", "beta", "gamma", "delta"}
	for _, name := range repos {
		os.MkdirAll(filepath.Join(dir, name, ".git"), 0o755)
	}

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != len(repos) {
		t.Fatalf("expected %d projects, got %d", len(repos), len(discovered))
	}

	// Verify all repos are found
	names := make(map[string]bool)
	for _, d := range discovered {
		names[d.Name] = true
	}
	for _, name := range repos {
		if !names[name] {
			t.Errorf("expected to find project %q", name)
		}
	}
}

func TestScan_SkipsNestedGitRepos(t *testing.T) {
	dir := t.TempDir()
	// Create a top-level repo with a nested git repo inside
	os.MkdirAll(filepath.Join(dir, "parent", ".git"), 0o755)
	os.MkdirAll(filepath.Join(dir, "parent", "child", ".git"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	// Scan only checks top-level entries, so only "parent" is found
	if len(discovered) != 1 {
		t.Fatalf("expected 1 project (top-level only), got %d", len(discovered))
	}
	if discovered[0].Name != "parent" {
		t.Errorf("Name = %q, want %q", discovered[0].Name, "parent")
	}
}

func TestScan_MixedContent(t *testing.T) {
	dir := t.TempDir()

	// Git repo
	os.MkdirAll(filepath.Join(dir, "repo1", ".git"), 0o755)
	// Regular directory (no .git)
	os.MkdirAll(filepath.Join(dir, "notagitrepo"), 0o755)
	// Hidden directory with .git
	os.MkdirAll(filepath.Join(dir, ".secretrepo", ".git"), 0o755)
	// Regular file
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644)
	// Directory with .git as a file (worktree)
	os.MkdirAll(filepath.Join(dir, "worktree"), 0o755)
	os.WriteFile(filepath.Join(dir, "worktree", ".git"), []byte("gitdir: /elsewhere"), 0o644)
	// Another git repo
	os.MkdirAll(filepath.Join(dir, "repo2", ".git"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(discovered))
	}

	names := make(map[string]bool)
	for _, d := range discovered {
		names[d.Name] = true
	}
	if !names["repo1"] {
		t.Error("expected repo1 to be discovered")
	}
	if !names["repo2"] {
		t.Error("expected repo2 to be discovered")
	}
}

func TestScan_NameMatchesDirName(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "my-cool-project", ".git"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 project, got %d", len(discovered))
	}
	if discovered[0].Name != "my-cool-project" {
		t.Errorf("Name = %q, want %q", discovered[0].Name, "my-cool-project")
	}
}

func TestScan_SkipsMultipleHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hiddenDirs := []string{".config", ".local", ".cache", ".git"}
	for _, name := range hiddenDirs {
		os.MkdirAll(filepath.Join(dir, name, ".git"), 0o755)
	}
	os.MkdirAll(filepath.Join(dir, "real-project", ".git"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 project (skipping hidden), got %d", len(discovered))
	}
	if discovered[0].Name != "real-project" {
		t.Errorf("Name = %q, want %q", discovered[0].Name, "real-project")
	}
}

func TestScan_EmptyGitDir(t *testing.T) {
	// A .git directory that's empty should still be detected
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "empty-git", ".git"), 0o755)

	discovered, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 project, got %d", len(discovered))
	}
}
