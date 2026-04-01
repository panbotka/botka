package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"botka/internal/models"
)

// TestCreateTaskArgs_validate verifies validation for create_task arguments.
func TestCreateTaskArgs_validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       createTaskArgs
		wantStatus models.TaskStatus
		wantErr    bool
	}{
		{
			name:       "valid with defaults",
			args:       createTaskArgs{Title: "t", ProjectName: "p", Spec: "s"},
			wantStatus: models.TaskStatusPending,
		},
		{
			name:       "valid with pending status",
			args:       createTaskArgs{Title: "t", ProjectName: "p", Spec: "s", Status: "pending"},
			wantStatus: models.TaskStatusPending,
		},
		{
			name:       "valid with queued status",
			args:       createTaskArgs{Title: "t", ProjectName: "p", Spec: "s", Status: "queued"},
			wantStatus: models.TaskStatusQueued,
		},
		{
			name:    "missing title",
			args:    createTaskArgs{ProjectName: "p", Spec: "s"},
			wantErr: true,
		},
		{
			name:    "missing project_name",
			args:    createTaskArgs{Title: "t", Spec: "s"},
			wantErr: true,
		},
		{
			name:    "missing spec",
			args:    createTaskArgs{Title: "t", ProjectName: "p"},
			wantErr: true,
		},
		{
			name:    "invalid status",
			args:    createTaskArgs{Title: "t", ProjectName: "p", Spec: "s", Status: "running"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, err := tt.args.validate()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("got status %q, want %q", status, tt.wantStatus)
			}
		})
	}
}

// TestUpdateTaskArgs_buildUpdates verifies update map construction and transition validation.
func TestUpdateTaskArgs_buildUpdates(t *testing.T) {
	t.Parallel()

	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	tests := []struct {
		name        string
		args        updateTaskArgs
		current     models.TaskStatus
		wantChanged []string
		wantErr     bool
	}{
		{
			name:        "update title only",
			args:        updateTaskArgs{Title: strPtr("new title")},
			current:     models.TaskStatusPending,
			wantChanged: []string{"title"},
		},
		{
			name:        "update multiple fields",
			args:        updateTaskArgs{Title: strPtr("t"), Spec: strPtr("s"), Priority: intPtr(5)},
			current:     models.TaskStatusPending,
			wantChanged: []string{"title", "spec", "priority"},
		},
		{
			name:        "valid status transition pending to queued",
			args:        updateTaskArgs{Status: strPtr("queued")},
			current:     models.TaskStatusPending,
			wantChanged: []string{"status"},
		},
		{
			name:        "valid status transition failed to queued",
			args:        updateTaskArgs{Status: strPtr("queued")},
			current:     models.TaskStatusFailed,
			wantChanged: []string{"status"},
		},
		{
			name:    "invalid status transition pending to done",
			args:    updateTaskArgs{Status: strPtr("done")},
			current: models.TaskStatusPending,
			wantErr: true,
		},
		{
			name:    "invalid status transition done to queued",
			args:    updateTaskArgs{Status: strPtr("queued")},
			current: models.TaskStatusDone,
			wantErr: true,
		},
		{
			name:        "no changes",
			args:        updateTaskArgs{},
			current:     models.TaskStatusPending,
			wantChanged: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			updates, changed, err := tt.args.buildUpdates(tt.current)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(changed) != len(tt.wantChanged) {
				t.Errorf("got changed %v, want %v", changed, tt.wantChanged)
				return
			}
			for i, c := range changed {
				if c != tt.wantChanged[i] {
					t.Errorf("changed[%d] = %q, want %q", i, c, tt.wantChanged[i])
				}
			}
			if tt.wantChanged == nil && len(updates) != 0 {
				t.Errorf("expected empty updates, got %v", updates)
			}
		})
	}
}

// TestFormatTaskDetail verifies task detail formatting includes key fields.
func TestFormatTaskDetail(t *testing.T) {
	t.Parallel()

	task := &models.Task{
		Title:    "test task",
		Status:   models.TaskStatusQueued,
		Priority: 5,
		Spec:     "do the thing",
		Project:  models.Project{Name: "myproject"},
	}

	result := formatTaskDetail(task)

	checks := []string{"test task", "queued", "5", "myproject", "do the thing"}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("formatTaskDetail missing %q in output:\n%s", check, result)
		}
	}
}

// TestFormatTaskDetail_withFailureReason verifies failure reason is included.
func TestFormatTaskDetail_withFailureReason(t *testing.T) {
	t.Parallel()

	reason := "something went wrong"
	task := &models.Task{
		Title:         "failed task",
		Status:        models.TaskStatusFailed,
		Spec:          "spec",
		FailureReason: &reason,
		Project:       models.Project{Name: "proj"},
	}

	result := formatTaskDetail(task)
	if !strings.Contains(result, "something went wrong") {
		t.Error("expected failure reason in output")
	}
}

