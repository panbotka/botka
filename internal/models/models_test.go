package models

import (
	"testing"
)

// TestTableNames verifies that all models return their expected table names.
func TestTableNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model interface{ TableName() string }
		want  string
	}{
		{name: "Project", model: &Project{}, want: "projects"},
		{name: "Task", model: &Task{}, want: "tasks"},
		{name: "TaskExecution", model: &TaskExecution{}, want: "task_executions"},
		{name: "RunnerState", model: &RunnerState{}, want: "runner_state"},
		{name: "Persona", model: &Persona{}, want: "personas"},
		{name: "Thread", model: &Thread{}, want: "threads"},
		{name: "Message", model: &Message{}, want: "messages"},
		{name: "Attachment", model: &Attachment{}, want: "attachments"},
		{name: "BranchSelection", model: &BranchSelection{}, want: "branch_selections"},
		{name: "Tag", model: &Tag{}, want: "tags"},
		{name: "Memory", model: &Memory{}, want: "memories"},
		{name: "User", model: &User{}, want: "users"},
		{name: "ThreadAccess", model: &ThreadAccess{}, want: "thread_access"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.model.TableName(); got != tt.want {
				t.Errorf("%s.TableName() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
