package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

// initTestRepo initializes a git repo in dir with two commits and returns the
// SHA of each commit.
func initTestRepo(t *testing.T, dir string) (baseSHA, headSHA string) {
	t.Helper()

	// init repo
	runGit := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %s", args, string(out))
		}
		return out
	}

	runGit("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package foo\n\nfunc Foo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "foo.go")
	runGit("commit", "-m", "initial")
	baseSHA = strings.TrimSpace(string(runGit("rev-parse", "HEAD")))

	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package foo\n\nfunc Foo() {\n\tprintln(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.go"), []byte("package foo\n\nfunc Bar() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "foo.go", "bar.go")
	runGit("commit", "-m", "changes")
	headSHA = strings.TrimSpace(string(runGit("rev-parse", "HEAD")))
	return baseSHA, headSHA
}

func createDiffTaskWithProject(t *testing.T, db *gorm.DB, projectPath string, baseSHA, headSHA *string) models.Task {
	t.Helper()
	p := models.Project{
		Name:           "diff-test-project",
		Path:           projectPath,
		BranchStrategy: "main",
		Active:         true,
	}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	task := models.Task{
		Title:         "diff test",
		Spec:          "spec",
		ProjectID:     p.ID,
		Status:        models.TaskStatusDone,
		BaseCommitSHA: baseSHA,
		HeadCommitSHA: headSHA,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task
}

func TestTaskDiff_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	dir := t.TempDir()
	baseSHA, headSHA := initTestRepo(t, dir)
	task := createDiffTaskWithProject(t, db, dir, &baseSHA, &headSHA)

	router := taskRouter(db)
	w := doRequest(router, "GET", "/api/v1/tasks/"+task.ID.String()+"/diff", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			BaseCommitSHA string `json:"base_commit_sha"`
			HeadCommitSHA string `json:"head_commit_sha"`
			Files         []struct {
				Path      string `json:"path"`
				Status    string `json:"status"`
				Additions int    `json:"additions"`
				Deletions int    `json:"deletions"`
			} `json:"files"`
			Diff string `json:"diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.BaseCommitSHA != baseSHA || resp.Data.HeadCommitSHA != headSHA {
		t.Errorf("unexpected SHAs: %+v", resp.Data)
	}
	if len(resp.Data.Files) != 2 {
		t.Fatalf("expected 2 files, got %d: %+v", len(resp.Data.Files), resp.Data.Files)
	}

	statuses := map[string]string{}
	for _, f := range resp.Data.Files {
		statuses[f.Path] = f.Status
	}
	if statuses["foo.go"] != "modified" {
		t.Errorf("foo.go status: got %q, want modified", statuses["foo.go"])
	}
	if statuses["bar.go"] != "added" {
		t.Errorf("bar.go status: got %q, want added", statuses["bar.go"])
	}
	if !strings.Contains(resp.Data.Diff, "foo.go") {
		t.Errorf("raw diff missing foo.go content")
	}
}

func TestTaskDiff_MissingSHAs(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	dir := t.TempDir()
	task := createDiffTaskWithProject(t, db, dir, nil, nil)

	router := taskRouter(db)
	w := doRequest(router, "GET", "/api/v1/tasks/"+task.ID.String()+"/diff", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskDiff_EqualSHAs(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	dir := t.TempDir()
	sha := "abc123"
	task := createDiffTaskWithProject(t, db, dir, &sha, &sha)

	router := taskRouter(db)
	w := doRequest(router, "GET", "/api/v1/tasks/"+task.ID.String()+"/diff", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskDiff_TaskNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	router := taskRouter(db)
	w := doRequest(router, "GET", "/api/v1/tasks/"+uuid.New().String()+"/diff", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTaskDiff_InvalidUUID(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	router := taskRouter(db)
	w := doRequest(router, "GET", "/api/v1/tasks/not-a-uuid/diff", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
