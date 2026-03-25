-- Add started_at and completed_at columns to tasks table
ALTER TABLE tasks ADD COLUMN started_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN completed_at TIMESTAMPTZ;

-- Backfill from task_executions: first execution's started_at, last execution's finished_at
UPDATE tasks t
SET started_at = sub.first_started,
    completed_at = sub.last_finished
FROM (
    SELECT task_id,
           MIN(started_at) AS first_started,
           MAX(finished_at) AS last_finished
    FROM task_executions
    GROUP BY task_id
) sub
WHERE t.id = sub.task_id
  AND t.status IN ('done', 'failed', 'needs_review', 'cancelled');
