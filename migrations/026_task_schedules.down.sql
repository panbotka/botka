DROP INDEX IF EXISTS idx_tasks_schedule_id;
ALTER TABLE tasks DROP COLUMN IF EXISTS schedule_id;

DROP INDEX IF EXISTS idx_task_schedules_due;
DROP INDEX IF EXISTS idx_task_schedules_project;
DROP TABLE IF EXISTS task_schedules;
