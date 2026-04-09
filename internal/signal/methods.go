package signal

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ListGroups returns every Signal group the account is a member of. It calls
// the JSON-RPC method "listGroups" with no parameters.
//
// Example:
//
//	groups, err := client.ListGroups(ctx)
//	if err != nil { ... }
//	for _, g := range groups { fmt.Println(g.ID, g.Name, g.MemberCount()) }
func (c *Client) ListGroups(ctx context.Context) ([]SignalGroup, error) {
	var groups []SignalGroup
	if err := c.call(ctx, "listGroups", struct{}{}, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// sendParams is the params object for the "send" JSON-RPC method when sending
// to a group.
type sendParams struct {
	GroupID string `json:"groupId"`
	Message string `json:"message"`
}

// SendGroupMessage sends a plain-text message to the group identified by
// groupID. groupID must be the base64 form returned in SignalGroup.ID.
// The returned SendResult exposes the Signal server timestamp and a
// per-recipient delivery status slice.
//
// SendGroupMessage returns a wrapped error if groupID or message is empty,
// an *RPCError if the daemon rejects the request, or ErrDaemonUnreachable
// if the daemon is not running.
func (c *Client) SendGroupMessage(ctx context.Context, groupID, message string) (SendResult, error) {
	if strings.TrimSpace(groupID) == "" {
		return SendResult{}, errors.New("signal: groupID is required")
	}
	if message == "" {
		return SendResult{}, errors.New("signal: message is required")
	}
	var result SendResult
	if err := c.call(ctx, "send", sendParams{GroupID: groupID, Message: message}, &result); err != nil {
		return SendResult{}, err
	}
	return result, nil
}

// receiveParams is the params object for the "receive" JSON-RPC method.
type receiveParams struct {
	Timeout int `json:"timeout"`
}

// Receive polls the daemon for incoming messages using the synchronous
// "receive" JSON-RPC method. timeoutSeconds is the maximum number of seconds
// the daemon may block waiting for new envelopes.
//
// Receive only works when signal-cli is started with --receive-mode=manual.
// When the daemon is in auto-receive mode (the default), it rejects the call
// with the error "Receive command cannot be used if messages are already
// being received.", which this function translates into ErrAutoReceiveActive.
// Callers should fall back to Subscribe in that case.
func (c *Client) Receive(ctx context.Context, timeoutSeconds int) ([]SignalMessage, error) {
	if timeoutSeconds < 0 {
		return nil, errors.New("signal: timeoutSeconds must be non-negative")
	}
	var envelopes []rawEnvelope
	if err := c.call(ctx, "receive", receiveParams{Timeout: timeoutSeconds}, &envelopes); err != nil {
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) && isAutoReceiveMessage(rpcErr.Message) {
			return nil, fmt.Errorf("%w: %s", ErrAutoReceiveActive, rpcErr.Message)
		}
		return nil, err
	}
	messages := make([]SignalMessage, 0, len(envelopes))
	for _, raw := range envelopes {
		if raw.Envelope == nil {
			continue
		}
		messages = append(messages, MessageFromEnvelope(*raw.Envelope))
	}
	return messages, nil
}

// rawEnvelope wraps an Envelope so we can decode either the plain envelope
// shape used by some signal-cli versions, or the {"envelope": {...}} wrapper
// used by others. Fields we do not care about are ignored.
type rawEnvelope struct {
	Envelope *Envelope `json:"envelope"`
}

// isAutoReceiveMessage reports whether message is the distinctive error text
// signal-cli returns when the synchronous receive call is disallowed.
func isAutoReceiveMessage(message string) bool {
	return strings.Contains(strings.ToLower(message), "receive command cannot be used")
}
