package models

import "time"

// SignalBridge links a Botka thread to a Signal group chat. Each thread may
// have at most one bridge (enforced by a unique index on thread_id). Bridges
// can be deactivated without being deleted via the Active flag.
type SignalBridge struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ThreadID  int64     `gorm:"not null;uniqueIndex" json:"thread_id"`
	GroupID   string    `gorm:"type:text;not null" json:"group_id"`
	GroupName string    `gorm:"type:text;not null;default:''" json:"group_name"`
	Active    bool      `gorm:"not null;default:true" json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the database table name for the SignalBridge model.
func (SignalBridge) TableName() string {
	return "signal_bridges"
}
