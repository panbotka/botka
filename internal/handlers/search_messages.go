package handlers

import (
	"errors"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MessageSearchHit is one result from /api/v1/search/messages.
type MessageSearchHit struct {
	MessageID      int64     `json:"message_id"`
	ThreadID       int64     `json:"thread_id"`
	ThreadTitle    string    `json:"thread_title"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
	ContentSnippet string    `json:"content_snippet"`
	Rank           float64   `json:"rank"`
}

// MessageSearchParams holds the parsed inputs for a message search.
type MessageSearchParams struct {
	Query    string
	Limit    int
	Offset   int
	ThreadID int64 // 0 means search across all threads
}

// Sentinels used as ts_headline StartSel/StopSel. We pick alphanumeric tokens
// that contain no HTML-special characters, then HTML-escape the entire snippet
// before substituting them for <mark>/</mark>. This guarantees that any HTML
// in user content is escaped while still permitting only <mark> highlighting.
const (
	headlineStartSentinel = "BOTKAMARKSTART"
	headlineEndSentinel   = "BOTKAMARKEND"
)

// messageSearchMaxLimit is the upper bound on result page size, per spec.
const (
	messageSearchDefaultLimit = 30
	messageSearchMaxLimit     = 100
)

// SearchMessages performs cross-thread (or in-thread) full-text search over
// non-deleted chat messages, returning ranked hits with highlighted snippets.
func (h *SearchHandler) SearchMessages(c *gin.Context) {
	params, err := parseMessageSearchParams(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	hits, total, err := h.searchMessages(params)
	if err != nil {
		slog.Error("search messages", "error", err)
		respondError(c, http.StatusInternalServerError, "search failed")
		return
	}
	respondList(c, hits, total)
}

// parseMessageSearchParams reads and validates query parameters for message
// search. limit defaults to 30 and is capped at 100; offset defaults to 0.
func parseMessageSearchParams(c *gin.Context) (MessageSearchParams, error) {
	p := MessageSearchParams{
		Query:  strings.TrimSpace(c.Query("q")),
		Limit:  messageSearchDefaultLimit,
		Offset: 0,
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return p, errors.New("limit must be a positive integer")
		}
		if n > messageSearchMaxLimit {
			n = messageSearchMaxLimit
		}
		p.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return p, errors.New("offset must be a non-negative integer")
		}
		p.Offset = n
	}
	if v := c.Query("thread_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return p, errors.New("thread_id must be a positive integer")
		}
		p.ThreadID = n
	}
	return p, nil
}

// searchMessages executes the count + ranked query and returns sanitized hits.
// An empty query yields an empty result rather than an error, so the frontend
// can debounce calls without juggling status codes.
func (h *SearchHandler) searchMessages(p MessageSearchParams) ([]MessageSearchHit, int64, error) {
	if p.Query == "" {
		return []MessageSearchHit{}, 0, nil
	}
	return RunMessageSearch(h.db, p)
}

// RunMessageSearch executes the count + ranked query against the messages
// table and returns sanitized hits. Exported so the MCP tool can reuse the
// same logic as the HTTP handler.
func RunMessageSearch(db *gorm.DB, p MessageSearchParams) ([]MessageSearchHit, int64, error) {
	if p.Query == "" {
		return []MessageSearchHit{}, 0, nil
	}
	if p.Limit <= 0 {
		p.Limit = messageSearchDefaultLimit
	}
	if p.Limit > messageSearchMaxLimit {
		p.Limit = messageSearchMaxLimit
	}

	// Build the WHERE fragment shared by count + select. plainto_tsquery is
	// safe against malformed input: it ignores operators and quotes.
	whereSQL := `m.deleted_at IS NULL
		AND m.search_vector @@ plainto_tsquery('simple', ?)`
	whereArgs := []interface{}{p.Query}
	if p.ThreadID > 0 {
		whereSQL += " AND m.thread_id = ?"
		whereArgs = append(whereArgs, p.ThreadID)
	}

	// Total count (without limit/offset) so the frontend can paginate.
	var total int64
	countSQL := `SELECT COUNT(*) FROM messages m WHERE ` + whereSQL
	if err := db.Raw(countSQL, whereArgs...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []MessageSearchHit{}, 0, nil
	}

	// ts_headline options: per spec — MaxWords=20, MinWords=10, ShortWord=3,
	// HighlightAll=false, MaxFragments=2. StartSel/StopSel use alphanumeric
	// sentinels that survive HTML-escaping unchanged so we can substitute
	// them for <mark>/</mark> after escaping the rest of the snippet.
	headlineOpts := "StartSel=" + headlineStartSentinel +
		", StopSel=" + headlineEndSentinel +
		", MaxWords=20, MinWords=10, ShortWord=3, HighlightAll=false, MaxFragments=2"

	type row struct {
		MessageID   int64
		ThreadID    int64
		ThreadTitle string
		Role        string
		CreatedAt   time.Time
		Headline    string
		Rank        float64
	}

	selectSQL := `
		SELECT
			m.id AS message_id,
			m.thread_id,
			t.title AS thread_title,
			m.role,
			m.created_at,
			ts_headline('pg_catalog.simple', m.content,
				plainto_tsquery('simple', ?), ?) AS headline,
			ts_rank(m.search_vector, plainto_tsquery('simple', ?)) AS rank
		FROM messages m
		JOIN threads t ON t.id = m.thread_id
		WHERE ` + whereSQL + `
		ORDER BY rank DESC, m.created_at DESC
		LIMIT ? OFFSET ?`

	args := []interface{}{p.Query, headlineOpts, p.Query}
	args = append(args, whereArgs...)
	args = append(args, p.Limit, p.Offset)

	var rows []row
	if err := db.Raw(selectSQL, args...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	hits := make([]MessageSearchHit, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, MessageSearchHit{
			MessageID:      r.MessageID,
			ThreadID:       r.ThreadID,
			ThreadTitle:    r.ThreadTitle,
			Role:           r.Role,
			CreatedAt:      r.CreatedAt,
			ContentSnippet: sanitizeHeadline(r.Headline),
			Rank:           r.Rank,
		})
	}
	return hits, total, nil
}

// sanitizeHeadline converts a ts_headline output that uses our sentinels into
// safe HTML containing only <mark> tags. Everything else is HTML-escaped, so
// any tags or entities in the original content are rendered as inert text.
func sanitizeHeadline(s string) string {
	escaped := html.EscapeString(s)
	escaped = strings.ReplaceAll(escaped, headlineStartSentinel, "<mark>")
	escaped = strings.ReplaceAll(escaped, headlineEndSentinel, "</mark>")
	return escaped
}
