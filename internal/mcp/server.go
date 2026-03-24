// Package mcp implements a Model Context Protocol server using JSON-RPC 2.0.
// It supports both stdio and SSE transports, exposing task management tools.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"gorm.io/gorm"
)

var newline = []byte{'\n'}

// JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// request is a JSON-RPC 2.0 request or notification.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// response is a JSON-RPC 2.0 response.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolDef describes an MCP tool for the tools/list response.
type toolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// toolHandler processes a tools/call invocation and returns a result or error.
type toolHandler func(args json.RawMessage) (interface{}, error)

// RunnerController provides runner control operations for MCP tools.
type RunnerController interface {
	Resume()
	StartN(n int)
}

// Server handles MCP protocol messages. It is transport-agnostic;
// use RunStdio for stdio or SSEHandler for HTTP/SSE.
type Server struct {
	db     *gorm.DB
	runner RunnerController // nil in stdio mode
}

// NewServer creates a new MCP server backed by the given database.
// The runner parameter may be nil (e.g. in stdio mode) — runner control tools
// will return an error in that case.
func NewServer(db *gorm.DB, runner RunnerController) *Server {
	return &Server{db: db, runner: runner}
}

// RunStdio reads JSON-RPC 2.0 messages from stdin and writes responses
// to stdout. It blocks until stdin is closed or a read error occurs.
func RunStdio(db *gorm.DB) error {
	return NewServer(db, nil).serve(os.Stdin, os.Stdout)
}

// HandleMessage processes a single JSON-RPC 2.0 message and returns
// the serialized JSON response. Returns nil for notifications.
func (s *Server) HandleMessage(data []byte) []byte {
	var req request
	if err := json.Unmarshal(data, &req); err != nil {
		return marshalResponse(errorResponse(nil, codeParseError, "parse error"))
	}

	// Notifications have no id and receive no response.
	if req.ID == nil {
		s.handleNotification(&req)
		return nil
	}

	return marshalResponse(s.dispatch(&req))
}

// serve reads newline-delimited JSON-RPC messages from r and writes responses to w.
func (s *Server) serve(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	slog.Info("mcp server started")

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		resp := s.HandleMessage(line)
		if resp != nil {
			_, _ = w.Write(resp)    //nolint:errcheck // best-effort write
			_, _ = w.Write(newline) //nolint:errcheck // best-effort write
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin: %w", err)
	}

	slog.Info("mcp server stopped")
	return nil
}

// handleNotification processes notifications (requests without an id).
func (s *Server) handleNotification(req *request) {
	switch req.Method {
	case "notifications/initialized":
		slog.Info("client initialized")
	default:
		slog.Info("unknown notification", "method", req.Method)
	}
}

// dispatch routes a JSON-RPC request to the appropriate handler.
func (s *Server) dispatch(req *request) *response {
	switch req.Method {
	case "initialize":
		return resultResponse(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "botka", "version": "1.0.0"},
		})
	case "tools/list":
		return resultResponse(req.ID, map[string]interface{}{
			"tools": toolDefinitions(),
		})
	case "tools/call":
		return s.handleToolsCall(req)
	default:
		return errorResponse(req.ID, codeMethodNotFound, "method not found: "+req.Method)
	}
}

// handleToolsCall dispatches a tools/call request to the matching tool handler.
func (s *Server) handleToolsCall(req *request) *response {
	if len(req.Params) == 0 {
		return errorResponse(req.ID, codeInvalidParams, "missing params")
	}

	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, codeInvalidParams, "invalid params")
	}
	if params.Name == "" {
		return errorResponse(req.ID, codeInvalidParams, "missing tool name")
	}

	handlers := s.toolHandlers()
	handler, ok := handlers[params.Name]
	if !ok {
		return toolErrorResponse(req.ID, "unknown tool: "+params.Name)
	}

	result, err := handler(params.Arguments)
	if err != nil {
		return toolErrorResponse(req.ID, err.Error())
	}

	return formatToolResult(req.ID, result)
}

// formatToolResult wraps a tool result into the MCP content response format.
func formatToolResult(id json.RawMessage, result interface{}) *response {
	var text string
	switch v := result.(type) {
	case string:
		text = v
	default:
		resultJSON, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return toolErrorResponse(id, "failed to marshal result")
		}
		text = string(resultJSON)
	}

	return resultResponse(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	})
}

