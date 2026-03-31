# Botka

Merged app combining **Saiduler** (AI task scheduler) and **Chatovadlo** (Claude Code chat UI) into a single application for managing all Claude Code interactions.

## Tech Stack

- **Backend:** Go 1.25+, Gin router, GORM ORM, PostgreSQL 17
- **Frontend:** React 19, TypeScript, Vite 6, Tailwind CSS 4, Lucide icons
- **Database:** `botka` on shared PostgreSQL at `localhost:5432`
- **Port:** 5110
- **PWA:** vite-plugin-pwa with service worker

## Directory Structure

```
cmd/server/          Entry point (HTTP server + MCP stdio mode)
cmd/migrate-data/    Data migration utilities from source projects
internal/
  config/            Environment config loading (.env + env vars)
  database/          GORM connection + golang-migrate migrations
  models/            All GORM models (20 models)
  handlers/          Gin HTTP handlers (24 handler files)
  claude/            Chat subprocess runner + context assembly
  runner/            Task scheduler loop + batch executor
  projects/          Git repo discovery and DB sync
  mcp/               MCP server (stdio + SSE transport)
  middleware/        HTTP middleware (auth, CORS, role-based access)
  static/            Frontend static file serving (go:embed)
frontend/src/
  api/               API client methods
  components/        React components (36 files)
  context/           React context (SSEContext, SettingsContext)
  hooks/             Custom React hooks (20 hooks)
  pages/             Page components (10 pages)
  types/             TypeScript type definitions
  utils/             Utility functions
migrations/          SQL migration files (golang-migrate)
packaging/           systemd service file
scripts/             Database creation scripts
data/                Runtime data (uploads, context assembly)
```

## Database

Botka uses the `botka` database on shared PostgreSQL (`localhost:5432`). To create it:

```bash
psql -h localhost -U postgres -f scripts/create-db.sql
```

This creates user `botka` with password `botka` and database `botka`.

Migrations run automatically on startup via golang-migrate. Manual control:

```bash
make migrate-up      # Apply all pending migrations
make migrate-down    # Rollback last migration
make migrate-create NAME=add_foo  # Create new migration pair
```

## Development

```bash
make run            # Run Go backend on :5110
make frontend-dev   # Run Vite dev server on :5173 (proxies /api to :5110)
make test           # Run Go tests with race detector
make lint           # Run golangci-lint (config: .golangci.yml)
make fmt            # Format code with goimports + gofmt
make check          # Full CI gate: fmt + vet + lint + test + frontend type-check
make build          # Build Go binary to build/botka
make prod-build     # Build frontend + Go binary to bin/botka
make clean          # Remove build artifacts
```

**Before every commit, `make check` must pass.** This runs Go formatting, vetting, linting, tests, and frontend TypeScript type-checking. Do not commit code that fails this gate.

## Testing

**~636 tests** across 58 test files covering all packages. Go tests use stdlib `testing`; frontend tests use Vitest.

```bash
make test           # Run all tests with race detector
make test-db        # Create botka_test database (run once)
make check          # Full CI gate: fmt + vet + lint + test
```

### Test Database

Handler integration tests require a `botka_test` PostgreSQL database. Create it once:

```bash
make test-db
```

Tests auto-skip when the database is unavailable, so `make test` always passes. The `DATABASE_TEST_URL` env var controls the connection string (default: `postgres://botka:botka@localhost:5432/botka_test?sslmode=disable`).

### Test Structure

- **Unit tests** (no DB): `config`, `middleware`, `runner` (buffer, parser, executor, usage), `projects`
- **Integration tests** (need `botka_test`): all `handlers` — HTTP-level tests via `httptest` + Gin test mode
- **Model/package tests**: `models` (enums, table names), `mcp` (JSON-RPC, tools, SSE), `claude` (event parsing, context assembly, registry)
- **Frontend tests**: Vitest unit tests in `frontend/src/` (94 tests across 10 files)

### Linting

golangci-lint v2 configured via `.golangci.yml` with strict linters: errcheck, govet, staticcheck, gocritic, revive, misspell, bodyclose, unconvert, whitespace, predeclared.

## Deployment

```bash
make install-service  # Install and enable systemd service
make deploy           # Build and deploy binary to /usr/local/bin
make docker-build     # Build Docker image
make docker-up        # Start with Docker Compose
make docker-down      # Stop Docker Compose
```

