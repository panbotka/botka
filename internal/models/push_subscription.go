package models

import "time"

// PushSubscription represents a Web Push subscription registered by a user's
// browser. Each row corresponds to a single browser/device pair; a user may
// have multiple subscriptions.
type PushSubscription struct {
	ID         int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     int64      `gorm:"not null;uniqueIndex:idx_push_user_endpoint" json:"user_id"`
	Endpoint   string     `gorm:"type:text;not null;uniqueIndex:idx_push_user_endpoint" json:"endpoint"`
	P256dh     string     `gorm:"type:text;not null" json:"p256dh"`
	Auth       string     `gorm:"type:text;not null" json:"auth"`
	UserAgent  string     `gorm:"type:text" json:"user_agent,omitempty"`
	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// TableName returns the database table name for the PushSubscription model.
func (PushSubscription) TableName() string {
	return "push_subscriptions"
}
