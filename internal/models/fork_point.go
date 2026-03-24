package models

import "time"

// ForkChild represents one branch option at a conversation fork point.
type ForkChild struct {
	ID        int64     `json:"id"`
	Preview   string    `json:"preview"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// ForkPoint describes a message with multiple child branches, including
// which branch is currently selected.
type ForkPoint struct {
	Children    []ForkChild `json:"children"`
	ActiveIndex int         `json:"active_index"`
}
