// Package signal provides a Go client for the signal-cli daemon HTTP JSON-RPC API.
//
// The client wraps the signal-cli daemon running in HTTP mode
// (signal-cli daemon --http <host:port>) and exposes a small, typed surface for
// listing Signal groups, sending group messages, and receiving incoming
// messages.
//
// Receive semantics: signal-cli's HTTP daemon runs in "auto-receive" mode by
// default, in which case the synchronous JSON-RPC `receive` method is refused
// with the error "Receive command cannot be used if messages are already
// being received.". For that deployment model the daemon instead exposes a
// Server-Sent Events stream at /api/v1/events that pushes every incoming
// envelope as a JSON-RPC notification. This package provides both entry
// points:
//
//   - Receive polls via JSON-RPC receive — useful when the daemon was started
//     with --receive-mode=manual.
//   - Subscribe consumes the SSE stream and calls a handler for each message
//     — the only option when the daemon is in auto-receive mode.
package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Default timeouts and endpoint paths used by the client.
const (
	// DefaultSendTimeout is the HTTP timeout applied to synchronous
	// JSON-RPC requests (listGroups, send) when the caller does not
	// supply a context with its own deadline.
	DefaultSendTimeout = 30 * time.Second

	// rpcPath is the JSON-RPC endpoint path exposed by signal-cli daemon.
	rpcPath = "/api/v1/rpc"

	// eventsPath is the Server-Sent Events endpoint that signal-cli daemon
	// uses to publish incoming-message notifications when running in
	// auto-receive mode.
	eventsPath = "/api/v1/events"
)

// ErrDaemonUnreachable is returned when the HTTP transport cannot connect to
// the signal-cli daemon (for example, connection refused). Callers can use
// errors.Is to distinguish this from protocol-level errors.
var ErrDaemonUnreachable = errors.New("signal-cli daemon unreachable")

// ErrAutoReceiveActive is returned by Receive when the daemon is running in
// auto-receive mode and refuses the synchronous `receive` JSON-RPC call.
// Callers should fall back to Subscribe in that case.
var ErrAutoReceiveActive = errors.New("signal-cli daemon is in auto-receive mode; use Subscribe")

// Client is a thin wrapper over signal-cli's HTTP JSON-RPC API.
//
// Client is safe for concurrent use by multiple goroutines. The zero value is
// not usable; always construct a Client with NewClient.
type Client struct {
	baseURL string
	http    *http.Client
	nextID  atomic.Int64
}

// ClientOption configures optional behavior on a Client.
type ClientOption func(*Client)

// WithHTTPClient overrides the HTTP client used for JSON-RPC requests. The
// provided client's Timeout is also used as the default per-call deadline
// when the caller's context has no deadline attached.
func WithHTTPClient(h *http.Client) ClientOption {
	return func(c *Client) { c.http = h }
}

// NewClient constructs a Client pointing at the signal-cli daemon base URL
// (for example, "http://127.0.0.1:5107"). Any trailing slashes on baseURL
// are trimmed. If no HTTP client is supplied via WithHTTPClient, a client
// with a 30 second timeout is used.
func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: DefaultSendTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// jsonRPCRequest is the JSON-RPC 2.0 request envelope.
type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int64  `json:"id"`
}

// jsonRPCError describes a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("signal-cli rpc error %d: %s", e.Code, e.Message)
}

// RPCError is the exported error type returned when signal-cli replies with a
// JSON-RPC error object. Callers can type-assert to inspect the code and
// original message.
type RPCError struct {
	Code    int
	Message string
	Data    json.RawMessage
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	return fmt.Sprintf("signal-cli rpc error %d: %s", e.Code, e.Message)
}

// jsonRPCResponse is the JSON-RPC 2.0 response envelope. Result is captured as
// raw JSON so the caller can decode it into the method-specific type.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *jsonRPCError   `json:"error"`
	ID      int64           `json:"id"`
}

// call executes a JSON-RPC method against the daemon and decodes the result
// into out. If out is nil the result is discarded. call returns RPCError for
// protocol-level errors, ErrDaemonUnreachable for connection failures, and
// a wrapped error for any other transport or decoding problem.
func (c *Client) call(ctx context.Context, method string, params, out any) error {
	id := c.nextID.Add(1)
	body, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	})
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", method, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+rpcPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if isConnRefused(err) {
			return fmt.Errorf("%w: %s: %w", ErrDaemonUnreachable, c.baseURL, err)
		}
		return fmt.Errorf("signal-cli %s http request: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("signal-cli %s http %d: %s", method, resp.StatusCode, strings.TrimSpace(string(preview)))
	}

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if rpcResp.Error != nil {
		return &RPCError{Code: rpcResp.Error.Code, Message: rpcResp.Error.Message, Data: rpcResp.Error.Data}
	}
	if out == nil {
		return nil
	}
	if len(rpcResp.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(rpcResp.Result, out); err != nil {
		return fmt.Errorf("decode %s result: %w", method, err)
	}
	return nil
}

// isConnRefused reports whether err looks like a connection-refused error
// from the HTTP transport. It performs a string match because net.OpError's
// underlying syscall error is not stable across platforms.
func isConnRefused(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") || strings.Contains(msg, "connect: refused")
}