## Architecture

### Two Claude Code Spawn Paths

The app has two separate Claude Code subprocess implementations. This is intentional — they serve fundamentally different use cases with different lifecycle requirements:

1. **Chat mode** (`internal/claude/runner.go`): Spawns interactive Claude Code sessions for the chat UI. Uses `--resume` for session continuity, streams NDJSON events parsed into typed `StreamEvent` values, and sends them to the browser via SSE. Processes are tracked in a registry (`internal/claude/registry.go`). A session pool (`internal/claude/pool.go`) pre-warms subprocesses between messages so subsequent responses skip process startup.

2. **Task mode** (`internal/runner/executor.go`): Spawns batch Claude Code sessions for autonomous task execution. Uses process groups (`Setpgid`) for reliable timeout/kill, has retry logic with backoff, runs optional verification commands, and creates PRs on feature branches. No session continuity — each execution is standalone.

### Project/Folder Unification

Saiduler's `projects` and Chatovadlo's `folders` are merged into a single `projects` table. Projects serve dual roles:
- **For tasks:** Git repos with `branch_strategy`, `verification_command`
- **For chat:** Workspace directories with `claude_md` context, assigned to threads

### Hierarchical Context Assembly

Chat messages are enriched with hierarchical context assembled in `internal/claude/context.go`:
1. SOUL.md (identity) from OpenClaw workspace
2. USER.md (user info)
3. MEMORY.md (operational memory)
4. Recent daily notes (last 3 days)
5. App memories from database
6. Thread system prompt (persona or custom)
7. Project CLAUDE.md
8. Conversation history (last 200 messages, truncated)

### API Pattern

All endpoints under `/api/v1`. Response envelope:
- Single item: `{"data": T}`
- List: `{"data": T[], "total": N}`
- Error: `{"error": "message"}`

SSE streaming for chat messages and live task output.

### MCP Server

Two transports:
- **Stdio:** `botka mcp` — JSON-RPC 2.0 on stdin/stdout, for use as Claude Code MCP server
- **SSE:** `/mcp/sse` — HTTP SSE transport, requires `Authorization: Bearer <MCP_TOKEN>` header

Exposes task management tools: create_task, list_tasks, get_task, update_task, list_projects.

### Key Packages

| Package | Purpose |
|---------|---------|
| `internal/config` | Environment config loading from `.env` + env vars |
| `internal/database` | GORM connection + golang-migrate |
| `internal/models` | All GORM models (Project, Task, Thread, Message, etc.) |
| `internal/handlers` | Gin HTTP handlers for all API endpoints |
| `internal/claude` | Chat subprocess runner, session pool, and hierarchical context assembly |
| `internal/runner` | Task scheduler loop + batch executor + usage monitor |
| `internal/projects` | Git repo filesystem discovery and DB sync |
| `internal/mcp` | MCP server (stdio + SSE transport) |
| `internal/middleware` | Auth, CORS, and role-based access middleware |
| `internal/static` | Embedded frontend file serving |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `5110` | HTTP server port |
| `DATABASE_URL` | `postgres://botka:botka@localhost:5432/botka?sslmode=disable` | PostgreSQL connection string |
| `PROJECTS_DIR` | `/home/pi/projects` | Directory to scan for git repos |
| `CLAUDE_PATH` | `claude` | Path to Claude Code CLI binary |
| `CLAUDE_USAGE_CMD` | `/home/pi/bin/claude-usage` | Command to fetch API usage data as JSON |
| `CLAUDE_CONTEXT_DIR` | `./data/context` | Directory for assembled context files |
| `CLAUDE_DEFAULT_WORK_DIR` | `/home/pi` | Default working directory for Claude chat sessions |
| `MAX_WORKERS` | `2` | Concurrent task execution slots |
| `USAGE_THRESHOLD_5H` | `0.90` | 5-hour rate limit threshold (fraction) |
| `USAGE_THRESHOLD_7D` | `0.95` | 7-day rate limit threshold (fraction) |
| `OPENCLAW_URL` | `http://localhost:18789` | OpenClaw Whisper transcription endpoint |
| `OPENCLAW_TOKEN` | *(empty)* | OpenClaw API token |
| `OPENCLAW_WORKSPACE` | `/home/pi/.openclaw/workspace` | Path to SOUL.md, USER.md, MEMORY.md |
| `WHISPER_ENABLED` | `true` | Enable voice input transcription |
| `UPLOAD_DIR` | `./data/uploads` | Directory for uploaded files |
| `AI_MODEL` | `sonnet` | Default Claude model for new chats |
| `AVAILABLE_MODELS` | `sonnet,opus,haiku` | CSV list of available models |
| `WEBAUTHN_ORIGIN` | `http://localhost:5110` | WebAuthn relying party origin |
| `WEBAUTHN_RPID` | *(derived from origin)* | WebAuthn relying party ID (hostname) |
| `SESSION_MAX_AGE` | `720h` | Authentication session cookie max age |
| `MCP_TOKEN` | *(empty)* | Bearer token for MCP SSE transport (empty = SSE disabled) |
| `KEEPALIVE_ENABLED` | `true` | Enable periodic Claude Code ping to keep 5h rate limit window active |
| `KEEPALIVE_INTERVAL` | `60m` | Interval between keepalive pings (Go duration) |

