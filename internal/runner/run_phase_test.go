package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/config"
	"botka/internal/models"
)

// claudeStub writes an executable script that impersonates the claude CLI by
// emitting the two stream-json lines the parser needs for a successful run:
// a system/init line carrying the model, and a successful result line.
func claudeStub(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude-stub.sh")
	script := `#!/bin/sh
printf '%s\n' '{"type":"system","subtype":"init","model":"claude-stub"}'
printf '%s\n' '{"type":"result","subtype":"success","duration_ms":5,"usage":{"input_tokens":1,"output_tokens":2}}'
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatalf("write claude stub: %v", err)
	}
	return path
}

// gitRepo initializes an empty git repository with one commit so the executor's
// branch setup and push steps have something to operate on. The repo has no
// remote, so pushAndCreatePR fails fast without reaching `gh`.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
		{"commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v unavailable: %v: %s", args, err, out)
		}
	}
	return dir
}

func strPtr(s string) *string { return &s }

// TestExecute_RecordsPhaseSequence pins the order of phase transitions the
// executor announces during a successful run.
func TestExecute_RecordsPhaseSequence(t *testing.T) {
	tests := []struct {
		name           string
		branchStrategy string
		verification   *string
		want           []models.RunPhase
	}{
		{
			name:           "full pipeline",
			branchStrategy: "feature_branch",
			verification:   strPtr("true"),
			want: []models.RunPhase{
				models.RunPhasePreparing,
				models.RunPhaseAgent,
				models.RunPhaseVerifying,
				models.RunPhasePublishing,
			},
		},
		{
			name:           "no verification command, no feature branch",
			branchStrategy: "main",
			verification:   nil,
			want: []models.RunPhase{
				models.RunPhasePreparing,
				models.RunPhaseAgent,
			},
		},
		{
			name:           "empty verification command skips verifying",
			branchStrategy: "main",
			verification:   strPtr(""),
			want: []models.RunPhase{
				models.RunPhasePreparing,
				models.RunPhaseAgent,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []models.RunPhase
			e := &Executor{
				localClaudePath: claudeStub(t),
				onPhase: func(_ *models.Task, phase models.RunPhase) {
					got = append(got, phase)
				},
			}
			project := &models.Project{
				Name:                "phase-fixture",
				Path:                gitRepo(t),
				BranchStrategy:      tt.branchStrategy,
				VerificationCommand: tt.verification,
			}
			task := &models.Task{ID: uuid.New(), Title: "phase test", Spec: "spec"}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := e.Execute(ctx, task, project, NewBuffer(4096), "")
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if result.Status != models.TaskStatusDone {
				t.Fatalf("status = %q, want done", result.Status)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("phases = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("phase[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestExecute_WithoutRecorderDoesNotPanic guards the nil-recorder path used by
// the executor's own unit tests.
func TestExecute_WithoutRecorderDoesNotPanic(t *testing.T) {
	e := &Executor{localClaudePath: claudeStub(t)}
	project := &models.Project{Name: "phase-fixture", Path: gitRepo(t), BranchStrategy: "main"}
	task := &models.Task{ID: uuid.New(), Title: "phase test", Spec: "spec"}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if _, err := e.Execute(ctx, task, project, NewBuffer(4096), ""); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

// newPhaseRunner builds a Runner with just enough wiring for the phase-recording
// and finalization paths.
func newPhaseRunner(db *gorm.DB) *Runner {
	return &Runner{
		db:             db,
		config:         &config.Config{},
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		retryNotBefore: make(map[uuid.UUID]time.Time),
		TaskEvents:     NewTaskEventHub(),
	}
}

// readRunPhase returns the persisted run_phase for a task, nil when SQL NULL.
func readRunPhase(t *testing.T, db *gorm.DB, taskID uuid.UUID) *models.RunPhase {
	t.Helper()
	var task models.Task
	if err := db.First(&task, "id = ?", taskID).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	return task.RunPhase
}

func TestRecordPhase_PersistsAndPublishes(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "phase-record")
	task := createTask(t, db, proj.ID, "running task", models.TaskStatusRunning)

	r := newPhaseRunner(db)
	events, unsubscribe := r.TaskEvents.Subscribe()
	defer unsubscribe()

	r.recordPhase(&task, models.RunPhaseVerifying)

	got := readRunPhase(t, db, task.ID)
	if got == nil || *got != models.RunPhaseVerifying {
		t.Fatalf("run_phase = %v, want verifying", got)
	}
	if task.RunPhase == nil || *task.RunPhase != models.RunPhaseVerifying {
		t.Errorf("in-memory task.RunPhase = %v, want verifying", task.RunPhase)
	}

	select {
	case ev := <-events:
		if ev.TaskID != task.ID {
			t.Errorf("event task_id = %s, want %s", ev.TaskID, task.ID)
		}
		if ev.Status != models.TaskStatusRunning {
			t.Errorf("event status = %q, want running", ev.Status)
		}
		if ev.RunPhase == nil || *ev.RunPhase != models.RunPhaseVerifying {
			t.Errorf("event run_phase = %v, want verifying", ev.RunPhase)
		}
	case <-time.After(time.Second):
		t.Fatal("no task event published")
	}
}

// TestRecordPhase_IgnoresNonRunningTask covers the guard that stops a late phase
// write from resurrecting a phase on an already-finished task.
func TestRecordPhase_IgnoresNonRunningTask(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "phase-guard")
	task := createTask(t, db, proj.ID, "finished task", models.TaskStatusDone)

	r := newPhaseRunner(db)
	events, unsubscribe := r.TaskEvents.Subscribe()
	defer unsubscribe()

	r.recordPhase(&task, models.RunPhasePublishing)

	if got := readRunPhase(t, db, task.ID); got != nil {
		t.Errorf("run_phase = %v, want nil", *got)
	}
	select {
	case ev := <-events:
		t.Fatalf("unexpected event published: %+v", ev)
	default:
	}
}

// TestFinalizeTask_ClearsRunPhase asserts that every terminal status write wipes
// the phase, so a killed or crashed task never keeps a stale one.
func TestFinalizeTask_ClearsRunPhase(t *testing.T) {
	db := setupTestDB(t)

	for _, status := range []models.TaskStatus{
		models.TaskStatusDone,
		models.TaskStatusFailed,
		models.TaskStatusCancelled,
	} {
		t.Run(string(status), func(t *testing.T) {
			cleanTables(t, db)
			proj := createProject(t, db, "phase-finalize")
			task := createTask(t, db, proj.ID, "running task", models.TaskStatusRunning)

			r := newPhaseRunner(db)
			r.recordPhase(&task, models.RunPhaseAgent)
			if got := readRunPhase(t, db, task.ID); got == nil {
				t.Fatal("precondition: run_phase should be set before finalize")
			}

			r.finalizeTask(&task, &ExecutionResult{Status: status})

			if got := readRunPhase(t, db, task.ID); got != nil {
				t.Errorf("run_phase = %q after %s, want nil", *got, status)
			}
			if task.RunPhase != nil {
				t.Errorf("in-memory task.RunPhase = %v after %s, want nil", task.RunPhase, status)
			}
		})
	}
}

// TestRequeueTask_ClearsRunPhase covers the retry path, which leaves `running`
// without passing through finalizeTask.
func TestRequeueTask_ClearsRunPhase(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "phase-requeue")
	task := createTask(t, db, proj.ID, "running task", models.TaskStatusRunning)

	r := newPhaseRunner(db)
	r.recordPhase(&task, models.RunPhaseAgent)
	r.requeueTask(&task, "transient failure")

	if got := readRunPhase(t, db, task.ID); got != nil {
		t.Errorf("run_phase = %q after requeue, want nil", *got)
	}
}

// TestRecoverOrphanedTasks_ClearsRunPhase covers the restart path: a task that
// was mid-phase when the process died must come back with no phase.
func TestRecoverOrphanedTasks_ClearsRunPhase(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "phase-orphan")
	task := createTask(t, db, proj.ID, "orphaned task", models.TaskStatusRunning)

	r := newPhaseRunner(db)
	r.recordPhase(&task, models.RunPhasePublishing)
	r.recoverOrphanedTasks()

	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("status = %q, want queued", reloaded.Status)
	}
	if reloaded.RunPhase != nil {
		t.Errorf("run_phase = %q after recovery, want nil", *reloaded.RunPhase)
	}
}

func TestRunPhase_IsValid(t *testing.T) {
	valid := []models.RunPhase{
		models.RunPhasePreparing,
		models.RunPhaseAgent,
		models.RunPhaseVerifying,
		models.RunPhasePublishing,
		models.RunPhaseSummarizing,
	}
	for _, p := range valid {
		if !p.IsValid() {
			t.Errorf("%q should be valid", p)
		}
	}
	if models.RunPhase("bogus").IsValid() {
		t.Error("bogus phase should not be valid")
	}
}
