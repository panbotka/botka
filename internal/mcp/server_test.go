package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestHandleMessage_parseError verifies that malformed JSON returns a parse error.
func TestHandleMessage_parseError(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	resp := srv.HandleMessage([]byte("{bad json"))
	if resp == nil {
		t.Fatal("expected response for parse error, got nil")
	}

	var r response
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if r.Error == nil {
		t.Fatal("expected error in response")
	}
	if r.Error.Code != codeParseError {
		t.Errorf("got error code %d, want %d", r.Error.Code, codeParseError)
	}
}

// TestHandleMessage_notification verifies that notifications (no id) return nil.
func TestHandleMessage_notification(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	msg := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	resp := srv.HandleMessage([]byte(msg))
	if resp != nil {
		t.Errorf("expected nil for notification, got %s", resp)
	}
}

// TestDispatch_initialize verifies the initialize method returns server info.
func TestDispatch_initialize(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	msg := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	resp := srv.HandleMessage([]byte(msg))
	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	var r response
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}

	result, ok := r.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}
	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("serverInfo is not a map")
	}
	if serverInfo["name"] != "botka" {
		t.Errorf("got server name %q, want %q", serverInfo["name"], "botka")
	}
}

// TestDispatch_toolsList verifies tools/list returns tool definitions.
func TestDispatch_toolsList(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	msg := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	resp := srv.HandleMessage([]byte(msg))
	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	var r response
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Error != nil {
		t.Fatalf("unexpected error: %v", r.Error)
	}

	result, ok := r.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("tools is not a slice")
	}
	if len(tools) == 0 {
		t.Error("expected at least one tool definition")
	}

	// Verify expected tool names exist.
	expectedTools := map[string]bool{
		"create_task":          false,
		"list_tasks":           false,
		"get_task":             false,
		"update_task":          false,
		"list_projects":        false,
		"get_runner_status":    false,
		"start_runner":         false,
		"update_project":       false,
		"run_command":          false,
		"list_commands":        false,
		"kill_command":         false,
		"list_threads":         false,
		"list_thread_sources":  false,
		"add_thread_source":    false,
		"remove_thread_source": false,
		"update_thread_source": false,
	}
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := toolMap["name"].(string)
		if _, exists := expectedTools[name]; exists {
			expectedTools[name] = true
		}
	}
	for name, found := range expectedTools {
		if !found {
			t.Errorf("missing tool definition: %s", name)
		}
	}
}

// TestDispatch_methodNotFound verifies unknown methods return an error.
func TestDispatch_methodNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	msg := `{"jsonrpc":"2.0","id":3,"method":"unknown/method"}`
	resp := srv.HandleMessage([]byte(msg))
	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	var r response
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if r.Error.Code != codeMethodNotFound {
		t.Errorf("got error code %d, want %d", r.Error.Code, codeMethodNotFound)
	}
}

// TestHandleToolsCall_missingParams verifies tools/call with no params returns an error.
func TestHandleToolsCall_missingParams(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	msg := `{"jsonrpc":"2.0","id":4,"method":"tools/call"}`
	resp := srv.HandleMessage([]byte(msg))
	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	var r response
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.Error == nil {
		t.Fatal("expected error for missing params")
	}
	if r.Error.Code != codeInvalidParams {
		t.Errorf("got error code %d, want %d", r.Error.Code, codeInvalidParams)
	}
}

// TestHandleToolsCall_unknownTool verifies that calling an unknown tool returns a tool error.
func TestHandleToolsCall_unknownTool(t *testing.T) {
	t.Parallel()
	srv := NewServer(nil, nil, nil)

	msg := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nonexistent","arguments":{}}}`
	resp := srv.HandleMessage([]byte(msg))
	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	var r response
	if err := json.Unmarshal(resp, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Tool errors are returned as results with isError=true, not JSON-RPC errors.
	if r.Error != nil {
		t.Fatalf("expected tool error result, got JSON-RPC error: %v", r.Error)
	}
	result, ok := r.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}
	isError, _ := result["isError"].(bool)
	if !isError {
		t.Error("expected isError=true in tool error response")
	}
}

// TestToolDefinitions_hasRequiredFields verifies all tool definitions have name, description, and schema.
func TestToolDefinitions_hasRequiredFields(t *testing.T) {
	t.Parallel()
	defs := toolDefinitions()
	if len(defs) == 0 {
		t.Fatal("expected tool definitions, got none")
	}
	for _, def := range defs {
		if def.Name == "" {
			t.Error("tool definition has empty name")
		}
		if def.Description == "" {
			t.Errorf("tool %q has empty description", def.Name)
		}
		if def.InputSchema == nil {
			t.Errorf("tool %q has nil input schema", def.Name)
		}
	}
}

