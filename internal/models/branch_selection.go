package models

// BranchSelection records which child message is currently selected at a
// conversation fork point within a thread. This allows the UI to display
// the correct branch when multiple replies exist for a given message.
type BranchSelection struct {
	ID              int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ThreadID        int64 `gorm:"not null;uniqueIndex:idx_branch_thread_fork" json:"thread_id"`
	ForkMessageID   int64 `gorm:"not null;default:0;uniqueIndex:idx_branch_thread_fork" json:"fork_message_id"`
	SelectedChildID int64 `gorm:"not null" json:"selected_child_id"`
}

// TableName returns the database table name for the BranchSelection model.
func (BranchSelection) TableName() string {
	return "branch_selections"
}
