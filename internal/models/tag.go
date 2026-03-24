package models

import "time"

// Tag represents a label that can be applied to threads for categorization
// and filtering. Each tag has a unique name and a hex color for display.
type Tag struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"size:100;not null;uniqueIndex" json:"name"`
	Color     string    `gorm:"size:7;not null;default:#6b7280" json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName returns the database table name for the Tag model.
func (Tag) TableName() string {
	return "tags"
}
