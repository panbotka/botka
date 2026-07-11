package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

// runGitIn runs a git command in dir and returns its trimmed combined output.
func runGitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// dirtyGitRepo builds a repo with a clean base commit, then dirties it the way a
// failed attempt would: one extra commit (tracked partial work) plus an
// untracked file. It returns the working directory and the base commit SHA.
func dirtyGitRepo(t *testing.T) (dir, baseSHA string) {
	t.Helper()
	dir = gitRepo(t)
	baseSHA = runGitIn(t, dir, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(dir, "partial.txt"), []byte("wip"), 0o600); err != nil {
		t.Fatalf("write partial.txt: %v", err)
	}
	runGitIn(t, dir, "add", "-A")
	runGitIn(t, dir, "commit", "-m", "attempt 1 partial work")

	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("junk"), 0o600); err != nil {
		t.Fatalf("write untracked.txt: %v", err)
	}
	return dir, baseSHA
}

// createProjectAt creates a project row whose Path points at an existing repo,
// so the runner's git helpers operate on a real working tree.
func createProjectAt(t *testing.T, db *gorm.DB, name, path string) models.Project {
	t.Helper()
	p := models.Project{Name: name, Path: path, BranchStrategy: "main", Active: true}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p
}

// (a) A retried task that finally succeeds must not keep the failure_reason from
// the attempt it superseded, on either a done or a needs_review terminal status.
func TestFinalizeTask_ClearsFailureReasonOnSuccess(t *testing.T) {
	db := setupTestDB(t)

	for _, status := range []models.TaskStatus{models.TaskStatusDone, models.TaskStatusNeedsReview} {
		t.Run(string(status), func(t *testing.T) {
			cleanTables(t, db)
			proj := createProject(t, db, "retry-clear")
			task := createTask(t, db, proj.ID, "retried task", models.TaskStatusRunning)

			reason := "execution timed out"
			db.Model(&task).Updates(map[string]interface{}{"failure_reason": reason, "retry_count": 1})
			task.FailureReason = &reason

			r := newPhaseRunner(db)
			r.finalizeTask(&task, &ExecutionResult{Status: status})

			var reloaded models.Task
			if err := db.First(&reloaded, task.ID).Error; err != nil {
				t.Fatalf("reload task: %v", err)
			}
			if reloaded.FailureReason != nil {
				t.Errorf("persisted failure_reason = %q, want nil", *reloaded.FailureReason)
			}
			if task.FailureReason != nil {
				t.Errorf("in-memory task.FailureReason = %q, want nil", *task.FailureReason)
			}
		})
	}
}

// A genuine failure must still record its error — the clear only applies to
// successful terminal statuses.
func TestFinalizeTask_KeepsFailureReasonOnFailure(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "retry-keep")
	task := createTask(t, db, proj.ID, "failing task", models.TaskStatusRunning)

	r := newPhaseRunner(db)
	r.finalizeTask(&task, &ExecutionResult{Status: models.TaskStatusFailed, ErrorMessage: "boom"})

	var reloaded models.Task
	if err := db.First(&reloaded, task.ID).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.FailureReason == nil || *reloaded.FailureReason != "boom" {
		t.Errorf("persisted failure_reason = %v, want %q", reloaded.FailureReason, "boom")
	}
}

// (b) Claiming a task for a retry must advance started_at so the displayed
// duration measures the current attempt, not the work plus a superseded one.
func TestClaimTask_AdvancesStartedAtOnRelaunch(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "retry-started-at")
	task := createTask(t, db, proj.ID, "relaunched task", models.TaskStatusQueued)

	old := time.Now().Add(-40 * time.Minute)
	db.Model(&task).Updates(map[string]interface{}{"started_at": old, "retry_count": 1})
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}

	r := newPhaseRunner(db)
	tx := db.Begin()
	defer tx.Rollback() //nolint:errcheck // no-op after claimTask commits

	claimed, exec, err := r.claimTask(tx, &task)
	if err != nil {
		t.Fatalf("claimTask: %v", err)
	}

	if claimed.StartedAt == nil || !claimed.StartedAt.After(old) {
		t.Errorf("in-memory started_at = %v, want advanced past %v", claimed.StartedAt, old)
	}
	if exec.Attempt != 2 {
		t.Errorf("execution attempt = %d, want 2", exec.Attempt)
	}

	var reloaded models.Task
	if err := db.First(&reloaded, task.ID).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.StartedAt == nil || !reloaded.StartedAt.After(old) {
		t.Errorf("persisted started_at = %v, want advanced past %v", reloaded.StartedAt, old)
	}
}

