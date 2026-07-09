-- Global registry of Claude Code skills discovered on disk.
CREATE TABLE skills (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(200) NOT NULL UNIQUE,
    description     TEXT NOT NULL DEFAULT '',
    source          VARCHAR(200) NOT NULL DEFAULT 'user',
    default_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ
);

CREATE INDEX idx_skills_active ON skills (active);

-- Per-thread overrides of a skill's default_enabled. A missing row means the
-- thread inherits the skill's current default.
CREATE TABLE thread_skills (
    thread_id  BIGINT NOT NULL REFERENCES threads (id) ON DELETE CASCADE,
    skill_name VARCHAR(200) NOT NULL,
    enabled    BOOLEAN NOT NULL,
    PRIMARY KEY (thread_id, skill_name)
);

CREATE INDEX idx_thread_skills_skill_name ON thread_skills (skill_name);
