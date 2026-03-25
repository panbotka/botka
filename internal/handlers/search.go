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

// Search performs diacritic-insensitive substring search across messages
// using PostgreSQL's unaccent extension.
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
		Content    string
	}

	var rows []row
	err := h.db.Raw(`
		SELECT
			m.id AS message_id, m.role, m.created_at AS msg_created,
			t.id AS thread_id, t.title, t.model, t.created_at AS t_created, t.updated_at AS t_updated,
			m.content
		FROM messages m
		JOIN threads t ON t.id = m.thread_id
		WHERE unaccent(lower(m.content)) LIKE '%' || unaccent(lower(?)) || '%'
		ORDER BY m.created_at DESC
		LIMIT 50`, q).Scan(&rows).Error

	if err != nil {
		slog.Error("search error", "error", err)
		respondError(c, http.StatusInternalServerError, "search failed")
		return
	}

	// Group by thread, preserving order
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
			Snippet:   buildSnippet(r.Content, q),
			CreatedAt: r.MsgCreated,
		})
	}

	results := make([]searchResult, 0, len(order))
	for _, id := range order {
		results = append(results, *threadMap[id])
	}

	respondOK(c, results)
}

// buildSnippet extracts a ~120-char window around the first match of query in
// content, with <mark> tags highlighting the matched text. The match is
// diacritic-insensitive and case-insensitive.
func buildSnippet(content, query string) string {
	normContent := stripDiacritics(strings.ToLower(content))
	normQuery := stripDiacritics(strings.ToLower(query))

	pos := strings.Index(normContent, normQuery)
	if pos < 0 {
		// Fallback: return beginning of content
		if len(content) > 120 {
			return content[:120] + "..."
		}
		return content
	}

	// Window: 50 chars before match, match itself, then fill to ~120 total
	start := pos - 50
	if start < 0 {
		start = 0
	}
	end := start + 120
	if end > len(content) {
		end = len(content)
	}

	// Align start/end to not split multi-byte UTF-8 runes
	runes := []rune(content)
	// Convert byte positions to rune positions
	runeStart := len([]rune(content[:start]))
	matchRuneStart := len([]rune(content[:pos]))
	// The matched text in the original content may have different rune count
	// than the query (diacritics are still single runes). Use the normalized
	// query length to find the right span in the original.
	matchRuneEnd := matchRuneStart + len([]rune(normQuery))
	if matchRuneEnd > len(runes) {
		matchRuneEnd = len(runes)
	}

	runeEnd := runeStart + len([]rune(content[start:end]))
	if runeEnd > len(runes) {
		runeEnd = len(runes)
	}

	var b strings.Builder
	if runeStart > 0 {
		b.WriteString("...")
	}
	b.WriteString(string(runes[runeStart:matchRuneStart]))
	b.WriteString("<mark>")
	b.WriteString(string(runes[matchRuneStart:matchRuneEnd]))
	b.WriteString("</mark>")
	b.WriteString(string(runes[matchRuneEnd:runeEnd]))
	if runeEnd < len(runes) {
		b.WriteString("...")
	}
	return b.String()
}

// stripDiacritics removes diacritical marks from a string by decomposing
// Unicode characters and removing combining marks.
func stripDiacritics(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if mapped, ok := diacriticMap[r]; ok {
			b.WriteRune(mapped)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// diacriticMap maps common accented characters to their base forms.
// This covers Czech, Slovak, German, French, Spanish, Polish, and other
// Central/Western European languages.
var diacriticMap = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a', 'ą': 'a',
	'č': 'c', 'ć': 'c', 'ç': 'c',
	'ď': 'd', 'đ': 'd',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e', 'ě': 'e', 'ę': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ľ': 'l', 'ĺ': 'l', 'ł': 'l',
	'ň': 'n', 'ń': 'n', 'ñ': 'n',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o', 'ő': 'o',
	'ř': 'r', 'ŕ': 'r',
	'š': 's', 'ś': 's', 'ş': 's',
	'ť': 't', 'ţ': 't',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u', 'ů': 'u', 'ű': 'u',
	'ý': 'y', 'ÿ': 'y',
	'ž': 'z', 'ź': 'z', 'ż': 'z',
	// Uppercase variants
	'Á': 'A', 'À': 'A', 'Â': 'A', 'Ä': 'A', 'Ã': 'A', 'Å': 'A', 'Ą': 'A',
	'Č': 'C', 'Ć': 'C', 'Ç': 'C',
	'Ď': 'D', 'Đ': 'D',
	'É': 'E', 'È': 'E', 'Ê': 'E', 'Ë': 'E', 'Ě': 'E', 'Ę': 'E',
	'Í': 'I', 'Ì': 'I', 'Î': 'I', 'Ï': 'I',
	'Ľ': 'L', 'Ĺ': 'L', 'Ł': 'L',
	'Ň': 'N', 'Ń': 'N', 'Ñ': 'N',
	'Ó': 'O', 'Ò': 'O', 'Ô': 'O', 'Ö': 'O', 'Õ': 'O', 'Ő': 'O',
	'Ř': 'R', 'Ŕ': 'R',
	'Š': 'S', 'Ś': 'S', 'Ş': 'S',
	'Ť': 'T', 'Ţ': 'T',
	'Ú': 'U', 'Ù': 'U', 'Û': 'U', 'Ü': 'U', 'Ů': 'U', 'Ű': 'U',
	'Ý': 'Y', 'Ÿ': 'Y',
	'Ž': 'Z', 'Ź': 'Z', 'Ż': 'Z',
}
