package mcp

import (
	"encoding/json"
	"fmt"

	"botka/internal/handlers"
)

// searchMessagesArgs holds the arguments for the search_messages tool.
type searchMessagesArgs struct {
	Query    string `json:"query"`
	ThreadID int64  `json:"thread_id"`
	Limit    int    `json:"limit"`
}

// handleSearchMessages performs full-text search across chat messages,
// optionally restricted to a single thread. Results are ranked by relevance
// and include the same shape as GET /api/v1/search/messages so the same
// frontend code can consume both.
func (s *Server) handleSearchMessages(raw json.RawMessage) (interface{}, error) {
	var args searchMessagesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	hits, total, err := handlers.RunMessageSearch(s.db, handlers.MessageSearchParams{
		Query:    args.Query,
		ThreadID: args.ThreadID,
		Limit:    args.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return map[string]interface{}{
		"data":  hits,
		"total": total,
	}, nil
}
