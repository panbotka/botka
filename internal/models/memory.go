package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Memory stores a piece of contextual information that persists across chat
// sessions. Memories are used to maintain long-term knowledge about the user,
// project, or conversation history.
type Memory struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the database table name for the Memory model.
func (Memory) TableName() string {
	return "memories"
}

// BeforeCreate generates a UUID primary key if one has not been explicitly set.
func (m *Memory) BeforeCreate(_ *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}
