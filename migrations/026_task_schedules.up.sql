CREATE TABLE task_schedules (
    id BIGSERIAL PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    spec TEXT NOT NULL DEFAULT '',
    cron_expression TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_schedules_due ON task_schedules(next_run_at) WHERE enabled = true;
CREATE INDEX idx_task_schedules_project ON task_schedules(project_id);

ALTER TABLE tasks
    ADD COLUMN schedule_id BIGINT REFERENCES task_schedules(id) ON DELETE SET NULL;

CREATE INDEX idx_tasks_schedule_id ON tasks(schedule_id) WHERE schedule_id IS NOT NULL;
