// Package models defines GORM structs for all database tables.
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Project represents a unified workspace directory that serves dual roles:
// a git repository for task scheduling (from Saiduler) and a chat workspace
// with claude_md context (from Chatovadlo). Each project maps to a directory
// on disk and may have associated tasks and chat threads.
type Project struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name                string    `gorm:"size:255;not null" json:"name"`
	Path                string    `gorm:"size:1024;uniqueIndex;not null" json:"path"`
	BranchStrategy      string    `gorm:"size:20;not null;default:main" json:"branch_strategy"`
	VerificationCommand *string   `gorm:"type:text" json:"verification_command"`
	DevCommand          *string   `gorm:"type:text" json:"dev_command"`
	DeployCommand       *string   `gorm:"type:text" json:"deploy_command"`
	Active              bool      `gorm:"not null;default:true" json:"active"`
	ClaudeMD            string    `gorm:"column:claude_md;type:text;not null;default:''" json:"claude_md"`
	SortOrder           int       `gorm:"not null;default:0" json:"sort_order"`
	Tasks               []Task    `gorm:"foreignKey:ProjectID" json:"tasks,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// TableName returns the database table name for the Project model.
func (Project) TableName() string {
	return "projects"
}

// BeforeCreate generates a UUID primary key if one has not been explicitly set.
func (p *Project) BeforeCreate(_ *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
