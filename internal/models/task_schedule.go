package models

import (
	"time"

	"github.com/google/uuid"
)

// TaskSchedule defines a cron-based recurring task generator. The runner
// scans enabled schedules every minute and creates a Task in pending status
// when next_run_at is due, then advances next_run_at to the next firing time
// according to cron_expression.
type TaskSchedule struct {
	ID             int64      `json:"id" gorm:"primaryKey"`
	ProjectID      uuid.UUID  `json:"project_id" gorm:"type:uuid;not null"`
	Project        *Project   `json:"project,omitempty"`
	Title          string     `json:"title" gorm:"not null"`
	Spec           string     `json:"spec" gorm:"type:text;not null;default:''"`
	CronExpression string     `json:"cron_expression" gorm:"not null"`
	Priority       int        `json:"priority" gorm:"not null;default:0"`
	Enabled        bool       `json:"enabled" gorm:"not null;default:true"`
	LastRunAt      *time.Time `json:"last_run_at"`
	NextRunAt      *time.Time `json:"next_run_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName returns the database table name for TaskSchedule.
func (TaskSchedule) TableName() string {
	return "task_schedules"
}
