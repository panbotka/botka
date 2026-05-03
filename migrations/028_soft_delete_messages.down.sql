-- Reverse 028_soft_delete_messages.up.sql.

DROP INDEX IF EXISTS idx_branch_thread_fork;
ALTER TABLE branch_selections
    ADD CONSTRAINT branch_selections_thread_id_fork_message_id_key
    UNIQUE (thread_id, fork_message_id);

DROP INDEX IF EXISTS idx_attachments_deleted_at;
DROP INDEX IF EXISTS idx_messages_deleted_at;

ALTER TABLE branch_selections DROP COLUMN deleted_at;
ALTER TABLE attachments DROP COLUMN deleted_at;
ALTER TABLE messages DROP COLUMN deleted_at;
