package models

import "time"

// Session represents an authenticated user session.
type Session struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	UserID    int64     `gorm:"not null;index" json:"user_id"`
	ExpiresAt time.Time `gorm:"not null;index" json:"expires_at"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the database table name for the Session model.
func (Session) TableName() string {
	return "sessions"
}
