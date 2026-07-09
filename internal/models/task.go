package models

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	// TaskStatusPending indicates the task has been created but not yet queued.
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusQueued indicates the task is waiting in the scheduler queue.
	TaskStatusQueued TaskStatus = "queued"
	// TaskStatusRunning indicates the task is currently being executed.
	TaskStatusRunning TaskStatus = "running"
	// TaskStatusDone indicates the task completed successfully.
	TaskStatusDone TaskStatus = "done"
	// TaskStatusFailed indicates the task completed with an error.
	TaskStatusFailed TaskStatus = "failed"
	// TaskStatusNeedsReview indicates the task requires human review before proceeding.
	TaskStatusNeedsReview TaskStatus = "needs_review"
	// TaskStatusCancelled indicates the task was cancelled before completion.
	TaskStatusCancelled TaskStatus = "cancelled"
	// TaskStatusDeleted indicates the task was soft-deleted.
	TaskStatusDeleted TaskStatus = "deleted"
)

// validStatuses contains all valid TaskStatus values for validation.
var validStatuses = map[TaskStatus]bool{
	TaskStatusPending:     true,
	TaskStatusQueued:      true,
	TaskStatusRunning:     true,
	TaskStatusDone:        true,
	TaskStatusFailed:      true,
	TaskStatusNeedsReview: true,
	TaskStatusCancelled:   true,
	TaskStatusDeleted:     true,
}

// IsValid reports whether the TaskStatus is a recognized status value.
func (s TaskStatus) IsValid() bool {
	return validStatuses[s]
}

// Scan implements the sql.Scanner interface for reading TaskStatus from the database.
func (s *TaskStatus) Scan(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("TaskStatus.Scan: expected string, got %T", value)
	}
	status := TaskStatus(str)
	if !status.IsValid() {
		return fmt.Errorf("TaskStatus.Scan: invalid status %q", str)
	}
	*s = status
	return nil
}

// Value implements the driver.Valuer interface for writing TaskStatus to the database.
func (s TaskStatus) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("TaskStatus.Value: invalid status %q", s)
	}
	return string(s), nil
}

// RunPhase names the step of the executor pipeline a running task is currently in.
// It exists to make the wait legible: a task can sit in "running" for minutes after
// the Claude session already committed, while verification and the PR push finish.
//
// A RunPhase is only meaningful while Status is TaskStatusRunning. Every write of a
// terminal status clears it back to NULL, so a crashed or killed task never keeps a
// stale phase.
type RunPhase string

const (
	// RunPhasePreparing covers the spec sync and feature-branch setup.
	RunPhasePreparing RunPhase = "preparing"
	// RunPhaseAgent covers the Claude Code subprocess itself.
	RunPhaseAgent RunPhase = "agent"
	// RunPhaseVerifying covers the project's verification command.
	RunPhaseVerifying RunPhase = "verifying"
	// RunPhasePublishing covers pushing the feature branch and opening a PR.
	RunPhasePublishing RunPhase = "publishing"
	// RunPhaseSummarizing covers generating the failure summary for a failed task.
	RunPhaseSummarizing RunPhase = "summarizing"
)

// validRunPhases contains all valid RunPhase values for validation.
var validRunPhases = map[RunPhase]bool{
	RunPhasePreparing:   true,
	RunPhaseAgent:       true,
	RunPhaseVerifying:   true,
	RunPhasePublishing:  true,
	RunPhaseSummarizing: true,
}

// IsValid reports whether the RunPhase is a recognized phase value.
func (p RunPhase) IsValid() bool {
	return validRunPhases[p]
}

// Task represents a unit of work to be executed by the scheduler against a project.
// Tasks are ordered by priority and progress through the TaskStatus lifecycle.
type Task struct {
	ID                  uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	Title               string          `gorm:"size:500;not null" json:"title"`
	Spec                string          `gorm:"type:text;not null;default:''" json:"spec"`
	Status              TaskStatus      `gorm:"size:20;not null;default:pending" json:"status"`
	RunPhase            *RunPhase       `gorm:"type:text" json:"run_phase,omitempty"`
	Priority            int             `gorm:"not null;default:0" json:"priority"`
	ProjectID           uuid.UUID       `gorm:"type:uuid;not null" json:"project_id"`
	Project             Project         `json:"project,omitempty"`
	FailureReason       *string         `gorm:"type:text" json:"failure_reason"`
	RetryCount          int             `gorm:"not null;default:0" json:"retry_count"`
	Executions          []TaskExecution `gorm:"foreignKey:TaskID" json:"executions,omitempty"`
	StartedAt           *time.Time      `json:"started_at"`
	CompletedAt         *time.Time      `json:"completed_at"`
	InputTokens         *int64          `json:"input_tokens"`
	OutputTokens        *int64          `json:"output_tokens"`
	CacheReadTokens     *int64          `json:"cache_read_tokens"`
	CacheCreationTokens *int64          `json:"cache_creation_tokens"`
	CostUSD             *float64        `gorm:"type:numeric(12,6)" json:"cost_usd"`
	Model               *string         `gorm:"type:text" json:"model"`
	BaseCommitSHA       *string         `gorm:"type:text" json:"base_commit_sha,omitempty"`
	HeadCommitSHA       *string         `gorm:"type:text" json:"head_commit_sha,omitempty"`
	FailureSummary      *string         `gorm:"type:text" json:"failure_summary,omitempty"`
	ScheduleID          *int64          `json:"schedule_id,omitempty"`
	Schedule            *TaskSchedule   `json:"schedule,omitempty" gorm:"foreignKey:ScheduleID"`
	Tags                []TaskTag       `gorm:"many2many:task_tag_assignments;joinForeignKey:task_id;joinReferences:tag_id" json:"tags,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// TableName returns the database table name for the Task model.
func (Task) TableName() string {
	return "tasks"
}

// BeforeCreate generates a UUID primary key if one has not been explicitly set.
func (t *Task) BeforeCreate(_ *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}