// TestFormatExecution verifies execution formatting output.
func TestFormatExecution(t *testing.T) {
	t.Parallel()

	exitCode := 0
	durationMs := int64(5000)
	costUSD := 0.1234
	summary := "done well"

	exec := &models.TaskExecution{
		Attempt:    1,
		ExitCode:   &exitCode,
		DurationMs: &durationMs,
		CostUSD:    &costUSD,
		Summary:    &summary,
	}

	var b strings.Builder
	formatExecution(&b, exec)
	output := b.String()

	checks := []string{"Attempt 1", "exit_code=0", "duration=5000ms", "cost=$0.1234", "done well"}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("formatExecution missing %q in output:\n%s", check, output)
		}
	}
}

// TestHandleStartRunner_noRunner verifies start_runner fails in stdio mode.
func TestHandleStartRunner_noRunner(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleStartRunner(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error when runner is nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestHandleStartRunner_unlimited verifies start_runner calls Resume with no count.
func TestHandleStartRunner_unlimited(t *testing.T) {
	t.Parallel()
	runner := &mockRunner{}
	srv := NewServer(nil, runner, nil)

	result, err := srv.handleStartRunner(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !runner.resumed {
		t.Error("expected Resume to be called")
	}
	if !strings.Contains(result.(string), "unlimited") {
		t.Errorf("unexpected result: %v", result)
	}
}

// TestHandleStartRunner_withCount verifies start_runner calls StartN with the given count.
func TestHandleStartRunner_withCount(t *testing.T) {
	t.Parallel()
	runner := &mockRunner{}
	srv := NewServer(nil, runner, nil)

	result, err := srv.handleStartRunner(json.RawMessage(`{"count": 5}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.startN != 5 {
		t.Errorf("expected StartN(5), got StartN(%d)", runner.startN)
	}
	if !strings.Contains(result.(string), "5 tasks") {
		t.Errorf("unexpected result: %v", result)
	}
}

// TestAllowedTransitions verifies the transition map has expected entries.
func TestAllowedTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from models.TaskStatus
		to   models.TaskStatus
		want bool
	}{
		{"pending to queued", models.TaskStatusPending, models.TaskStatusQueued, true},
		{"pending to cancelled", models.TaskStatusPending, models.TaskStatusCancelled, true},
		{"pending to done", models.TaskStatusPending, models.TaskStatusDone, false},
		{"queued to pending", models.TaskStatusQueued, models.TaskStatusPending, true},
		{"queued to cancelled", models.TaskStatusQueued, models.TaskStatusCancelled, true},
		{"failed to queued", models.TaskStatusFailed, models.TaskStatusQueued, true},
		{"failed to cancelled", models.TaskStatusFailed, models.TaskStatusCancelled, true},
		{"failed to done", models.TaskStatusFailed, models.TaskStatusDone, false},
		{"needs_review to queued", models.TaskStatusNeedsReview, models.TaskStatusQueued, true},
		{"needs_review to done", models.TaskStatusNeedsReview, models.TaskStatusDone, true},
		{"done to queued", models.TaskStatusDone, models.TaskStatusQueued, false},
		{"running to queued", models.TaskStatusRunning, models.TaskStatusQueued, false},
		{"deleted to pending", models.TaskStatusDeleted, models.TaskStatusPending, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			allowed := allowedTransitions[tt.from]
			got := allowed[tt.to]
			if got != tt.want {
				t.Errorf("transition %s → %s: got %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

// TestCreateTaskArgs_validateErrorMessages verifies exact error messages from validation.
func TestCreateTaskArgs_validateErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    createTaskArgs
		wantErr string
		wantSt  models.TaskStatus
	}{
		{"empty title", createTaskArgs{ProjectName: "p", Spec: "s"}, "title is required", ""},
		{"empty project", createTaskArgs{Title: "t", Spec: "s"}, "project_name is required", ""},
		{"empty spec", createTaskArgs{Title: "t", ProjectName: "p"}, "spec is required", ""},
		{"default status", createTaskArgs{Title: "t", ProjectName: "p", Spec: "s"}, "", models.TaskStatusPending},
		{"explicit pending", createTaskArgs{Title: "t", ProjectName: "p", Spec: "s", Status: "pending"}, "", models.TaskStatusPending},
		{"explicit queued", createTaskArgs{Title: "t", ProjectName: "p", Spec: "s", Status: "queued"}, "", models.TaskStatusQueued},
		{"invalid status", createTaskArgs{Title: "t", ProjectName: "p", Spec: "s", Status: "running"}, "status must be pending or queued", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st, err := tt.args.validate()
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Errorf("validate() error = %v, want %q", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if st != tt.wantSt {
					t.Errorf("status = %s, want %s", st, tt.wantSt)
				}
			}
		})
	}
}

// TestUpdateTaskArgs_buildUpdatesFieldCounts verifies the number of update entries
// and changed field names for various update combinations.
func TestUpdateTaskArgs_buildUpdatesFieldCounts(t *testing.T) {
	t.Parallel()

	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	tests := []struct {
		name       string
		args       updateTaskArgs
		current    models.TaskStatus
		wantCount  int
		wantFields []string
		wantErr    bool
	}{
		{"title only", updateTaskArgs{Title: strPtr("new title")}, models.TaskStatusPending, 1, []string{"title"}, false},
		{"spec only", updateTaskArgs{Spec: strPtr("new spec")}, models.TaskStatusPending, 1, []string{"spec"}, false},
		{"priority only", updateTaskArgs{Priority: intPtr(5)}, models.TaskStatusPending, 1, []string{"priority"}, false},
		{"valid status transition", updateTaskArgs{Status: strPtr("queued")}, models.TaskStatusPending, 1, []string{"status"}, false},
		{"invalid transition from done", updateTaskArgs{Status: strPtr("queued")}, models.TaskStatusDone, 0, nil, true},
		{"all fields", updateTaskArgs{Title: strPtr("new title"), Spec: strPtr("new spec"), Priority: intPtr(5)}, models.TaskStatusPending, 3, []string{"title", "spec", "priority"}, false},
		{"no changes", updateTaskArgs{}, models.TaskStatusPending, 0, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			updates, changed, err := tt.args.buildUpdates(tt.current)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(updates) != tt.wantCount {
				t.Errorf("got %d updates, want %d", len(updates), tt.wantCount)
			}
			if len(changed) != len(tt.wantFields) {
				t.Errorf("got changed %v, want %v", changed, tt.wantFields)
			}
		})
	}
}

// TestFormatTaskDetail_withExecutions verifies that executions are included in task detail formatting.
func TestFormatTaskDetail_withExecutions(t *testing.T) {
	t.Parallel()

	exitCode := 0
	duration := int64(5000)
	cost := 0.05
	summary := "All tests pass"
	task := &models.Task{
		Title:   "Test Task",
		Status:  models.TaskStatusDone,
		Project: models.Project{Name: "test"},
		Executions: []models.TaskExecution{
			{Attempt: 1, ExitCode: &exitCode, DurationMs: &duration, CostUSD: &cost, Summary: &summary},
		},
	}
	result := formatTaskDetail(task)
	checks := []string{"Test Task", "Attempt 1", "exit_code=0", "All tests pass"}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("formatTaskDetail missing %q in output:\n%s", check, result)
		}
	}
}

// TestHandleKillTask_noRunner verifies kill_task fails in stdio mode.
func TestHandleKillTask_noRunner(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	_, err := srv.handleKillTask(json.RawMessage(`{"task_id":"` + uuid.New().String() + `"}`))
	if err == nil {
		t.Fatal("expected error when runner is nil")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestHandleKillTask_missingTaskID verifies kill_task requires task_id.
func TestHandleKillTask_missingTaskID(t *testing.T) {
	t.Parallel()
	runner := &mockRunner{}
	srv := NewServer(nil, runner, nil)

	_, err := srv.handleKillTask(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing task_id")
	}
	if !strings.Contains(err.Error(), "task_id is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleKillTask_invalidUUID verifies kill_task rejects invalid UUIDs.
func TestHandleKillTask_invalidUUID(t *testing.T) {
	t.Parallel()
	runner := &mockRunner{}
	srv := NewServer(nil, runner, nil)

	_, err := srv.handleKillTask(json.RawMessage(`{"task_id":"not-a-uuid"}`))
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}
	if !strings.Contains(err.Error(), "invalid task_id") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestHandleKillTask_success verifies kill_task calls KillTask on the runner.
func TestHandleKillTask_success(t *testing.T) {
	t.Parallel()
	runner := &mockRunner{}
	srv := NewServer(nil, runner, nil)
	taskID := uuid.New()

	result, err := srv.handleKillTask(json.RawMessage(`{"task_id":"` + taskID.String() + `"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runner.killedTask != taskID {
		t.Errorf("expected KillTask(%s), got KillTask(%s)", taskID, runner.killedTask)
	}
	if !strings.Contains(result.(string), "Kill initiated") {
		t.Errorf("unexpected result: %v", result)
	}
}

// TestHandleKillTask_notRunning verifies kill_task returns error when task is not running.
func TestHandleKillTask_notRunning(t *testing.T) {
	t.Parallel()
	taskID := uuid.New()
	runner := &mockRunner{killErr: fmt.Errorf("task %s is not currently running", taskID)}
	srv := NewServer(nil, runner, nil)

	_, err := srv.handleKillTask(json.RawMessage(`{"task_id":"` + taskID.String() + `"}`))
	if err == nil {
		t.Fatal("expected error for non-running task")
	}
	if !strings.Contains(err.Error(), "not currently running") {
		t.Errorf("unexpected error: %v", err)
	}
}
