ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_status_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_status_check
    CHECK (status IN ('pending', 'queued', 'running', 'done', 'failed', 'needs_review', 'cancelled', 'deleted'));
