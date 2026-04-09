package signal

import "encoding/json"

// Member is a single member (or admin) of a Signal group.
type Member struct {
	// Number is the E.164 phone number, e.g. "+420723750652". May be empty
	// if the account only exposes a UUID.
	Number string `json:"number"`
	// UUID is the Signal account UUID. May be empty.
	UUID string `json:"uuid"`
}

// SignalGroup describes a Signal group the account is a member of.
type SignalGroup struct {
	// ID is the base64-encoded group identifier signal-cli uses in RPC calls
	// (for example, "4vnBAIsEAQqiNuELxVNJqfhDzHz4ZwMn9T0BjZtr+ng=").
	ID string `json:"id"`
	// Name is the human-readable group name.
	Name string `json:"name"`
	// Description is the optional group description set by admins.
	Description string `json:"description,omitempty"`
	// Members lists all current members of the group.
	Members []Member `json:"members"`
}

// MemberCount returns the number of current members in the group. It exists
// as a convenience for callers that only want to display a count.
func (g SignalGroup) MemberCount() int { return len(g.Members) }

// RecipientAddress identifies a recipient in a SendResult entry.
type RecipientAddress struct {
	Number string `json:"number,omitempty"`
	UUID   string `json:"uuid,omitempty"`
}

// SendRecipientResult describes the delivery status for one recipient of a
// send operation.
type SendRecipientResult struct {
	// RecipientAddress identifies the recipient device/user.
	RecipientAddress RecipientAddress `json:"recipientAddress"`
	// Type is the per-recipient outcome, e.g. "SUCCESS", "UNREGISTERED_FAILURE".
	Type string `json:"type"`
}

// SendResult is the result returned by a successful send call.
type SendResult struct {
	// Timestamp is the Signal server timestamp (epoch milliseconds) that
	// uniquely identifies this message.
	Timestamp int64 `json:"timestamp"`
	// Results contains one entry per recipient.
	Results []SendRecipientResult `json:"results"`
}

// DataMessage is the content portion of an incoming envelope.
type DataMessage struct {
	Timestamp        int64             `json:"timestamp"`
	Message          string            `json:"message"`
	ExpiresInSeconds int               `json:"expiresInSeconds"`
	ViewOnce         bool              `json:"viewOnce"`
	GroupInfo        *DataMessageGroup `json:"groupInfo,omitempty"`
}

// DataMessageGroup identifies the group a DataMessage belongs to.
type DataMessageGroup struct {
	GroupID string `json:"groupId"`
	Type    string `json:"type"`
}

// Envelope is the raw envelope structure published by signal-cli for each
// incoming message. Fields are a subset of what the daemon emits — only the
// ones needed by SignalMessage.FromEnvelope are modeled; everything else is
// retained as Extra for callers that need fuller fidelity.
type Envelope struct {
	Source       string          `json:"source"`
	SourceNumber string          `json:"sourceNumber"`
	SourceUUID   string          `json:"sourceUuid"`
	SourceName   string          `json:"sourceName"`
	SourceDevice int             `json:"sourceDevice"`
	Timestamp    int64           `json:"timestamp"`
	DataMessage  *DataMessage    `json:"dataMessage,omitempty"`
	Extra        json.RawMessage `json:"-"`
}

// SignalMessage is a flattened, caller-friendly view of an incoming Signal
// message. It collapses the envelope/dataMessage hierarchy into a single
// struct with only the fields needed by Botka.
type SignalMessage struct {
	// SourceNumber is the sender's E.164 phone number (may be empty).
	SourceNumber string
	// SourceName is the sender's display name (may be empty).
	SourceName string
	// Timestamp is the sender-assigned timestamp in epoch milliseconds.
	Timestamp int64
	// Text is the message body. Empty for non-text messages.
	Text string
	// GroupID is the base64 group ID if the message was sent to a group,
	// or the empty string for a direct message.
	GroupID string
	// GroupName is populated when the caller correlates GroupID with a
	// listGroups result; Subscribe leaves it empty.
	GroupName string
}

// MessageFromEnvelope projects an Envelope into the flattened SignalMessage
// representation. It is exported so tests and higher-level packages can
// reuse the conversion logic.
func MessageFromEnvelope(env Envelope) SignalMessage {
	msg := SignalMessage{
		SourceNumber: env.SourceNumber,
		SourceName:   env.SourceName,
		Timestamp:    env.Timestamp,
	}
	if msg.SourceNumber == "" {
		msg.SourceNumber = env.Source
	}
	if env.DataMessage != nil {
		msg.Text = env.DataMessage.Message
		if env.DataMessage.Timestamp != 0 {
			msg.Timestamp = env.DataMessage.Timestamp
		}
		if env.DataMessage.GroupInfo != nil {
			msg.GroupID = env.DataMessage.GroupInfo.GroupID
		}
	}
	return msg
}
