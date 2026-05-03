package models

import "gorm.io/gorm"

// BranchSelection records which child message is currently selected at a
// conversation fork point within a thread. This allows the UI to display
// the correct branch when multiple replies exist for a given message.
type BranchSelection struct {
	ID              int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	ThreadID        int64          `gorm:"not null" json:"thread_id"`
	ForkMessageID   int64          `gorm:"not null;default:0" json:"fork_message_id"`
	SelectedChildID int64          `gorm:"not null" json:"selected_child_id"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName returns the database table name for the BranchSelection model.
func (BranchSelection) TableName() string {
	return "branch_selections"
}
