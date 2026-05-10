package models

import "time"

// ThreadFolder is a node in the chat sidebar's folder tree. Folders may be
// nested via ParentID (NULL means root). Sibling order is controlled by
// Position; the API layer assigns sequential positions on insert and reorder.
type ThreadFolder struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	ParentID  *int64    `json:"parent_id"`
	Position  int       `gorm:"not null;default:0" json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the database table name for the ThreadFolder model.
func (ThreadFolder) TableName() string {
	return "thread_folders"
}
