-- Thread folders: hierarchical organization for chat threads.
-- Folders can be nested up to 5 levels deep (enforced at the API layer).
-- A thread with folder_id = NULL lives at the root level of the sidebar.

CREATE TABLE thread_folders (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    parent_id BIGINT REFERENCES thread_folders(id) ON DELETE RESTRICT,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_thread_folders_parent_id ON thread_folders(parent_id);
CREATE INDEX idx_thread_folders_parent_position ON thread_folders(parent_id, position);

ALTER TABLE threads
    ADD COLUMN folder_id BIGINT REFERENCES thread_folders(id) ON DELETE SET NULL;

CREATE INDEX idx_threads_folder_id ON threads(folder_id);
