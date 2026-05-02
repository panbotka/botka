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
	"botka/internal/runner"
)

// initTestRepo initializes a git repo in dir with two commits and returns the
// SHA of each commit. The second commit modifies foo.go and adds bar.go so
// callers have a deterministic two-file diff to assert against.
func initTestRepo(t *testing.T, dir string) (baseSHA, headSHA string) {
	t.Helper()

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

// diffResponse is the parsed shape of the /tasks/:id/diff response envelope.
type diffResponse struct {
	Data struct {
		Diff  string `json:"diff"`
		Stats struct {
			FilesChanged int `json:"files_changed"`
			Insertions   int `json:"insertions"`
			Deletions    int `json:"deletions"`
		} `json:"stats"`
		Truncated bool `json:"truncated"`
	} `json:"data"`
}

func decodeDiff(t *testing.T, body []byte) diffResponse {
	t.Helper()
	var resp diffResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp
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

	resp := decodeDiff(t, w.Body.Bytes())
	if resp.Data.Stats.FilesChanged != 2 {
		t.Errorf("files_changed: got %d, want 2", resp.Data.Stats.FilesChanged)
	}
	if resp.Data.Stats.Insertions == 0 {
		t.Errorf("expected non-zero insertions, got %+v", resp.Data.Stats)
	}
	if resp.Data.Truncated {
		t.Errorf("did not expect truncated=true for small diff")
	}
	if !strings.Contains(resp.Data.Diff, "foo.go") {
		t.Errorf("raw diff missing foo.go content: %s", resp.Data.Diff)
	}
	if !strings.Contains(resp.Data.Diff, "bar.go") {
		t.Errorf("raw diff missing bar.go content: %s", resp.Data.Diff)
	}
}

func TestTaskDiff_MissingSHAs(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	dir := t.TempDir()
	task := createDiffTaskWithProject(t, db, dir, nil, nil)

	router := taskRouter(db)
	w := doRequest(router, "GET", "/api/v1/tasks/"+task.ID.String()+"/diff", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeDiff(t, w.Body.Bytes())
	if resp.Data.Diff != "" {
		t.Errorf("expected empty diff, got %q", resp.Data.Diff)
	}
	if resp.Data.Stats.FilesChanged != 0 {
		t.Errorf("expected zero files_changed, got %d", resp.Data.Stats.FilesChanged)
	}
	if resp.Data.Truncated {
		t.Errorf("did not expect truncated=true for empty diff")
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
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeDiff(t, w.Body.Bytes())
	if resp.Data.Diff != "" || resp.Data.Stats.FilesChanged != 0 {
		t.Errorf("expected empty result for equal SHAs, got %+v", resp.Data)
	}
}

func TestTaskDiff_MissingCommit(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	dir := t.TempDir()
	_, headSHA := initTestRepo(t, dir)
	bogus := "0000000000000000000000000000000000000000"
	task := createDiffTaskWithProject(t, db, dir, &bogus, &headSHA)

	router := taskRouter(db)
	w := doRequest(router, "GET", "/api/v1/tasks/"+task.ID.String()+"/diff", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "commit not found in repository") {
		t.Errorf("expected 'commit not found in repository' error, got: %s", w.Body.String())
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

func TestTaskDiff_TruncationFlag(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Build a repo whose second commit adds a single file with > MaxDiffBytes
	// of content, so the raw diff exceeds the cap and the handler must set
	// truncated=true while keeping the prefix intact.
	dir := t.TempDir()
	bigContent := strings.Repeat("xxxxxxxx\n", (runner.MaxDiffBytes/9)+1024)
	if len(bigContent) <= runner.MaxDiffBytes {
		t.Fatalf("test setup error: payload smaller than cap (%d <= %d)", len(bigContent), runner.MaxDiffBytes)
	}

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", args, string(out))
		}
	}
	runGit("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "seed.txt")
	runGit("commit", "-m", "initial")
	baseOut, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse base: %v: %s", err, string(baseOut))
	}
	baseSHA := strings.TrimSpace(string(baseOut))

	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(bigContent), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "big.txt")
	runGit("commit", "-m", "big")
	headOut, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse head: %v: %s", err, string(headOut))
	}
	headSHA := strings.TrimSpace(string(headOut))

	task := createDiffTaskWithProject(t, db, dir, &baseSHA, &headSHA)
	router := taskRouter(db)
	w := doRequest(router, "GET", "/api/v1/tasks/"+task.ID.String()+"/diff", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeDiff(t, w.Body.Bytes())
	if !resp.Data.Truncated {
		t.Errorf("expected truncated=true for over-sized diff")
	}
	if len(resp.Data.Diff) > runner.MaxDiffBytes {
		t.Errorf("diff length %d exceeds cap %d", len(resp.Data.Diff), runner.MaxDiffBytes)
	}
	if len(resp.Data.Diff) < runner.MaxDiffBytes-1024 {
		t.Errorf("diff length %d unexpectedly short of cap %d", len(resp.Data.Diff), runner.MaxDiffBytes)
	}
}
