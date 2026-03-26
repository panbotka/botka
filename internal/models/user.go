package models

import "time"

// UserRole represents the access level of a user.
type UserRole string

const (
	// RoleAdmin has full access to all features.
	RoleAdmin UserRole = "admin"
	// RoleExternal has restricted access to assigned threads only.
	RoleExternal UserRole = "external"
)

// User represents an authenticated user account.
type User struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	Role         UserRole  `gorm:"not null;default:admin" json:"role"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName returns the database table name for the User model.
func (User) TableName() string {
	return "users"
}

// IsAdmin returns true if the user has admin role.
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// ThreadAccess represents an external user's access to a specific thread.
type ThreadAccess struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    int64     `gorm:"not null;uniqueIndex:idx_user_thread" json:"user_id"`
	ThreadID  int64     `gorm:"not null;uniqueIndex:idx_user_thread" json:"thread_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the database table name for the ThreadAccess model.
func (ThreadAccess) TableName() string {
	return "thread_access"
}