// toolHandlers returns the mapping of tool names to their handler functions.
func (s *Server) toolHandlers() map[string]toolHandler {
	return map[string]toolHandler{
		"create_task":       s.handleCreateTask,
		"list_tasks":        s.handleListTasks,
		"get_task":          s.handleGetTask,
		"update_task":       s.handleUpdateTask,
		"list_projects":     s.handleListProjects,
		"get_runner_status": s.handleGetRunnerStatus,
		"start_runner":      s.handleStartRunner,
	}
}

// resultResponse builds a JSON-RPC 2.0 success response.
func resultResponse(id json.RawMessage, result interface{}) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}

// errorResponse builds a JSON-RPC 2.0 error response.
func errorResponse(id json.RawMessage, code int, message string) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

// toolErrorResponse builds a tools/call error result per MCP protocol.
func toolErrorResponse(id json.RawMessage, message string) *response {
	return resultResponse(id, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": "Error: " + message},
		},
		"isError": true,
	})
}

// marshalResponse serializes a response to JSON. Returns nil on error.
func marshalResponse(resp *response) []byte {
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("failed to marshal response", "error", err)
		return nil
	}
	return data
}

// prop builds a JSON Schema property with type and description.
func prop(typ, desc string) map[string]interface{} {
	return map[string]interface{}{"type": typ, "description": desc}
}

// uuidProp builds a JSON Schema string property with uuid format.
func uuidProp(desc string) map[string]interface{} {
	return map[string]interface{}{
		"type": "string", "format": "uuid", "description": desc,
	}
}

// enumProp builds a JSON Schema string property with allowed values.
func enumProp(desc string, values ...string) map[string]interface{} {
	return map[string]interface{}{
		"type": "string", "enum": values, "description": desc,
	}
}

// schema builds a JSON Schema object with the given properties and required fields.
func schema(props map[string]interface{}, required ...string) map[string]interface{} {
	s := map[string]interface{}{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// toolDefinitions returns the MCP tool definitions for the tools/list response.
func toolDefinitions() []toolDef {
	defs := taskToolDefinitions()
	return append(defs, runnerToolDefinitions()...)
}

// taskToolDefinitions returns tool definitions for task management operations.
func taskToolDefinitions() []toolDef {
	allStatuses := []string{
		"pending", "queued", "running", "done",
		"failed", "needs_review", "cancelled",
	}
	return []toolDef{
		{
			Name:        "create_task",
			Description: "Create a new task to be executed by the scheduler",
			InputSchema: schema(map[string]interface{}{
				"title":        prop("string", "Short title for the task"),
				"project_name": prop("string", "Name of the target project (case-insensitive)"),
				"spec":         prop("string", "Detailed spec or prompt for Claude"),
				"priority":     prop("integer", "Higher values run first (default 0)"),
				"status":       enumProp("Initial status", "pending", "queued"),
			}, "title", "project_name", "spec"),
		},
		{
			Name:        "list_tasks",
			Description: "List tasks with optional filtering by status or project",
			InputSchema: schema(map[string]interface{}{
				"status":       enumProp("Filter by status", allStatuses...),
				"project_name": prop("string", "Filter by project name (case-insensitive)"),
				"limit":        prop("integer", "Max results (default 20)"),
			}),
		},
		{
			Name:        "get_task",
			Description: "Get detailed information about a task",
			InputSchema: schema(map[string]interface{}{
				"task_id": uuidProp("UUID of the task"),
			}, "task_id"),
		},
		{
			Name:        "update_task",
			Description: "Update a task's title, spec, priority, or status",
			InputSchema: schema(map[string]interface{}{
				"task_id":  uuidProp("UUID of the task to update"),
				"title":    prop("string", "New title"),
				"spec":     prop("string", "New specification"),
				"priority": prop("integer", "New priority value"),
				"status": enumProp(
					"New status (must be a valid transition)",
					"pending", "queued", "cancelled", "done",
				),
			}, "task_id"),
		},
		{
			Name:        "list_projects",
			Description: "List all active projects for task scheduling",
			InputSchema: schema(map[string]interface{}{}),
		},
	}
}

// runnerToolDefinitions returns tool definitions for runner control operations.
func runnerToolDefinitions() []toolDef {
	return []toolDef{
		{
			Name:        "get_runner_status",
			Description: "Get scheduler status with running tasks and usage",
			InputSchema: schema(map[string]interface{}{}),
		},
		{
			Name:        "start_runner",
			Description: "Start the task runner. Optionally set a count to auto-stop after that many tasks complete.",
			InputSchema: schema(map[string]interface{}{
				"count": prop("integer", "Number of tasks to process before auto-stopping (0 or omit for unlimited)"),
			}),
		},
	}
}