## Task Agent Safety

**CRITICAL: Running `make deploy`, `make install-service`, `systemctl restart botka`, or `systemctl stop botka` WILL KILL THIS PROCESS and leave the task stuck forever. This applies to ALL commands that restart the botka systemd service.**

Task agents run inside Botka — deploying or restarting would kill the agent's own process. Only build and test, never deploy.

If your task requires deploying changes, mark the task as done and note that deployment is needed — the user will deploy manually.

**CRITICAL: Never run a second botka process (e.g. `make run`, `go run ./cmd/server`) while the systemd service is running.** Two processes sharing the same database will both run scheduler loops independently, causing concurrent task execution on the same project. The unique partial index `idx_one_running_per_project` (migration 016) prevents data corruption, but the duplicate work and git conflicts are still harmful. Always stop the service first: `sudo systemctl stop botka`.

## Important Patterns

- **Frontend embeds in binary:** `frontend/dist` is embedded via `go:embed` in the root `embed.go`. The `ensure-dist` Makefile target creates a placeholder so `go build` works without building frontend first.
- **GORM models** use `uuid.UUID` primary keys for tasks/projects and `int64` (bigserial) for chat entities (threads, messages, personas, tags).
- **Full-text search** on messages uses a PostgreSQL GIN index on a `tsvector` column.
- **Task scheduling** uses `SELECT ... FOR UPDATE SKIP LOCKED` for safe concurrent task picking.
- **One task per project:** Enforced at **three levels** — (1) in-memory executors map keyed by `project_id`, (2) `NOT EXISTS` subquery in the pick query, and (3) a **unique partial index** `idx_one_running_per_project ON tasks (project_id) WHERE status = 'running'` (migration 016). The DB-level index is critical: the in-memory guard only protects within a single process, and the `NOT EXISTS` subquery has a TOCTOU race under concurrent transactions. If two processes share the database, only the unique index prevents them from both claiming tasks for the same project. The runner handles the resulting unique violation gracefully in `pickNextTask()` (skips and retries next tick). **Never remove this index.**
- **MAX_WORKERS enforcement:** `launchTask()` re-checks `len(r.executors) >= r.maxWorkers` under the mutex before adding a new executor. This is the authoritative enforcement point — if the limit is reached, the task is requeued. The earlier check in `collectTickState()` is an optimization to avoid unnecessary DB queries; `launchTask()` is the safety net that prevents over-allocation even if multiple processes share the database.
- **Session pool:** After each chat response, a pre-warmed Claude process is spawned with `--resume` and kept alive for 5 minutes via a stdin keepalive byte. The next message pipes its prompt to the waiting process, skipping startup. Sessions are evicted on model/project changes, session clears, or thread deletion.
- **Session validation:** Claude Code stores sessions per working directory at `~/.claude/projects/<encoded-dir>/<id>.jsonl`. Before resuming, `SessionExists()` checks the file exists for the current directory. Changing a thread's project clears the session ID. Stale session errors ("No conversation found") auto-clear the session for the next attempt.

## Source Projects

Both source projects are available on this machine for reference:

- **Saiduler** (`/home/pi/projects/saiduler`): Task scheduling, runner, MCP, frontend design system
- **Chatovadlo** (`/home/pi/projects/chatovadlo`): Chat UI, Claude subprocess, context assembly, personas, tags, memories

When implementing a feature, read the corresponding source file first rather than writing from scratch.
