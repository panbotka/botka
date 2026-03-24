-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Projects (unified: Saiduler projects + Chatovadlo folders)
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    path VARCHAR(1024) UNIQUE NOT NULL,
    branch_strategy VARCHAR(20) NOT NULL DEFAULT 'main'
        CHECK (branch_strategy IN ('main', 'feature_branch')),
    verification_command TEXT,
    active BOOLEAN NOT NULL DEFAULT true,
    claude_md TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Tasks (from Saiduler)
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(500) NOT NULL,
    spec TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'queued', 'running', 'done', 'failed', 'needs_review', 'cancelled')),
    priority INTEGER NOT NULL DEFAULT 0,
    project_id UUID NOT NULL REFERENCES projects(id),
    failure_reason TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_priority ON tasks(priority DESC);
CREATE INDEX idx_tasks_project_id ON tasks(project_id);

-- Task Executions (from Saiduler)
CREATE TABLE task_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    attempt INTEGER NOT NULL DEFAULT 1,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    exit_code INTEGER,
    cost_usd NUMERIC(10,6),
    duration_ms BIGINT,
    summary TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_task_executions_task_id ON task_executions(task_id);

-- Runner State (from Saiduler)
CREATE TABLE runner_state (
    id INTEGER PRIMARY KEY DEFAULT 1,
    state TEXT NOT NULL DEFAULT 'stopped' CHECK (state IN ('running', 'paused', 'stopped')),
    completed_count INTEGER NOT NULL DEFAULT 0,
    task_limit INTEGER,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO runner_state (id, state) VALUES (1, 'stopped');

-- Personas (from Chatovadlo)
CREATE TABLE personas (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    system_prompt TEXT NOT NULL DEFAULT '',
    default_model VARCHAR(100),
    icon VARCHAR(10),
    starter_message TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Threads (from Chatovadlo, folder_id renamed to project_id)
CREATE TABLE threads (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(500) NOT NULL DEFAULT 'New Chat',
    model VARCHAR(100),
    system_prompt TEXT NOT NULL DEFAULT '',
    persona_id BIGINT REFERENCES personas(id) ON DELETE SET NULL,
    persona_name VARCHAR(255) NOT NULL DEFAULT '',
    project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    pinned BOOLEAN NOT NULL DEFAULT false,
    archived BOOLEAN NOT NULL DEFAULT false,
    claude_session_id VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_threads_project_id ON threads(project_id);

-- Messages (from Chatovadlo)
CREATE TABLE messages (
    id BIGSERIAL PRIMARY KEY,
    thread_id BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content TEXT NOT NULL DEFAULT '',
    parent_id BIGINT REFERENCES messages(id) ON DELETE SET NULL,
    thinking TEXT,
    thinking_duration_ms INTEGER,
    prompt_tokens INTEGER,
    completion_tokens INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_messages_thread_id ON messages(thread_id);
CREATE INDEX idx_messages_thread_created ON messages(thread_id, created_at);
CREATE INDEX idx_messages_parent_id ON messages(parent_id);

-- Full-text search on messages
ALTER TABLE messages ADD COLUMN tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;
CREATE INDEX idx_messages_tsv ON messages USING gin(tsv);

-- Attachments (from Chatovadlo)
CREATE TABLE attachments (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    stored_name VARCHAR(500) NOT NULL,
    original_name VARCHAR(500) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    size BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_attachments_message_id ON attachments(message_id);

-- Branch Selections (from Chatovadlo)
CREATE TABLE branch_selections (
    id BIGSERIAL PRIMARY KEY,
    thread_id BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    fork_message_id BIGINT NOT NULL DEFAULT 0,
    selected_child_id BIGINT NOT NULL,
    UNIQUE(thread_id, fork_message_id)
);

-- Tags (from Chatovadlo)
CREATE TABLE tags (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    color VARCHAR(7) NOT NULL DEFAULT '#6b7280',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Thread Tags (from Chatovadlo)
CREATE TABLE thread_tags (
    thread_id BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    tag_id BIGINT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (thread_id, tag_id)
);
CREATE INDEX idx_thread_tags_tag_id ON thread_tags(tag_id);

-- Memories (from Chatovadlo)
CREATE TABLE memories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
