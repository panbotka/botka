# Botka

AI-powered task scheduler and Claude Code chat interface — a unified app for managing both autonomous and interactive Claude Code sessions.

Botka merges two projects into one:
- **Saiduler** — autonomous task scheduling with Claude Code
- **Chatovadlo** — interactive chat UI for Claude Code

## Features

### Task Scheduling
- Create and queue coding tasks for autonomous execution
- Runner with start/pause/stop controls and configurable worker count
- Live task output streaming via SSE
- Anthropic API usage monitoring with adaptive rate limiting
- Project discovery from filesystem git repos
- Feature branch creation, verification commands, automatic PR creation
- Task prioritization with drag-and-drop reordering
- Batch status updates and retry logic with backoff
- MCP server for creating tasks from Claude Code sessions

### Chat Interface
- Real-time streaming chat with Claude Code
- Conversation branching and forking at any message
- Message editing with automatic branch creation
- Claude Code session continuity via `--resume` with pre-warmed subprocess pool (5-minute idle TTL)
- Personas with custom system prompts and default models
- Project-based workspaces (Claude runs in project directory)
- Hierarchical context assembly (SOUL.md, USER.md, MEMORY.md, daily notes, app memories, persona, project CLAUDE.md)
- File uploads (images, PDFs, text)
- Voice input with OpenClaw Whisper transcription
- Full-text search across all messages (PostgreSQL GIN index)
- Tags for thread organization
- App-level memories included in system prompts
- Message export (Markdown/JSON)
- Keyboard shortcuts and command palette

### Shared
- Unified project management (git repos + chat workspaces)
- PWA with offline support and mobile-optimized bottom navigation
- Clean design system (zinc/emerald/red/amber palette)
- Dark mode toggle
- MCP server (stdio + SSE transport)

## Quick Start

### Prerequisites

- Go 1.25+
- Node.js 22+
- PostgreSQL 17 (shared instance at `localhost:5432`)
- Claude Code CLI (`claude`)
- Optional: OpenClaw for voice transcription

### Setup

1. **Create the database:**
   ```bash
   psql -h localhost -U postgres -f scripts/create-db.sql
   ```
   Or manually:
   ```sql
   CREATE USER botka WITH PASSWORD 'botka';
   CREATE DATABASE botka OWNER botka;
   ```

2. **Install dependencies:**
   ```bash
   cd frontend && npm ci && cd ..
   ```

3. **Configure environment:**
   ```bash
   cp .env.example .env
   # Edit .env as needed
   ```

4. **Run in development:**
   ```bash
   make run            # Backend on :5110
   make frontend-dev   # Frontend on :5173 (in another terminal)
   ```

5. **Open** http://localhost:5173 (dev) or http://localhost:5110 (production)

## Configuration

All settings are loaded from `.env` file and environment variables (env vars take precedence). See `.env.example` for a complete template.

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `5110` | HTTP server port |
| `DATABASE_URL` | `postgres://botka:botka@localhost:5432/botka?sslmode=disable` | PostgreSQL connection string |

### Projects & Claude

| Variable | Default | Description |
|----------|---------|-------------|
| `PROJECTS_DIR` | `/home/pi/projects` | Directory to scan for git repos |
| `CLAUDE_PATH` | `claude` | Path to Claude Code CLI binary |
| `CLAUDE_CREDENTIALS_PATH` | `/home/pi/.claude/.credentials.json` | API credentials for usage monitoring |
| `CLAUDE_CONTEXT_DIR` | `./data/context` | Directory for assembled context files |
| `CLAUDE_DEFAULT_WORK_DIR` | `/home/pi` | Default working directory for Claude chat sessions |
| `AI_MODEL` | `sonnet` | Default Claude model for new chats |
| `AVAILABLE_MODELS` | `sonnet,opus,haiku` | Comma-separated list of available models |

### Task Runner

| Variable | Default | Description |
|----------|---------|-------------|
| `MAX_WORKERS` | `2` | Concurrent task execution slots |
| `USAGE_POLL_INTERVAL` | `15m` | How often to poll API usage stats |
| `USAGE_THRESHOLD_5H` | `0.90` | 5-hour rate limit threshold (0.0–1.0) |
| `USAGE_THRESHOLD_7D` | `0.95` | 7-day rate limit threshold (0.0–1.0) |

### Voice & Files

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENCLAW_URL` | `http://localhost:18789` | OpenClaw Whisper transcription endpoint |
| `OPENCLAW_TOKEN` | *(empty)* | OpenClaw API token |
| `OPENCLAW_WORKSPACE` | `/home/pi/.openclaw/workspace` | Path to SOUL.md, USER.md, MEMORY.md |
| `WHISPER_ENABLED` | `true` | Enable voice input transcription |
| `UPLOAD_DIR` | `./data/uploads` | Directory for uploaded files |

## Development

```bash
make run              # Run Go backend on :5110
make frontend-dev     # Run Vite dev server on :5173 (proxies /api to :5110)
make test             # Run Go tests with race detector
make lint             # Run golangci-lint (config: .golangci.yml)
make fmt              # Format code with goimports + gofmt
make vet              # Run go vet
make check            # Full CI gate: fmt + vet + lint + test
make build            # Build Go binary to build/botka
make prod-build       # Build frontend + Go binary to bin/botka
make clean            # Remove build artifacts
make frontend-install # Install frontend npm dependencies
make frontend-build   # Build frontend only
```

### Testing

~295 tests across 28 test files. Handler integration tests use a `botka_test` PostgreSQL database:

```bash
make test-db          # Create test database (one-time)
make test             # Run all tests with race detector
make check            # Full CI gate: fmt + vet + lint + test
```

Tests auto-skip when the test database is unavailable, so `make test` always passes without it.

### Database Migrations

