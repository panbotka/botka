-- Task notes: free-form, timestamped human observations attached to a task.
-- Notes are append-only from a UX standpoint but support edit + soft delete.
-- They are NOT included in the context handed to Claude when the task runs —
-- this table is human-only metadata for triage, follow-ups, and outside context.

CREATE TABLE task_notes (
    id BIGSERIAL PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    author TEXT NOT NULL DEFAULT 'user',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_task_notes_task_id ON task_notes(task_id);
CREATE INDEX idx_task_notes_deleted_at ON task_notes(deleted_at);
