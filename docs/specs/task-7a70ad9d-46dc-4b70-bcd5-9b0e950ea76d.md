# Cross-thread full-text message search

Search across all chat messages with previews, ranking, and "jump to message" navigation.

## Background

Messages already have a `tsvector` `search_vector` column with a GIN index, but there is no API or UI for searching across threads. Users want to find "that conversation about VAPID keys" or "where I asked Claude how to debounce X" without scrolling through threads.

## Requirements

- New endpoint `GET /api/v1/search/messages?q=<query>&limit=N&offset=N&thread_id=<optional>`:
  - Filters out soft-deleted messages (`deleted_at IS NULL`) and soft-deleted threads.
  - Ranks by `ts_rank(search_vector, plainto_tsquery('simple', q))` desc, tiebreaker `created_at` desc.
  - Default limit 30, max 100.
  - Optional `thread_id` constrains search to a single thread (for in-thread search UX).
  - Each result includes: `message_id`, `thread_id`, `thread_title`, `role`, `created_at`, `content_snippet` (first match window with `<mark>` HTML highlighting via `ts_headline`), `rank`.
- Response shape: `{"data": [...], "total": <number-of-matches-without-limit>}`.
- Highlighting: use `ts_headline` with `MaxWords=20, MinWords=10, ShortWord=3, HighlightAll=false, MaxFragments=2`. Sanitize the headline before rendering (only `<mark>` tags allowed).
- Frontend:
  - Global Cmd+K / Ctrl+K opens a search palette overlay.
  - Search input with 250ms debounce.
  - Results list: thread title (small caps) + role icon + snippet with highlighted matches.
  - Clicking a result navigates to the thread and scrolls to the message; the message gets a brief flash highlight (1.5s) so the user can spot it.
  - Within a thread, a separate "Search this thread" affordance reuses the same endpoint with `thread_id`.
  - Show "Total: N matches" and pagination ("Load more").
- MCP: add `search_messages` tool with `query`, `thread_id`, `limit` parameters returning the same shape.

## Implementation Notes

- The `search_vector` column and GIN index already exist on `messages` — do not recreate them. Just query.
- For "jump to message", the frontend already has thread routing; add a query param like `?msg=<id>` that the thread component reads on mount and uses to scroll the message into view.
- Use `simple` text search config (matches existing `search_vector` config). Do not switch to `english`/`czech` — content is mixed-language.
- Sanitization: render `content_snippet` via a strict HTML sanitizer that only allows `<mark>`. Strip everything else server-side before returning.
