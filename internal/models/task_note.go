package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaskNote is a free-form, timestamped observation attached to a task. Notes
// support edit and soft-delete; they are human-only metadata and are never
// included in the context handed to Claude when the task runs.
type TaskNote struct {
	ID        int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"task_id"`
	Body      string         `gorm:"type:text;not null" json:"body"`
	Author    string         `gorm:"type:text;not null;default:user" json:"author"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the database table name for the TaskNote model.
func (TaskNote) TableName() string {
	return "task_notes"
}
