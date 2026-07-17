package models

import "time"

// Bookmark is an app-level, global pinned link shown in the chat header.
// Bookmarks are shared across all threads (they are not per-thread); each stores
// a URL plus display metadata (page title and favicon URL) that is fetched from
// the page when the bookmark is created.
type Bookmark struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	URL        string    `gorm:"type:text;not null" json:"url"`
	Title      string    `gorm:"type:text;not null;default:''" json:"title"`
	FaviconURL string    `gorm:"type:text;not null;default:''" json:"favicon_url"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName returns the database table name for the Bookmark model.
func (Bookmark) TableName() string {
	return "bookmarks"
}
