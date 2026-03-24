package models

import "time"

// Persona defines a reusable chat personality with a system prompt, default model,
// and optional starter message. Personas can be assigned to threads to customize
// the assistant's behavior.
type Persona struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string    `gorm:"size:255;not null" json:"name"`
	SystemPrompt   string    `gorm:"type:text;not null;default:''" json:"system_prompt"`
	DefaultModel   *string   `gorm:"size:100" json:"default_model"`
	Icon           *string   `gorm:"size:10" json:"icon"`
	StarterMessage *string   `gorm:"type:text" json:"starter_message"`
	SortOrder      int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName returns the database table name for the Persona model.
func (Persona) TableName() string {
	return "personas"
}
