package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaskExecution records a single execution attempt for a task.
// Each task may have multiple executions if it is retried after failure.
type TaskExecution struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID       uuid.UUID  `gorm:"type:uuid;not null" json:"task_id"`
	Task         Task       `json:"task,omitempty"`
	Attempt      int        `gorm:"not null;default:1" json:"attempt"`
	StartedAt    time.Time  `gorm:"not null" json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	ExitCode     *int       `json:"exit_code"`
	CostUSD      *float64   `gorm:"type:numeric(10,6)" json:"cost_usd"`
	DurationMs   *int64     `json:"duration_ms"`
	Summary      *string    `gorm:"type:text" json:"summary"`
	ErrorMessage *string    `gorm:"type:text" json:"error_message"`
	RawOutput    *string    `gorm:"type:text" json:"raw_output,omitempty"`
	GitHeadSHA   *string    `gorm:"type:text" json:"git_head_sha,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TableName returns the database table name for the TaskExecution model.
func (TaskExecution) TableName() string {
	return "task_executions"
}

// BeforeCreate generates a UUID primary key if one has not been explicitly set.
func (te *TaskExecution) BeforeCreate(_ *gorm.DB) error {
	if te.ID == uuid.Nil {
		te.ID = uuid.New()
	}
	return nil
}
