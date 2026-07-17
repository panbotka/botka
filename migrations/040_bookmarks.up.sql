-- App-level bookmarks: global (not per-thread) pinned links shown in the chat
-- header. Each bookmark stores its URL plus display metadata (page title and
-- favicon URL) fetched from the page when the bookmark is created.
CREATE TABLE bookmarks (
    id          BIGSERIAL PRIMARY KEY,
    url         TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    favicon_url TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ
);

CREATE INDEX idx_bookmarks_sort_order ON bookmarks (sort_order);
