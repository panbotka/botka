package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// buildPrefixTSQuery turns a free-text query into a PostgreSQL to_tsquery string
// where every word is a prefix match joined with AND, e.g. "horiz zalu" becomes
// "horiz:* & zalu:*". This lets search match partial words as the user types.
//
// tsquery operator characters are stripped from each token so user input can
// never inject operators or produce a parse error. Accented letters are kept;
// the caller folds them through botka_immutable_unaccent in SQL. Returns "" when
// nothing usable remains (caller should treat that as "no results").
func buildPrefixTSQuery(raw string) string {
	tokens := make([]string, 0, 4)
	for _, field := range strings.Fields(raw) {
		clean := strings.Map(func(r rune) rune {
			switch r {
			case '&', '|', '!', '(', ')', ':', '*', '\'', '"', '\\', '<', '>':
				return -1
			default:
				return r
			}
		}, field)
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		tokens = append(tokens, clean+":*")
	}
	return strings.Join(tokens, " & ")
}

// ThreadSearchHit is one conversation result from the unified search. There is
// at most one hit per thread; the snippet is the best-matching message (empty
// when only the title matched).
type ThreadSearchHit struct {
	ThreadID       int64     `json:"thread_id"`
	ThreadTitle    string    `json:"thread_title"`
	MessageID      *int64    `json:"message_id,omitempty"`
	Role           string    `json:"role,omitempty"`
	ContentSnippet string    `json:"content_snippet"`
	MatchedTitle   bool      `json:"matched_title"`
	Rank           float64   `json:"rank"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TaskSearchHit is one task result from the unified search.
type TaskSearchHit struct {
	TaskID         string  `json:"task_id"`
	Title          string  `json:"title"`
	Status         string  `json:"status"`
	ProjectID      *string `json:"project_id,omitempty"`
	ContentSnippet string  `json:"content_snippet"`
	Rank           float64 `json:"rank"`
}

// UnifiedSearchResult is the payload of GET /api/v1/search/all: ranked
// conversation and task hits for one query.
type UnifiedSearchResult struct {
	Threads []ThreadSearchHit `json:"threads"`
	Tasks   []TaskSearchHit   `json:"tasks"`
}

// unifiedSearchLimit caps each section of the unified search.
const unifiedSearchLimit = 20

// SearchAll runs a single full-text query across conversations and tasks,
// returning ranked hits for each. Threads rank title matches (weight A) above
// message-body matches (weight C); tasks rank title (A) above spec (B) above
// failure summary (C). Both fold diacritics and support prefix matching.
func (h *SearchHandler) SearchAll(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	tsq := buildPrefixTSQuery(query)
	if tsq == "" {
		respondOK(c, UnifiedSearchResult{Threads: []ThreadSearchHit{}, Tasks: []TaskSearchHit{}})
		return
	}

	threads, err := searchThreads(h.db, tsq)
	if err != nil {
		slog.Error("unified search: threads", "error", err)
		respondError(c, http.StatusInternalServerError, "search failed")
		return
	}
	tasks, err := searchTasks(h.db, tsq)
	if err != nil {
		slog.Error("unified search: tasks", "error", err)
		respondError(c, http.StatusInternalServerError, "search failed")
		return
	}
	respondOK(c, UnifiedSearchResult{Threads: threads, Tasks: tasks})
}

// headlineOptsUnified mirrors the ts_headline options used by message search so
// snippets render consistently across the two search endpoints.
var headlineOptsUnified = "StartSel=" + headlineStartSentinel +
	", StopSel=" + headlineEndSentinel +
	", MaxWords=20, MinWords=10, ShortWord=3, HighlightAll=false, MaxFragments=2"

// searchThreads returns one ranked hit per matching conversation: title matches
// (weight A) sort above body-only matches, and the snippet is the best-matching
// message in that thread (null when only the title matched).
func searchThreads(db *gorm.DB, tsq string) ([]ThreadSearchHit, error) {
	const sql = `
		WITH q AS (SELECT to_tsquery('pg_catalog.simple', botka_immutable_unaccent(?)) AS query)
		SELECT
			t.id AS thread_id,
			t.title AS thread_title,
			t.updated_at,
			(to_tsvector('pg_catalog.simple', botka_immutable_unaccent(t.title)) @@ q.query) AS matched_title,
			ts_rank(setweight(to_tsvector('pg_catalog.simple', botka_immutable_unaccent(t.title)), 'A'), q.query) AS title_rank,
			bm.message_id,
			bm.role,
			COALESCE(bm.body_rank, 0) AS body_rank,
			bm.headline
		FROM threads t
		CROSS JOIN q
		LEFT JOIN LATERAL (
			SELECT
				m.id AS message_id,
				m.role,
				ts_rank(setweight(m.search_vector, 'C'), q.query) AS body_rank,
				ts_headline('pg_catalog.simple', botka_immutable_unaccent(m.content), q.query, ?) AS headline
			FROM messages m
			WHERE m.thread_id = t.id AND m.deleted_at IS NULL AND m.search_vector @@ q.query
			ORDER BY ts_rank(m.search_vector, q.query) DESC, m.created_at DESC
			LIMIT 1
		) bm ON TRUE
		WHERE to_tsvector('pg_catalog.simple', botka_immutable_unaccent(t.title)) @@ q.query
			OR bm.message_id IS NOT NULL
		ORDER BY title_rank DESC, body_rank DESC, t.updated_at DESC
		LIMIT ?`

	type row struct {
		ThreadID     int64
		ThreadTitle  string
		UpdatedAt    time.Time
		MatchedTitle bool
		TitleRank    float64
		MessageID    *int64
		Role         string
		BodyRank     float64
		Headline     string
	}
	var rows []row
	if err := db.Raw(sql, tsq, headlineOptsUnified, unifiedSearchLimit).Scan(&rows).Error; err != nil {
		return nil, err
	}

	hits := make([]ThreadSearchHit, 0, len(rows))
	for _, r := range rows {
		snippet := ""
		if r.Headline != "" {
			snippet = sanitizeHeadline(r.Headline)
		}
		hits = append(hits, ThreadSearchHit{
			ThreadID:       r.ThreadID,
			ThreadTitle:    r.ThreadTitle,
			MessageID:      r.MessageID,
			Role:           r.Role,
			ContentSnippet: snippet,
			MatchedTitle:   r.MatchedTitle,
			Rank:           r.TitleRank + r.BodyRank,
			UpdatedAt:      r.UpdatedAt,
		})
	}
	return hits, nil
}

// searchTasks returns ranked task hits. The weighted search_vector (migration
// 037) makes title matches outrank spec matches, which outrank failure-summary
// matches. The snippet is taken from the spec (falling back to the title).
func searchTasks(db *gorm.DB, tsq string) ([]TaskSearchHit, error) {
	const sql = `
		WITH q AS (SELECT to_tsquery('pg_catalog.simple', botka_immutable_unaccent(?)) AS query)
		SELECT
			t.id AS task_id,
			t.title,
			t.status,
			t.project_id,
			ts_rank(t.search_vector, q.query) AS rank,
			ts_headline('pg_catalog.simple',
				botka_immutable_unaccent(COALESCE(NULLIF(t.spec, ''), t.title)),
				q.query, ?) AS headline
		FROM tasks t
		CROSS JOIN q
		WHERE t.search_vector @@ q.query
		ORDER BY rank DESC, t.created_at DESC
		LIMIT ?`

	type row struct {
		TaskID    string
		Title     string
		Status    string
		ProjectID *string
		Rank      float64
		Headline  string
	}
	var rows []row
	if err := db.Raw(sql, tsq, headlineOptsUnified, unifiedSearchLimit).Scan(&rows).Error; err != nil {
		return nil, err
	}

	hits := make([]TaskSearchHit, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, TaskSearchHit{
			TaskID:         r.TaskID,
			Title:          r.Title,
			Status:         r.Status,
			ProjectID:      r.ProjectID,
			ContentSnippet: sanitizeHeadline(r.Headline),
			Rank:           r.Rank,
		})
	}
	return hits, nil
}