// TestResultResponse_format verifies the JSON-RPC 2.0 response structure.
func TestResultResponse_format(t *testing.T) {
	t.Parallel()
	id := json.RawMessage(`1`)
	r := resultResponse(id, "hello")
	if r.JSONRPC != "2.0" {
		t.Errorf("got jsonrpc %q, want %q", r.JSONRPC, "2.0")
	}
	if r.Error != nil {
		t.Error("expected nil error")
	}
	if r.Result != "hello" {
		t.Errorf("got result %v, want %q", r.Result, "hello")
	}
}

// TestErrorResponse_format verifies the JSON-RPC 2.0 error response structure.
func TestErrorResponse_format(t *testing.T) {
	t.Parallel()
	id := json.RawMessage(`2`)
	r := errorResponse(id, codeParseError, "bad json")
	if r.JSONRPC != "2.0" {
		t.Errorf("got jsonrpc %q, want %q", r.JSONRPC, "2.0")
	}
	if r.Result != nil {
		t.Error("expected nil result")
	}
	if r.Error == nil {
		t.Fatal("expected error")
	}
	if r.Error.Code != codeParseError {
		t.Errorf("got code %d, want %d", r.Error.Code, codeParseError)
	}
	if r.Error.Message != "bad json" {
		t.Errorf("got message %q, want %q", r.Error.Message, "bad json")
	}
}

// TestMarshalResponse_success verifies marshalResponse produces valid JSON.
func TestMarshalResponse_success(t *testing.T) {
	t.Parallel()
	r := resultResponse(json.RawMessage(`1`), "ok")
	data := marshalResponse(r)
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

// TestSchemaHelpers verifies the JSON schema helper functions.
func TestSchemaHelpers(t *testing.T) {
	t.Parallel()

	t.Run("prop", func(t *testing.T) {
		t.Parallel()
		p := prop("string", "a name")
		if p["type"] != "string" {
			t.Errorf("got type %v, want string", p["type"])
		}
		if p["description"] != "a name" {
			t.Errorf("got description %v, want 'a name'", p["description"])
		}
	})

	t.Run("uuidProp", func(t *testing.T) {
		t.Parallel()
		p := uuidProp("task id")
		if p["format"] != "uuid" {
			t.Errorf("got format %v, want uuid", p["format"])
		}
	})

	t.Run("enumProp", func(t *testing.T) {
		t.Parallel()
		p := enumProp("status", "a", "b")
		vals, ok := p["enum"].([]string)
		if !ok {
			t.Fatal("enum is not []string")
		}
		if len(vals) != 2 {
			t.Errorf("got %d enum values, want 2", len(vals))
		}
	})

	t.Run("schema", func(t *testing.T) {
		t.Parallel()
		s := schema(map[string]interface{}{"name": prop("string", "n")}, "name")
		if s["type"] != "object" {
			t.Errorf("got type %v, want object", s["type"])
		}
		req, ok := s["required"].([]string)
		if !ok {
			t.Fatal("required is not []string")
		}
		if len(req) != 1 || req[0] != "name" {
			t.Errorf("got required %v, want [name]", req)
		}
	})

	t.Run("schema_no_required", func(t *testing.T) {
		t.Parallel()
		s := schema(map[string]interface{}{})
		if _, ok := s["required"]; ok {
			t.Error("expected no required field for empty schema")
		}
	})
}

// mockRunner implements RunnerController for testing.
type mockRunner struct {
	resumed bool
	startN  int
}

// Resume records that Resume was called.
func (m *mockRunner) Resume() {
	m.resumed = true
}

// StartN records the count passed to StartN.
func (m *mockRunner) StartN(n int) {
	m.startN = n
}

// TestFormatToolResult_stringResult verifies string results are wrapped in MCP content format.
func TestFormatToolResult_stringResult(t *testing.T) {
	t.Parallel()
	id := json.RawMessage(`1`)
	r := formatToolResult(id, "hello world")
	if r.Error != nil {
		t.Fatal("unexpected error")
	}
	data := marshalResponse(r)

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	result, ok := parsed["result"].(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}
	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatal("expected content array")
	}
	item, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatal("content item is not a map")
	}
	if item["text"] != "hello world" {
		t.Errorf("got text %v, want %q", item["text"], "hello world")
	}
}

// TestServe_roundTrip verifies the stdio serve loop processes messages correctly.
func TestServe_roundTrip(t *testing.T) {
	t.Parallel()

	srv := NewServer(nil, nil, nil)
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"

	r := strings.NewReader(input)
	var w strings.Builder

	err := srv.serve(r, &w)
	if err != nil {
		t.Fatalf("serve error: %v", err)
	}

	if w.Len() == 0 {
		t.Fatal("expected response output")
	}

	// The output contains the response JSON followed by a newline.
	// Trim trailing newline before parsing.
	output := strings.TrimSpace(w.String())
	var resp response
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}
