package models

import (
	"time"

	"github.com/google/uuid"
)

// CronJob defines a scheduled prompt that runs on a cron schedule
// in a project context. Each job produces CronExecution records.
type CronJob struct {
	ID             int64      `json:"id" gorm:"primaryKey"`
	Name           string     `json:"name" gorm:"not null"`
	Schedule       string     `json:"schedule" gorm:"not null"`
	Prompt         string     `json:"prompt" gorm:"not null"`
	ProjectID      uuid.UUID  `json:"project_id" gorm:"type:uuid;not null"`
	Project        *Project   `json:"project,omitempty"`
	Enabled        bool       `json:"enabled" gorm:"not null;default:true"`
	TimeoutMinutes int        `json:"timeout_minutes" gorm:"not null;default:30"`
	Model          *string    `json:"model"`
	LastRunAt      *time.Time `json:"last_run_at"`
	LastStatus     *string    `json:"last_status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName returns the database table name for the CronJob model.
func (CronJob) TableName() string {
	return "cron_jobs"
}

// CronExecution records a single execution of a cron job.
type CronExecution struct {
	ID           int64      `json:"id" gorm:"primaryKey"`
	CronJobID    int64      `json:"cron_job_id" gorm:"not null"`
	CronJob      *CronJob   `json:"cron_job,omitempty"`
	Status       string     `json:"status" gorm:"not null;default:running"`
	Output       *string    `json:"output"`
	ErrorMessage *string    `json:"error_message"`
	CostUSD      float64    `json:"cost_usd"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	DurationMs   int        `json:"duration_ms"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

// TableName returns the database table name for the CronExecution model.
func (CronExecution) TableName() string {
	return "cron_executions"
}
