# Thread & Project MCP Server Assignment API

Add API endpoints for toggling which MCP servers are enabled per thread and per project. This uses the `thread_mcp_servers` and `project_mcp_servers` join tables.

## Context

MCP servers have an `is_default` flag. Default servers are automatically active everywhere. Non-default servers must be explicitly enabled per thread or project. The join tables store explicit assignments — presence of a row means "enabled for this thread/project".

## Requirements

### Thread MCP Endpoints

Add to the existing thread handler file or create a dedicated handler section.

**GET /api/v1/threads/:id/mcp-servers**
- Return all MCP servers with their enablement status for this thread
- Response: `{"data": [{"id": 1, "name": "photo-sorter", "server_type": "sse", "is_default": true, "active": true, "enabled": true}, ...]}` 
- The `enabled` field is true if: the server is_default=true OR it has a row in thread_mcp_servers for this thread
- Only include servers where `active=true`
- Order by name

**PUT /api/v1/threads/:id/mcp-servers**
- Set which non-default servers are explicitly enabled for this thread
- Request body: `{"mcp_server_ids": [1, 3, 5]}`
- Replace all rows in thread_mcp_servers for this thread (delete existing, insert new)
- Only IDs of non-default servers should be stored — default servers don't need explicit rows
- Response: `{"data": [...]}` (same format as GET, showing updated state)
- Return 404 if thread not found

### Project MCP Endpoints

Same pattern, on the project resource. Add to existing project handler or create a new section.

**GET /api/v1/projects/:id/mcp-servers**
- Same as thread version but for projects, using project_mcp_servers table
- Response format identical, with `enabled` reflecting project assignment

**PUT /api/v1/projects/:id/mcp-servers**
- Same as thread version but for projects
- Request body: `{"mcp_server_ids": [1, 3, 5]}`
- Replace all rows in project_mcp_servers for this project
- Response: updated list with enabled status

### Resolution Helper

Create a helper function (e.g., in `internal/models/mcp_server.go` or a new `internal/mcp/resolve.go`) that resolves which MCP servers are active for a given context:

```go
func ResolveMCPServers(db *gorm.DB, threadID *int64, projectID *uuid.UUID) ([]MCPServer, error)
```

Logic:
1. Start with all servers where `active=true AND is_default=true`
2. If threadID is provided, add servers from thread_mcp_servers for that thread (where active=true)
3. If projectID is provided, add servers from project_mcp_servers for that project (where active=true)
4. Deduplicate by server ID
5. Return the merged list

This function will be used by the MCP config generation task (next task in sequence).

## Testing

Write integration tests covering:
- GET thread mcp-servers: empty (only defaults shown), with assignments
- PUT thread mcp-servers: set servers, replace servers, clear all
- GET/PUT project mcp-servers: same patterns
- ResolveMCPServers: default-only, thread override, project override, combined thread+project, deduplication
- Edge cases: deleted server removes from assignments (CASCADE), inactive server excluded
