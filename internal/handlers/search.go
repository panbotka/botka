package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SearchHandler handles full-text search across chat messages.
type SearchHandler struct {
	db *gorm.DB
}

// NewSearchHandler creates a new SearchHandler with the given database connection.
func NewSearchHandler(db *gorm.DB) *SearchHandler {
	return &SearchHandler{db: db}
}

// RegisterSearchRoutes attaches search endpoints to the given router group.
func RegisterSearchRoutes(rg *gin.RouterGroup, h *SearchHandler) {
	rg.GET("/search", h.Search)
}

type searchMatch struct {
	MessageID int64     `json:"message_id"`
	Role      string    `json:"role"`
	Snippet   string    `json:"snippet"`
	CreatedAt time.Time `json:"created_at"`
}

type searchResult struct {
	ThreadID  int64         `json:"thread_id"`
	Title     string        `json:"title"`
	Model     *string       `json:"model"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Matches   []searchMatch `json:"matches"`
}

// Search performs full-text search across messages using PostgreSQL tsvector.
func (h *SearchHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		respondOK(c, []searchResult{})
		return
	}

	type row struct {
		MessageID  int64
		Role       string
		MsgCreated time.Time
		ThreadID   int64
		Title      string
		Model      *string
		TCreated   time.Time
		TUpdated   time.Time
		Snippet    string
	}

	var rows []row
	err := h.db.Raw(`
		SELECT
			m.id AS message_id, m.role, m.created_at AS msg_created,
			t.id AS thread_id, t.title, t.model, t.created_at AS t_created, t.updated_at AS t_updated,
			ts_headline('english', m.content, plainto_tsquery('english', ?),
				'StartSel=<mark>, StopSel=</mark>, MaxWords=35, MinWords=15') AS snippet
		FROM messages m
		JOIN threads t ON t.id = m.thread_id
		WHERE m.tsv @@ plainto_tsquery('english', ?)
		ORDER BY ts_rank(m.tsv, plainto_tsquery('english', ?)) DESC
		LIMIT 50`, q, q, q).Scan(&rows).Error

	if err != nil {
		slog.Error("search error", "error", err)
		respondError(c, http.StatusInternalServerError, "search failed")
		return
	}

	// Group by thread, preserving rank order
	threadMap := make(map[int64]*searchResult)
	var order []int64
	for _, r := range rows {
		if _, exists := threadMap[r.ThreadID]; !exists {
			threadMap[r.ThreadID] = &searchResult{
				ThreadID:  r.ThreadID,
				Title:     r.Title,
				Model:     r.Model,
				CreatedAt: r.TCreated,
				UpdatedAt: r.TUpdated,
			}
			order = append(order, r.ThreadID)
		}
		threadMap[r.ThreadID].Matches = append(threadMap[r.ThreadID].Matches, searchMatch{
			MessageID: r.MessageID,
			Role:      r.Role,
			Snippet:   r.Snippet,
			CreatedAt: r.MsgCreated,
		})
	}

	results := make([]searchResult, 0, len(order))
	for _, id := range order {
		results = append(results, *threadMap[id])
	}

	respondOK(c, results)
}
