-- Thread forks: track threads forked from another thread at a specific message.
-- A fork is a deep copy of an existing thread up to and including a chosen
-- message. The fork keeps a back-reference to its source so the UI can show
-- "↳ forked from <parent>" and the source can list its forks.
--
-- Both columns are nullable; threads created normally have NULL on both. The
-- self-reference uses ON DELETE SET NULL so deleting a parent thread does not
-- cascade-destroy its forks. The message reference uses ON DELETE SET NULL so
-- soft-deleting (or hard-deleting) the original fork point leaves the fork
-- intact, just without a precise pointer.

ALTER TABLE threads
    ADD COLUMN parent_thread_id BIGINT REFERENCES threads(id) ON DELETE SET NULL,
    ADD COLUMN forked_from_message_id BIGINT REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX idx_threads_parent_thread_id ON threads(parent_thread_id)
    WHERE parent_thread_id IS NOT NULL;
