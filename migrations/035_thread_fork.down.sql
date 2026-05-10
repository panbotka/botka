DROP INDEX IF EXISTS idx_threads_parent_thread_id;
ALTER TABLE threads
    DROP COLUMN IF EXISTS forked_from_message_id,
    DROP COLUMN IF EXISTS parent_thread_id;
