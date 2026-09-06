package models

import (
	"encoding/json"
	"time"
)

// Box auto-off outcomes. Only shutdown attempts are recorded, so these three
// cover every row.
const (
	// BoxAutoOffOutcomeShutdown means the shutdown command was accepted.
	BoxAutoOffOutcomeShutdown = "shutdown"
	// BoxAutoOffOutcomeFailed means the shutdown command failed.
	BoxAutoOffOutcomeFailed = "failed"
	// BoxAutoOffOutcomeAborted means the pre-shutdown re-check found late work
	// and called the shutdown off.
	BoxAutoOffOutcomeAborted = "aborted"
)

// BoxAutoOffEvent records one automatic shutdown attempt, with the readings
// and blockers that were current at the moment it fired.
type BoxAutoOffEvent struct {
	ID         int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	OccurredAt time.Time       `gorm:"column:occurred_at;not null" json:"occurred_at"`
	Outcome    string          `gorm:"type:text;not null" json:"outcome"`
	Detail     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"detail"`
}

// TableName returns the database table name for the BoxAutoOffEvent model.
func (BoxAutoOffEvent) TableName() string {
	return "box_auto_off_events"
}
