CREATE TABLE cron_jobs (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    schedule TEXT NOT NULL,
    prompt TEXT NOT NULL,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    timeout_minutes INTEGER NOT NULL DEFAULT 30,
    model TEXT,
    last_run_at TIMESTAMPTZ,
    last_status TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE cron_executions (
    id BIGSERIAL PRIMARY KEY,
    cron_job_id BIGINT NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failed', 'timeout')),
    output TEXT,
    error_message TEXT,
    cost_usd DOUBLE PRECISION,
    input_tokens INTEGER,
    output_tokens INTEGER,
    duration_ms INTEGER,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_cron_executions_job_id ON cron_executions(cron_job_id, started_at DESC);
