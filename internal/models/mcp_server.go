package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MCPServerType represents the transport type of an MCP server.
type MCPServerType string

const (
	MCPServerTypeStdio MCPServerType = "stdio"
	MCPServerTypeSSE   MCPServerType = "sse"
)

var validMCPServerTypes = map[MCPServerType]bool{
	MCPServerTypeStdio: true,
	MCPServerTypeSSE:   true,
}

// IsValid reports whether the MCPServerType is a recognized transport type.
func (t MCPServerType) IsValid() bool {
	return validMCPServerTypes[t]
}

// Scan implements the sql.Scanner interface for reading MCPServerType from the database.
func (t *MCPServerType) Scan(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("MCPServerType.Scan: expected string, got %T", value)
	}
	st := MCPServerType(str)
	if !st.IsValid() {
		return fmt.Errorf("MCPServerType.Scan: invalid server type %q", str)
	}
	*t = st
	return nil
}

// Value implements the driver.Valuer interface for writing MCPServerType to the database.
func (t MCPServerType) Value() (driver.Value, error) {
	if !t.IsValid() {
		return nil, fmt.Errorf("MCPServerType.Value: invalid server type %q", t)
	}
	return string(t), nil
}

// MCPServer represents an external MCP server that can be attached to threads and projects.
type MCPServer struct {
	ID         int64           `json:"id" gorm:"primaryKey"`
	Name       string          `json:"name" gorm:"uniqueIndex;not null"`
	ServerType MCPServerType   `json:"server_type" gorm:"not null"`
	Config     json.RawMessage `json:"config" gorm:"type:jsonb;not null;default:'{}'"`
	IsDefault  bool            `json:"is_default" gorm:"not null;default:false"`
	Active     bool            `json:"active" gorm:"not null;default:true"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// TableName returns the database table name for the MCPServer model.
func (MCPServer) TableName() string {
	return "mcp_servers"
}

// ThreadMCPServer is a join table linking threads to MCP servers.
type ThreadMCPServer struct {
	ThreadID    int64 `json:"thread_id" gorm:"primaryKey"`
	MCPServerID int64 `json:"mcp_server_id" gorm:"primaryKey"`
}

// TableName returns the database table name for the ThreadMCPServer model.
func (ThreadMCPServer) TableName() string {
	return "thread_mcp_servers"
}

// ProjectMCPServer is a join table linking projects to MCP servers.
type ProjectMCPServer struct {
	ProjectID   uuid.UUID `json:"project_id" gorm:"primaryKey;type:uuid"`
	MCPServerID int64     `json:"mcp_server_id" gorm:"primaryKey"`
}

// TableName returns the database table name for the ProjectMCPServer model.
func (ProjectMCPServer) TableName() string {
	return "project_mcp_servers"
}
