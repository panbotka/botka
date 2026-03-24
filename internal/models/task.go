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

// Task represents a unit of work to be executed by the scheduler against a project.
// Tasks are ordered by priority and progress through the TaskStatus lifecycle.
type Task struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	Title         string          `gorm:"size:500;not null" json:"title"`
	Spec          string          `gorm:"type:text;not null;default:''" json:"spec"`
	Status        TaskStatus      `gorm:"size:20;not null;default:pending" json:"status"`
	Priority      int             `gorm:"not null;default:0" json:"priority"`
	ProjectID     uuid.UUID       `gorm:"type:uuid;not null" json:"project_id"`
	Project       Project         `json:"project,omitempty"`
	FailureReason *string         `gorm:"type:text" json:"failure_reason"`
	RetryCount    int             `gorm:"not null;default:0" json:"retry_count"`
	Executions    []TaskExecution `gorm:"foreignKey:TaskID" json:"executions,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
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
