-- Soft delete support for messages, attachments, branch_selections.
-- /clear and Regenerate set deleted_at; thread DELETE still hard-deletes via .Unscoped().

ALTER TABLE messages ADD COLUMN deleted_at TIMESTAMPTZ NULL;
ALTER TABLE attachments ADD COLUMN deleted_at TIMESTAMPTZ NULL;
ALTER TABLE branch_selections ADD COLUMN deleted_at TIMESTAMPTZ NULL;

CREATE INDEX idx_messages_deleted_at ON messages(deleted_at);
CREATE INDEX idx_attachments_deleted_at ON attachments(deleted_at);

-- Drop the existing unique constraint on (thread_id, fork_message_id) and replace
-- it with a partial unique index that only applies to non-deleted rows. Otherwise
-- soft-deleting a branch_selection and creating a new one for the same fork
-- would violate the constraint.
ALTER TABLE branch_selections DROP CONSTRAINT branch_selections_thread_id_fork_message_id_key;
CREATE UNIQUE INDEX idx_branch_thread_fork
    ON branch_selections(thread_id, fork_message_id)
    WHERE deleted_at IS NULL;
