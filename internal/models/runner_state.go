package models

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// RunnerStateType represents the current state of the task runner.
type RunnerStateType string

const (
	// StateRunning indicates the runner is actively processing tasks.
	StateRunning RunnerStateType = "running"
	// StatePaused indicates the runner is paused and will not pick up new tasks.
	StatePaused RunnerStateType = "paused"
	// StateStopped indicates the runner is stopped.
	StateStopped RunnerStateType = "stopped"
)

// validRunnerStates contains all valid RunnerStateType values for validation.
var validRunnerStates = map[RunnerStateType]bool{
	StateRunning: true,
	StatePaused:  true,
	StateStopped: true,
}

// IsValid reports whether the RunnerStateType is a recognized state value.
func (s RunnerStateType) IsValid() bool {
	return validRunnerStates[s]
}

// Scan implements the sql.Scanner interface for reading RunnerStateType from the database.
func (s *RunnerStateType) Scan(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("RunnerStateType.Scan: expected string, got %T", value)
	}
	state := RunnerStateType(str)
	if !state.IsValid() {
		return fmt.Errorf("RunnerStateType.Scan: invalid state %q", str)
	}
	*s = state
	return nil
}

// Value implements the driver.Valuer interface for writing RunnerStateType to the database.
func (s RunnerStateType) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("RunnerStateType.Value: invalid state %q", s)
	}
	return string(s), nil
}

// RunnerState is a singleton row that tracks the current state of the task runner,
// including how many tasks it has completed and an optional limit on total tasks to process.
type RunnerState struct {
	ID             int             `gorm:"primaryKey;default:1" json:"id"`
	State          RunnerStateType `gorm:"type:text;not null;default:stopped" json:"state"`
	CompletedCount int             `gorm:"not null;default:0" json:"completed_count"`
	TaskLimit      *int            `json:"task_limit"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TableName returns the database table name for the RunnerState model.
func (RunnerState) TableName() string {
	return "runner_state"
}
