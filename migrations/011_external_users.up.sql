-- Add role column to users table (admin = full access, external = restricted)
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'admin';

-- Create thread_access table for external user thread assignments
CREATE TABLE thread_access (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    thread_id BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, thread_id)
);

CREATE INDEX idx_thread_access_user_id ON thread_access(user_id);
CREATE INDEX idx_thread_access_thread_id ON thread_access(thread_id);
