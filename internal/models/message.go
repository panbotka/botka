package models

import "time"

// Message represents a single chat message within a thread. Messages support
// branching via ParentID for conversation forks, and may include AI thinking
// metadata and token usage statistics.
type Message struct {
	ID                 int64        `gorm:"primaryKey;autoIncrement" json:"id"`
	ThreadID           int64        `gorm:"not null" json:"thread_id"`
	Role               string       `gorm:"size:20;not null" json:"role"`
	Content            string       `gorm:"type:text;not null;default:''" json:"content"`
	ParentID           *int64       `json:"parent_id"`
	Thinking           *string      `gorm:"type:text" json:"thinking,omitempty"`
	ThinkingDurationMs *int         `json:"thinking_duration_ms,omitempty"`
	PromptTokens       *int         `json:"prompt_tokens,omitempty"`
	CompletionTokens   *int         `json:"completion_tokens,omitempty"`
	CostUSD            *float64     `gorm:"type:numeric(10,6)" json:"cost_usd,omitempty"`
	Attachments        []Attachment `gorm:"foreignKey:MessageID" json:"attachments,omitempty"`
	CreatedAt          time.Time    `json:"created_at"`
}

// TableName returns the database table name for the Message model.
func (Message) TableName() string {
	return "messages"
}
