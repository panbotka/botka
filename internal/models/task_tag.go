package models

import (
	"time"

	"github.com/google/uuid"
)

// TaskTag is a colored label that can be attached to one or more tasks for
// visual categorization (bug, feature, refactor, ...). Unlike Tag (used for
// chat threads), TaskTag is keyed solely by tasks via TaskTagAssignment.
type TaskTag struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"size:50;not null" json:"name"`
	Color     string    `gorm:"size:7;not null;default:#6B7280" json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName returns the database table name for the TaskTag model.
func (TaskTag) TableName() string {
	return "task_tags"
}

// TaskTagAssignment is the join row linking a task to a task tag.
type TaskTagAssignment struct {
	TaskID uuid.UUID `gorm:"type:uuid;primaryKey" json:"task_id"`
	TagID  int64     `gorm:"primaryKey" json:"tag_id"`
}

// TableName returns the database table name for the join model.
func (TaskTagAssignment) TableName() string {
	return "task_tag_assignments"
}
