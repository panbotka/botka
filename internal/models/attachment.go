package models

import "time"

// Attachment represents a file attached to a chat message. Each attachment
// stores the file on disk under a generated name and records the original
// filename, MIME type, and size for display and retrieval.
type Attachment struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	MessageID    int64     `gorm:"not null" json:"message_id"`
	StoredName   string    `gorm:"size:500;not null" json:"stored_name"`
	OriginalName string    `gorm:"size:500;not null" json:"original_name"`
	MimeType     string    `gorm:"size:100;not null" json:"mime_type"`
	Size         int64     `gorm:"not null;default:0" json:"size"`
	CreatedAt    time.Time `json:"created_at"`
}

// TableName returns the database table name for the Attachment model.
func (Attachment) TableName() string {
	return "attachments"
}
