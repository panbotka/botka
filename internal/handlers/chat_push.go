package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"botka/internal/models"
	"botka/internal/push"
)

// PushNotifier is the subset of internal/push.Sender used by chat handlers
// to notify users of new assistant replies. Defined here (not imported
// directly) so tests can substitute a fake without depending on internal/push.
type PushNotifier interface {
	Send(ctx context.Context, userID int64, payload push.PushPayload) error
	Broadcast(ctx context.Context, payload push.PushPayload) error
}

const (
	chatPushSendTimeout = 30 * time.Second
	chatPushBodyMaxRune = 100
	chatPushDefaultName = "Botka Chat"
)

// notifyAssistantReply fires a Web Push for an assistant reply that has just
// been persisted. Best-effort: errors are logged and never propagated, and
// callers may invoke this in a goroutine.
//
// Recipient: threads have no explicit owner column, so we Broadcast to every
// subscription. ThreadAccess scopes external users to specific threads;
// admins implicitly see every thread.
//
// `clientStillConnected` is the suppression signal: when the SSE client that
// originated the message is still connected, the user is actively watching
// the response and we skip the OS-level notification.
//
// TODO: the SSE buffer in internal/claude tracks subscribers per thread but
// not per user. We rely on the per-request `clientStillConnected` flag to
// detect the message-sending user; viewers on other devices may receive a
// redundant push, but the frontend can suppress in-tab.
func notifyAssistantReply(
	notifier PushNotifier, enabled bool,
	thread *models.Thread, content string,
	clientStillConnected bool,
) {
	if notifier == nil || !enabled || thread == nil {
		return
	}
	if clientStillConnected {
		return
	}

	payload := push.PushPayload{
		Title: chatPushTitle(thread),
		Body:  truncateRunes(stripMarkdown(content), chatPushBodyMaxRune),
		URL:   fmt.Sprintf("/chat/%d", thread.ID),
		Tag:   fmt.Sprintf("thread-%d", thread.ID),
	}

	ctx, cancel := context.WithTimeout(context.Background(), chatPushSendTimeout)
	defer cancel()
	if err := notifier.Broadcast(ctx, payload); err != nil {
		slog.Warn("chat reply push broadcast failed",
			"thread_id", thread.ID, "error", err)
	}
}

func chatPushTitle(thread *models.Thread) string {
	if t := strings.TrimSpace(thread.Title); t != "" {
		return t
	}
	return chatPushDefaultName
}

// truncateRunes returns s if it has no more than maxRunes runes, otherwise
// the first maxRunes runes followed by an ellipsis. Operates on runes (not
// bytes) so multibyte UTF-8 input is not chopped mid-character.
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// markdown-stripping regexes. Compiled once at package load.
var (
	mdHeadingPrefix   = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+`)
	mdListPrefix      = regexp.MustCompile(`(?m)^\s{0,3}([-*+]|\d+\.)\s+`)
	mdBlockquote      = regexp.MustCompile(`(?m)^\s{0,3}>\s?`)
	mdCodeFence       = regexp.MustCompile("(?m)^\\s*```[^\n]*$")
	mdInlineCode      = regexp.MustCompile("`+")
	mdEmphasisMarkers = regexp.MustCompile(`(\*{1,3}|_{1,3})`)
	mdLink            = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	mdWhitespace      = regexp.MustCompile(`\s+`)
)

// stripMarkdown reduces a markdown-formatted message to a single line of
// plain text suitable for a notification body. The transforms are
// intentionally shallow: heading hashes, list bullets, blockquote markers,
// code-fence lines, inline backticks, simple emphasis markers, and link
// brackets. Everything else passes through, then whitespace collapses.
func stripMarkdown(s string) string {
	s = mdCodeFence.ReplaceAllString(s, "")
	s = mdHeadingPrefix.ReplaceAllString(s, "")
	s = mdListPrefix.ReplaceAllString(s, "")
	s = mdBlockquote.ReplaceAllString(s, "")
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdInlineCode.ReplaceAllString(s, "")
	s = mdEmphasisMarkers.ReplaceAllString(s, "")
	s = mdWhitespace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
