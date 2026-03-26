package models

import "time"

// ThreadSource represents a URL source attached to a chat thread.
// Sources are fetched during context assembly and their content is
// included in the system prompt sent to Claude.
type ThreadSource struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ThreadID  int64     `gorm:"not null" json:"thread_id"`
	URL       string    `gorm:"type:text;not null" json:"url"`
	Label     string    `gorm:"type:text;not null;default:''" json:"label"`
	Position  int       `gorm:"not null;default:0" json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the database table name for the ThreadSource model.
func (ThreadSource) TableName() string {
	return "thread_sources"
}