Migrations run automatically on server startup. For manual control:

```bash
make migrate-up                         # Apply all pending migrations
make migrate-down                       # Rollback last migration
make migrate-create NAME=add_feature    # Create new migration pair
```

## Deployment

### Systemd (recommended)

```bash
make install-service  # Copy unit file and enable service
make deploy           # Build frontend + binary, restart service
```

The service runs as the `pi` user, reads `.env` from the project directory, and restarts on failure. Logs are available via `journalctl -u botka`.

### Docker

```bash
make docker-build     # Build multi-stage Docker image
make docker-up        # Start with docker compose
make docker-down      # Stop
```

The Docker setup connects to the shared PostgreSQL instance via the `shared-db` Docker network. It mounts the projects directory read-write and Claude config read-only.

### MCP Server

Botka can run as a Claude Code MCP server for creating tasks from other Claude Code sessions:

```bash
botka mcp    # Stdio mode: JSON-RPC 2.0 on stdin/stdout
```

The HTTP server also exposes an SSE-based MCP transport at `/mcp/sse`.

## API Overview

All endpoints are under `/api/v1`. Responses use a consistent envelope format:
- Single item: `{"data": T}`
- Lists: `{"data": T[], "total": N}`
- Errors: `{"error": "message"}`

### Endpoints

| Group | Endpoints | Description |
|-------|-----------|-------------|
| Projects | `GET/PUT /projects`, `POST /projects/scan` | List, update, and discover git repos |
| Tasks | `GET/POST/PUT/DELETE /tasks`, retry, batch-status, reorder | Task CRUD and lifecycle management |
| Runner | `GET /runner/status`, `POST /runner/start\|pause\|stop` | Task scheduler controls |
| Threads | `GET/POST/PUT/DELETE /threads`, pin, archive, tags | Chat conversation management |
| Chat | `POST /threads/:id/messages`, regenerate, edit, branch | Send messages, stream responses via SSE |
| Personas | `GET/POST/PUT/DELETE /personas` | Chat personality management |
| Tags | `GET/POST/PUT/DELETE /tags` | Thread label management |
| Memories | `GET/POST/PUT/DELETE /memories` | App-level persistent memories |
| Files | `GET /files/:id` | Serve and download uploaded files |
| Search | `GET /search?q=...` | Full-text search across messages |
| Transcribe | `GET /transcribe/status`, `POST /transcribe` | Voice transcription via OpenClaw |
| Processes | `GET/DELETE /processes` | Active Claude Code process management |
| Status | `GET /status`, `GET /models` | System status and model availability |
| MCP | `POST /mcp/sse`, `POST /mcp/sse/message` | MCP SSE transport |

## Architecture

```
┌─────────────────────────────────────────┐
│                 Frontend                 │
│  React 19 + Vite + Tailwind CSS 4 + PWA │
│                                         │
│  ┌───────┐ ┌────┐ ┌─────┐ ┌──────────┐ │
│  │ Dash  │ │Chat│ │Tasks│ │ Projects  │ │
│  │ board │ │    │ │     │ │ Settings  │ │
│  └───────┘ └────┘ └─────┘ └──────────┘ │
└────────────────┬────────────────────────┘
                 │ /api/v1, /mcp, SSE
┌────────────────┴────────────────────────┐
│              Go Backend (Gin)            │
│                                         │
│  ┌──────────┐  ┌────────────────────┐   │
│  │ Chat     │  │ Task Scheduler     │   │
│  │ Handlers │  │ Runner + Executor  │   │
│  └────┬─────┘  └────────┬──────────┘   │
│       │                  │              │
│  ┌────┴─────┐  ┌────────┴──────────┐   │
│  │ claude/  │  │ runner/executor    │   │
│  │ runner   │  │ (batch spawn)     │   │
│  │(interact)│  │                    │   │
│  └────┬─────┘  └────────┬──────────┘   │
│       │                  │              │
│  ┌────┴──────────────────┴──────────┐   │
│  │         Claude Code CLI          │   │
│  └──────────────────────────────────┘   │
│                                         │
│  ┌──────┐  ┌────────┐  ┌───────────┐   │
│  │ MCP  │  │Projects│  │ GORM/PG   │   │
│  │Server│  │Discovery│ │ Database  │   │
│  └──────┘  └────────┘  └───────────┘   │
└─────────────────────────────────────────┘
```

### Two Claude Code Spawn Paths

The app maintains two separate subprocess implementations because they have fundamentally different requirements:

1. **Chat mode** (`internal/claude/runner.go`, `pool.go`) — Interactive sessions with `--resume` for continuity, NDJSON stream parsing, SSE to browser. A session pool pre-warms subprocesses between messages to eliminate startup latency. Processes tracked in a registry.

2. **Task mode** (`internal/runner/executor.go`) — Batch execution with process groups for timeout/kill, retry logic with backoff, verification commands, and automatic PR creation. No session continuity.

### Key Design Decisions

- **Project/Folder unification:** Saiduler's projects and Chatovadlo's folders are merged into one `projects` table serving both roles.
- **One task per project:** The scheduler prevents concurrent task execution on the same project to avoid git conflicts.
- **Usage monitoring:** The runner polls Anthropic API usage and pauses task scheduling when approaching rate limits.
- **Hierarchical context:** Chat sessions get layered context (identity, user info, memories, persona, project CLAUDE.md, conversation history).
- **Session pool:** After each chat response, a new Claude process is pre-spawned with `--resume` and kept alive for 5 minutes. When the next message arrives, it pipes the prompt directly to the waiting process, saving ~2-3s of process startup. Sessions are automatically evicted on model/project changes or session resets.
- **Frontend embedded in binary:** Production builds embed `frontend/dist` via `go:embed` for single-binary deployment.
