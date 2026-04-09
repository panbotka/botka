package signal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockRPCServer is a tiny httptest.Server preconfigured with a JSON-RPC
// handler that routes on the incoming method name.
type mockRPCServer struct {
	t       *testing.T
	handler func(method string, params json.RawMessage, id int64) (any, *jsonRPCError)
	server  *httptest.Server
}

// newMockRPCServer constructs a mockRPCServer. The caller must Close it.
func newMockRPCServer(t *testing.T, handler func(method string, params json.RawMessage, id int64) (any, *jsonRPCError)) *mockRPCServer {
	t.Helper()
	m := &mockRPCServer{t: t, handler: handler}
	m.server = httptest.NewServer(http.HandlerFunc(m.serve))
	return m
}

// serve implements http.Handler for the JSON-RPC endpoint.
func (m *mockRPCServer) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != rpcPath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		http.Error(w, "bad content type", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
		ID      int64           `json:"id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, rpcErr := m.handler(req.Method, req.Params, req.ID)
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
	}
	if rpcErr != nil {
		resp["error"] = rpcErr
	} else {
		resp["result"] = result
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Close shuts down the mock server.
func (m *mockRPCServer) Close() { m.server.Close() }

// URL returns the base URL of the mock server.
func (m *mockRPCServer) URL() string { return m.server.URL }

// --- NewClient ---

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	t.Parallel()
	c := NewClient("http://127.0.0.1:5107/")
	if c.baseURL != "http://127.0.0.1:5107" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://127.0.0.1:5107")
	}
}

func TestNewClient_CustomHTTPClient(t *testing.T) {
	t.Parallel()
	custom := &http.Client{Timeout: 5 * time.Second}
	c := NewClient("http://127.0.0.1:5107", WithHTTPClient(custom))
	if c.http != custom {
		t.Errorf("WithHTTPClient did not replace http client")
	}
}

// --- ListGroups ---

func TestListGroups_Success(t *testing.T) {
	t.Parallel()
	server := newMockRPCServer(t, func(method string, params json.RawMessage, id int64) (any, *jsonRPCError) {
		if method != "listGroups" {
			return nil, &jsonRPCError{Code: -32601, Message: "unexpected method " + method}
		}
		return []SignalGroup{
			{
				ID:   "abc123",
				Name: "Test Group",
				Members: []Member{
					{Number: "+1111", UUID: "uuid-1"},
					{Number: "+2222", UUID: "uuid-2"},
				},
			},
		}, nil
	})
	defer server.Close()

	c := NewClient(server.URL())
	groups, err := c.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("len(groups) = %d, want 1", len(groups))
	}
	if groups[0].ID != "abc123" || groups[0].Name != "Test Group" {
		t.Errorf("unexpected group %+v", groups[0])
	}
	if got := groups[0].MemberCount(); got != 2 {
		t.Errorf("MemberCount() = %d, want 2", got)
	}
}

func TestListGroups_RPCError(t *testing.T) {
	t.Parallel()
	server := newMockRPCServer(t, func(method string, params json.RawMessage, id int64) (any, *jsonRPCError) {
		return nil, &jsonRPCError{Code: -1, Message: "boom"}
	})
	defer server.Close()

	c := NewClient(server.URL())
	_, err := c.ListGroups(context.Background())
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *RPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != -1 || rpcErr.Message != "boom" {
		t.Errorf("unexpected rpc error: code=%d msg=%q", rpcErr.Code, rpcErr.Message)
	}
}

// --- SendGroupMessage ---

func TestSendGroupMessage_Success(t *testing.T) {
	t.Parallel()
	var gotParams sendParams
	server := newMockRPCServer(t, func(method string, params json.RawMessage, id int64) (any, *jsonRPCError) {
		if method != "send" {
			return nil, &jsonRPCError{Code: -32601, Message: "bad method"}
		}
		if err := json.Unmarshal(params, &gotParams); err != nil {
			return nil, &jsonRPCError{Code: -1, Message: err.Error()}
		}
		return SendResult{
			Timestamp: 1712654321000,
			Results: []SendRecipientResult{
				{RecipientAddress: RecipientAddress{Number: "+1111", UUID: "uuid-1"}, Type: "SUCCESS"},
			},
		}, nil
	})
	defer server.Close()

	c := NewClient(server.URL())
	res, err := c.SendGroupMessage(context.Background(), "base64==", "hello world")
	if err != nil {
		t.Fatalf("SendGroupMessage() error = %v", err)
	}
	if gotParams.GroupID != "base64==" || gotParams.Message != "hello world" {
		t.Errorf("unexpected params: %+v", gotParams)
	}
	if res.Timestamp != 1712654321000 {
		t.Errorf("Timestamp = %d, want 1712654321000", res.Timestamp)
	}
	if len(res.Results) != 1 || res.Results[0].Type != "SUCCESS" {
		t.Errorf("unexpected results: %+v", res.Results)
	}
}

func TestSendGroupMessage_EmptyGroupID(t *testing.T) {
	t.Parallel()
	c := NewClient("http://unused")
	_, err := c.SendGroupMessage(context.Background(), "", "hi")
	if err == nil {
		t.Fatal("expected error for empty groupID")
	}
}

func TestSendGroupMessage_EmptyMessage(t *testing.T) {
	t.Parallel()
	c := NewClient("http://unused")
	_, err := c.SendGroupMessage(context.Background(), "base64==", "")
	if err == nil {
		t.Fatal("expected error for empty message")
	}
}

func TestSendGroupMessage_RPCError(t *testing.T) {
	t.Parallel()
	server := newMockRPCServer(t, func(method string, params json.RawMessage, id int64) (any, *jsonRPCError) {
		return nil, &jsonRPCError{Code: -1, Message: "Invalid group id"}
	})
	defer server.Close()

	c := NewClient(server.URL())
	_, err := c.SendGroupMessage(context.Background(), "badid", "hi")
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *RPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != -1 {
		t.Errorf("Code = %d, want -1", rpcErr.Code)
	}
}

// --- Receive ---

func TestReceive_Success(t *testing.T) {
	t.Parallel()
	server := newMockRPCServer(t, func(method string, params json.RawMessage, id int64) (any, *jsonRPCError) {
		if method != "receive" {
			return nil, &jsonRPCError{Code: -32601, Message: "bad"}
		}
		return []map[string]any{
			{
				"envelope": map[string]any{
					"source":       "+1111",
					"sourceNumber": "+1111",
					"sourceName":   "Alice",
					"timestamp":    1712650000000,
					"dataMessage": map[string]any{
						"timestamp": 1712650000000,
						"message":   "hello",
						"groupInfo": map[string]any{
							"groupId": "g1",
							"type":    "DELIVER",
						},
					},
				},
			},
		}, nil
	})
	defer server.Close()

	c := NewClient(server.URL())
	msgs, err := c.Receive(context.Background(), 1)
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	m := msgs[0]
	if m.SourceNumber != "+1111" || m.SourceName != "Alice" || m.Text != "hello" || m.GroupID != "g1" {
		t.Errorf("unexpected message: %+v", m)
	}
}

func TestReceive_AutoReceiveMode(t *testing.T) {
	t.Parallel()
	server := newMockRPCServer(t, func(method string, params json.RawMessage, id int64) (any, *jsonRPCError) {
		return nil, &jsonRPCError{
			Code:    -1,
			Message: "Receive command cannot be used if messages are already being received.",
		}
	})
	defer server.Close()

	c := NewClient(server.URL())
	_, err := c.Receive(context.Background(), 1)
	if !errors.Is(err, ErrAutoReceiveActive) {
		t.Fatalf("expected ErrAutoReceiveActive, got %v", err)
	}
}

func TestReceive_NegativeTimeout(t *testing.T) {
	t.Parallel()
	c := NewClient("http://unused")
	_, err := c.Receive(context.Background(), -1)
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}
}

// --- Connection errors ---

func TestCall_ConnectionRefused(t *testing.T) {
	t.Parallel()
	// 127.0.0.1:1 is a reserved port with nothing listening.
	c := NewClient("http://127.0.0.1:1")
	_, err := c.ListGroups(context.Background())
	if !errors.Is(err, ErrDaemonUnreachable) {
		t.Fatalf("expected ErrDaemonUnreachable, got %v", err)
	}
}

func TestCall_MalformedResponse(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not json"))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	_, err := c.ListGroups(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed response")
	}
	if errors.Is(err, ErrDaemonUnreachable) {
		t.Errorf("malformed response should not be ErrDaemonUnreachable: %v", err)
	}
}

func TestCall_Non2xxStatus(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	_, err := c.ListGroups(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code: %v", err)
	}
}
