package models

import (
	"time"

	"github.com/google/uuid"
)

// Thread represents a chat conversation session. Each thread may be associated with
// a persona for customized behavior and a project for workspace context. Threads
// support pinning, archiving, and tagging for organization.
type Thread struct {
	ID              int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Title           string     `gorm:"size:500;not null;default:New Chat" json:"title"`
	Model           *string    `gorm:"size:100" json:"model"`
	SystemPrompt    string     `gorm:"type:text;not null;default:''" json:"system_prompt"`
	PersonaID       *int64     `json:"persona_id"`
	Persona         *Persona   `gorm:"foreignKey:PersonaID" json:"persona,omitempty"`
	PersonaName     string     `gorm:"size:255;not null;default:''" json:"persona_name"`
	ProjectID       *uuid.UUID `gorm:"type:uuid" json:"project_id"`
	Project         *Project   `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Pinned          bool       `gorm:"not null;default:false" json:"pinned"`
	Archived        bool       `gorm:"not null;default:false" json:"archived"`
	ClaudeSessionID *string    `gorm:"size:100" json:"claude_session_id"`
	Tags            []Tag      `gorm:"many2many:thread_tags" json:"tags,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TableName returns the database table name for the Thread model.
func (Thread) TableName() string {
	return "threads"
}
