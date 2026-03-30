-- Clean up any existing violations (keep earliest, requeue the rest).
UPDATE tasks
SET status = 'queued',
    failure_reason = 'recovered: duplicate running task cleaned during migration 016'
WHERE id IN (
    SELECT id FROM (
        SELECT id, ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY created_at ASC) AS rn
        FROM tasks
        WHERE status = 'running'
    ) sub WHERE rn > 1
);

-- Enforce one running task per project at the database level.
-- Prevents TOCTOU races when multiple processes share the same database.
CREATE UNIQUE INDEX idx_one_running_per_project ON tasks (project_id) WHERE status = 'running';
