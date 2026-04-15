CREATE TABLE mcp_servers (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    server_type TEXT NOT NULL CHECK (server_type IN ('stdio', 'sse')),
    config JSONB NOT NULL DEFAULT '{}',
    is_default BOOLEAN NOT NULL DEFAULT false,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE thread_mcp_servers (
    thread_id BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    mcp_server_id BIGINT NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
    PRIMARY KEY (thread_id, mcp_server_id)
);

CREATE TABLE project_mcp_servers (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    mcp_server_id BIGINT NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
    PRIMARY KEY (project_id, mcp_server_id)
);
