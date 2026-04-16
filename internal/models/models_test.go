package models

import (
	"testing"

	"github.com/google/uuid"
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
		{name: "Setting", model: &Setting{}, want: "app_settings"},
		{name: "Session", model: &Session{}, want: "sessions"},
		{name: "ThreadSource", model: &ThreadSource{}, want: "thread_sources"},
		{name: "SignalBridge", model: &SignalBridge{}, want: "signal_bridges"},
		{name: "WebAuthnCredential", model: &WebAuthnCredential{}, want: "webauthn_credentials"},
		{name: "MCPServer", model: &MCPServer{}, want: "mcp_servers"},
		{name: "ThreadMCPServer", model: &ThreadMCPServer{}, want: "thread_mcp_servers"},
		{name: "ProjectMCPServer", model: &ProjectMCPServer{}, want: "project_mcp_servers"},
		{name: "CronJob", model: &CronJob{}, want: "cron_jobs"},
		{name: "CronExecution", model: &CronExecution{}, want: "cron_executions"},
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

// TestBeforeCreate_GeneratesUUID verifies that BeforeCreate sets a non-nil UUID
// when the ID field is zero-valued.
func TestBeforeCreate_GeneratesUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		create func() (uuid.UUID, error)
	}{
		{
			name: "Project",
			create: func() (uuid.UUID, error) {
				p := &Project{}
				err := p.BeforeCreate(nil)
				return p.ID, err
			},
		},
		{
			name: "Task",
			create: func() (uuid.UUID, error) {
				tk := &Task{}
				err := tk.BeforeCreate(nil)
				return tk.ID, err
			},
		},
		{
			name: "Memory",
			create: func() (uuid.UUID, error) {
				m := &Memory{}
				err := m.BeforeCreate(nil)
				return m.ID, err
			},
		},
		{
			name: "TaskExecution",
			create: func() (uuid.UUID, error) {
				te := &TaskExecution{}
				err := te.BeforeCreate(nil)
				return te.ID, err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := tt.create()
			if err != nil {
				t.Fatalf("BeforeCreate returned error: %v", err)
			}
			if id == uuid.Nil {
				t.Error("expected non-nil UUID after BeforeCreate")
			}
		})
	}
}

// TestBeforeCreate_PreservesExistingUUID verifies that BeforeCreate does not
// overwrite an ID that was already set.
func TestBeforeCreate_PreservesExistingUUID(t *testing.T) {
	t.Parallel()

	existing := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	tests := []struct {
		name   string
		create func() (uuid.UUID, error)
	}{
		{
			name: "Project",
			create: func() (uuid.UUID, error) {
				p := &Project{ID: existing}
				err := p.BeforeCreate(nil)
				return p.ID, err
			},
		},
		{
			name: "Task",
			create: func() (uuid.UUID, error) {
				tk := &Task{ID: existing}
				err := tk.BeforeCreate(nil)
				return tk.ID, err
			},
		},
		{
			name: "Memory",
			create: func() (uuid.UUID, error) {
				m := &Memory{ID: existing}
				err := m.BeforeCreate(nil)
				return m.ID, err
			},
		},
		{
			name: "TaskExecution",
			create: func() (uuid.UUID, error) {
				te := &TaskExecution{ID: existing}
				err := te.BeforeCreate(nil)
				return te.ID, err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := tt.create()
			if err != nil {
				t.Fatalf("BeforeCreate returned error: %v", err)
			}
			if id != existing {
				t.Errorf("expected UUID %s to be preserved, got %s", existing, id)
			}
		})
	}
}

// TestAttachment_ComputeURL verifies that ComputeURL sets the URL based on StoredName.
func TestAttachment_ComputeURL(t *testing.T) {
	t.Parallel()

	a := &Attachment{StoredName: "abc123.jpg"}
	a.ComputeURL()

	want := "/api/v1/uploads/abc123.jpg"
	if a.URL != want {
		t.Errorf("ComputeURL() set URL = %q, want %q", a.URL, want)
	}
}

// TestAttachment_ComputeURL_EmptyName verifies ComputeURL with an empty stored name.
func TestAttachment_ComputeURL_EmptyName(t *testing.T) {
	t.Parallel()

	a := &Attachment{StoredName: ""}
	a.ComputeURL()

	want := "/api/v1/uploads/"
	if a.URL != want {
		t.Errorf("ComputeURL() set URL = %q, want %q", a.URL, want)
	}
}
