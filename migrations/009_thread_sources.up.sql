CREATE TABLE thread_sources (
    id BIGSERIAL PRIMARY KEY,
    thread_id BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    position INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_thread_sources_thread_id ON thread_sources(thread_id);