// (c) The retry path resets the working tree to the task's base commit, so
// attempt 2 starts from the same state attempt 1 did.
func TestRequeueTask_ResetsWorkingTreeToBase(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	dir, base := dirtyGitRepo(t)
	proj := createProjectAt(t, db, "retry-reset", dir)
	task := createTask(t, db, proj.ID, "timed-out task", models.TaskStatusRunning)
	db.Model(&task).Update("base_commit_sha", base)
	task.BaseCommitSHA = &base
	task.Project = proj

	r := &Runner{
		db:             db,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		retryNotBefore: make(map[uuid.UUID]time.Time),
		TaskEvents:     NewTaskEventHub(),
		executor:       &Executor{},
	}
	r.requeueTask(&task, "execution timed out")

	if head := runGitIn(t, dir, "rev-parse", "HEAD"); head != base {
		t.Errorf("HEAD = %s after requeue, want base %s", head, base)
	}
	if status := runGitIn(t, dir, "status", "--porcelain"); status != "" {
		t.Errorf("working tree not clean after requeue: %q", status)
	}
	if _, err := os.Stat(filepath.Join(dir, "partial.txt")); !os.IsNotExist(err) {
		t.Error("partial.txt should have been reset away")
	}
	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); !os.IsNotExist(err) {
		t.Error("untracked.txt should have been cleaned away")
	}
}

// (c) The non-retry (finalize) path is the terminal safety net: it commits any
// work the agent left uncommitted so a permanently-failed or done task never
// ends with a dirty working tree. Push is best-effort — dirtyGitRepo has no
// remote, so only the local commit is asserted here.
func TestFinalizeTask_CommitsLeftoverWork(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	dir, base := dirtyGitRepo(t)
	headBefore := runGitIn(t, dir, "rev-parse", "HEAD")
	if headBefore == base {
		t.Fatal("precondition: dirty repo HEAD should be ahead of base")
	}

	proj := createProjectAt(t, db, "finalize-commit", dir)
	task := createTask(t, db, proj.ID, "dead task", models.TaskStatusRunning)
	db.Model(&task).Update("base_commit_sha", base)
	task.BaseCommitSHA = &base
	task.Project = proj

	r := &Runner{
		db:             db,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		retryNotBefore: make(map[uuid.UUID]time.Time),
		TaskEvents:     NewTaskEventHub(),
		executor:       &Executor{},
	}
	r.finalizeTask(&task, &ExecutionResult{Status: models.TaskStatusFailed, ErrorMessage: "boom"})

	if status := runGitIn(t, dir, "status", "--porcelain"); status != "" {
		t.Errorf("working tree not clean after finalize: %q (leftovers must be committed)", status)
	}
	if head := runGitIn(t, dir, "rev-parse", "HEAD"); head == headBefore {
		t.Error("HEAD did not advance; leftover work was not committed")
	}
	if tracked := runGitIn(t, dir, "ls-files", "untracked.txt"); tracked == "" {
		t.Error("untracked.txt was not committed by the finalize safety net")
	}
}

// resetTreeToBase must be a safe no-op when base_commit_sha was never captured,
// rather than resetting to a guessed commit.
func TestResetTreeToBase_SkipsWithoutBaseCommit(t *testing.T) {
	dir, _ := dirtyGitRepo(t)
	headBefore := runGitIn(t, dir, "rev-parse", "HEAD")

	proj := models.Project{Name: "no-base", Path: dir, BranchStrategy: "main"}
	task := models.Task{ID: uuid.New(), Project: proj} // BaseCommitSHA nil

	r := &Runner{executor: &Executor{}}
	r.resetTreeToBase(&task)

	if head := runGitIn(t, dir, "rev-parse", "HEAD"); head != headBefore {
		t.Errorf("HEAD = %s, want unchanged %s (missing base must skip the reset)", head, headBefore)
	}
	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); err != nil {
		t.Error("untracked.txt should survive when the reset is skipped")
	}
}
