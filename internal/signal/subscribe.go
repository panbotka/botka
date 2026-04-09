package signal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MessageHandler is invoked for every incoming SignalMessage delivered over
// the SSE stream. Returning an error from the handler aborts the Subscribe
// loop and surfaces the error to the caller.
type MessageHandler func(ctx context.Context, msg SignalMessage) error

// Subscribe opens the signal-cli /api/v1/events Server-Sent Events stream and
// invokes handler for every incoming message envelope until ctx is cancelled,
// the stream closes, or handler returns an error.
//
// This is the receive path that works when signal-cli is in auto-receive
// mode (the default for daemon --http). The server pushes one SSE "data:"
// line per JSON-RPC notification; each notification has method "receive"
// and a params object containing the envelope.
//
// Subscribe uses its own HTTP request rather than the client's configured
// http.Client, because the stream is long-lived and must not be cut off by
// the default 30-second timeout.
func (c *Client) Subscribe(ctx context.Context, handler MessageHandler) error {
	if handler == nil {
		return errors.New("signal: handler is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+eventsPath, nil)
	if err != nil {
		return fmt.Errorf("build events request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Use a clone of the underlying Transport but with no overall timeout;
	// SSE streams must stay open indefinitely.
	streamClient := &http.Client{Transport: c.http.Transport}
	resp, err := streamClient.Do(req)
	if err != nil {
		if isConnRefused(err) {
			return fmt.Errorf("%w: %s: %w", ErrDaemonUnreachable, c.baseURL, err)
		}
		return fmt.Errorf("open events stream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("events stream http %d: %s", resp.StatusCode, strings.TrimSpace(string(preview)))
	}

	return parseSSEStream(ctx, resp.Body, handler)
}

// parseSSEStream reads an SSE stream from r and invokes handler for every
// incoming Signal message envelope it finds in "data:" lines. It exists as
// a separate function so the tests can exercise the parser without spinning
// up an HTTP server.
func parseSSEStream(ctx context.Context, r io.Reader, handler MessageHandler) error {
	scanner := bufio.NewScanner(r)
	// Raise the buffer size — SSE messages can be large, and the default
	// 64 KiB is not enough for attachments.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var dataBuf strings.Builder
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Text()
		switch {
		case line == "":
			// Blank line terminates an SSE event.
			if dataBuf.Len() > 0 {
				if err := dispatchSSEData(ctx, dataBuf.String(), handler); err != nil {
					return err
				}
				dataBuf.Reset()
			}
		case strings.HasPrefix(line, "data:"):
			payload := strings.TrimPrefix(line, "data:")
			payload = strings.TrimPrefix(payload, " ")
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(payload)
		default:
			// Ignore event:, id:, retry:, and comment lines (":...").
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read events stream: %w", err)
	}
	// Flush any trailing event that wasn't followed by a blank line.
	if dataBuf.Len() > 0 {
		if err := dispatchSSEData(ctx, dataBuf.String(), handler); err != nil {
			return err
		}
	}
	return nil
}

// notificationEnvelope is the JSON-RPC 2.0 notification shape signal-cli
// publishes on the SSE stream for each incoming Signal message. The daemon
// has used slightly different param shapes across versions; we model the
// flexible union here and pick whichever form is populated.
type notificationEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// notificationParams matches the modern signal-cli shape:
// {"envelope": {...}, "account": "+420..."}.
type notificationParams struct {
	Envelope *Envelope `json:"envelope"`
	Account  string    `json:"account"`
}

// dispatchSSEData decodes one SSE data block and forwards any enclosed
// SignalMessage to handler. Non-message notifications (e.g. presence or
// sync events) are silently ignored.
func dispatchSSEData(ctx context.Context, data string, handler MessageHandler) error {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil
	}
	var note notificationEnvelope
	if err := json.Unmarshal([]byte(data), &note); err != nil {
		// Non-JSON keepalive or comment; ignore.
		return nil
	}
	if note.Method != "" && note.Method != "receive" {
		return nil
	}
	if len(note.Params) == 0 {
		return nil
	}

	// Preferred shape: {"envelope": {...}}.
	var params notificationParams
	if err := json.Unmarshal(note.Params, &params); err == nil && params.Envelope != nil {
		return handler(ctx, MessageFromEnvelope(*params.Envelope))
	}

	// Fallback: params IS the envelope.
	var env Envelope
	if err := json.Unmarshal(note.Params, &env); err == nil && (env.DataMessage != nil || env.Source != "") {
		return handler(ctx, MessageFromEnvelope(env))
	}

	return nil
}
