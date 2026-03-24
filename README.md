# Botka

AI-powered task scheduler and Claude Code chat interface — a unified app for managing both autonomous and interactive Claude Code sessions.

## Features

### Task Scheduling (from Saiduler)
- Create and queue coding tasks for autonomous execution
- Runner with start/pause/stop controls and worker management
- Live task output streaming via SSE
- Anthropic API usage monitoring with adaptive polling
- Project discovery from filesystem git repos
- Feature branch creation, verification commands, automatic PR creation
- Task prioritization with drag-and-drop reordering
- Batch status updates and retry logic
- MCP server for creating tasks from Claude Code sessions

### Chat Interface (from Chatovadlo)
- Real-time streaming chat with Claude Code
- Conversation branching and forking at any message
- Message editing with automatic branch creation
- Claude Code session continuity via `--resume`
- Personas with custom system prompts and default models
- Project-based workspaces (Claude runs in project directory)
- Hierarchical context assembly (SOUL.md, USER.md, MEMORY.md, daily notes, app memories, persona, project CLAUDE.md)
- File uploads (images, PDFs, text)
- Voice input with OpenClaw Whisper transcription
- Full-text search across all messages
- Tags for thread organization
- App-level memories included in system prompts
- Message export (Markdown/JSON)
- Keyboard shortcuts and command palette

### Shared
- Unified project management (git repos + chat workspaces)
- PWA with offline support and mobile-optimized layout
- Saiduler's clean design system (zinc/emerald/red/amber)
- Dark mode toggle

## Quick Start

### Prerequisites
- Go 1.25+
- Node.js 22+
- PostgreSQL 17 (shared instance at localhost:5432)
- Claude Code CLI (`claude`)

### Setup

1. Create the database:
   ```sql
   CREATE USER botka WITH PASSWORD 'botka';
   CREATE DATABASE botka OWNER botka;
   ```

2. Configure environment:
   ```bash
   cp .env.example .env
   # Edit .env as needed
   ```

3. Run:
   ```bash
   make run            # Backend on :5110
   make frontend-dev   # Frontend on :5173 (in another terminal)
   ```

4. Open http://localhost:5173 (dev) or http://localhost:5110 (production)

## Configuration

See `.env.example` for all available environment variables.

Key settings:
| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 5110 | Server port |
| `DATABASE_URL` | postgres://botka:botka@localhost:5432/botka | PostgreSQL connection |
| `PROJECTS_DIR` | /home/pi/projects | Directory to scan for git repos |
| `MAX_WORKERS` | 2 | Concurrent task execution slots |
| `AI_MODEL` | sonnet | Default Claude model for chat |
| `OPENCLAW_WORKSPACE` | /home/pi/.openclaw/workspace | Path to SOUL.md, USER.md, etc. |

## Deployment

### Systemd
```bash
make deploy          # Build + deploy to systemd service
make install-service # Install systemd unit file
```

### Docker
```bash
docker compose up -d
```

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
│  │ (interactive)                    │   │
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
