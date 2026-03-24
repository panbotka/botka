package models

import (
	"testing"
)

// TestTaskStatus_IsValid verifies that all defined task statuses are recognized
// as valid and that arbitrary strings are rejected.
func TestTaskStatus_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status TaskStatus
		want   bool
	}{
		{name: "pending is valid", status: TaskStatusPending, want: true},
		{name: "queued is valid", status: TaskStatusQueued, want: true},
		{name: "running is valid", status: TaskStatusRunning, want: true},
		{name: "done is valid", status: TaskStatusDone, want: true},
		{name: "failed is valid", status: TaskStatusFailed, want: true},
		{name: "needs_review is valid", status: TaskStatusNeedsReview, want: true},
		{name: "cancelled is valid", status: TaskStatusCancelled, want: true},
		{name: "empty string is invalid", status: TaskStatus(""), want: false},
		{name: "arbitrary string is invalid", status: TaskStatus("bogus"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("TaskStatus(%q).IsValid() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

// TestTaskStatus_Scan verifies that scanning database values into TaskStatus
// works for valid strings and returns errors for invalid inputs.
func TestTaskStatus_Scan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		want    TaskStatus
		wantErr bool
	}{
		{name: "valid pending", input: "pending", want: TaskStatusPending, wantErr: false},
		{name: "valid done", input: "done", want: TaskStatusDone, wantErr: false},
		{name: "invalid string", input: "bogus", want: TaskStatus(""), wantErr: true},
		{name: "non-string type", input: 42, want: TaskStatus(""), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var s TaskStatus
			err := s.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Scan(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err == nil && s != tt.want {
				t.Errorf("Scan(%v) = %q, want %q", tt.input, s, tt.want)
			}
		})
	}
}

// TestTaskStatus_Value verifies that TaskStatus produces a valid driver.Value
// for recognized statuses and returns an error for invalid ones.
func TestTaskStatus_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  TaskStatus
		want    string
		wantErr bool
	}{
		{name: "pending produces string", status: TaskStatusPending, want: "pending", wantErr: false},
		{name: "running produces string", status: TaskStatusRunning, want: "running", wantErr: false},
		{name: "invalid produces error", status: TaskStatus("bogus"), want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val, err := tt.status.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				str, ok := val.(string)
				if !ok {
					t.Errorf("Value() returned %T, want string", val)
				} else if str != tt.want {
					t.Errorf("Value() = %q, want %q", str, tt.want)
				}
			}
		})
	}
}

// TestTask_TableName verifies that Task returns the correct table name.
func TestTask_TableName(t *testing.T) {
	t.Parallel()
	task := Task{}
	if got := task.TableName(); got != "tasks" {
		t.Errorf("Task.TableName() = %q, want %q", got, "tasks")
	}
}
