# Botka — Technical Specification

## Overview

Botka is a unified web application that merges two existing projects:
- **Saiduler** — AI task scheduler that queues and runs Claude Code sessions autonomously
- **Chatovadlo** — Interactive chat UI for Claude Code with full agentic capabilities

Both projects manage Claude Code subprocess sessions. Merging them provides a single interface for all Claude Code interactions — scheduled batch tasks and interactive conversations.

## Design Decisions

### Use Saiduler's Design
- Frontend design system: zinc/emerald/red/amber color palette on zinc-50 backgrounds
- Sidebar navigation with 5 sections: Dashboard, Chat, Tasks, Projects, Settings
- PWA-optimized (installable, offline static assets, mobile-responsive)

### Use Saiduler's Backend Patterns
- **Router:** Gin (not Chi)
- **ORM:** GORM (not raw pgx)
- **Migrations:** golang-migrate with embedded SQL files
- **API:** `/api/v1` prefix, `{"data": T}` response envelope

### Keep All Features from Both Projects
- Every feature from Saiduler and Chatovadlo must be preserved
- No auth for now (can be added later)
- OpenClaw integration kept for chat only (not tasks)
- MCP server exposes task tools only (not chat)

### Single Database
- New `botka` database on shared PostgreSQL (localhost:5432)
- Fresh schema merging tables from both apps

## Database Schema

### Unified Projects Table

Saiduler's `projects` + Chatovadlo's `folders` → single `projects` table:

| Column | Type | Source | Purpose |
|--------|------|--------|---------|
| id | UUID PK | both | Primary key |
| name | VARCHAR(255) | both | Display name |
| path | VARCHAR(1024) UNIQUE | both | Filesystem path |
| branch_strategy | VARCHAR(20) | Saiduler | 'main' or 'feature_branch' |
| verification_command | TEXT | Saiduler | Post-execution verification |
| active | BOOLEAN | Saiduler | Eligible for task scheduling |
| claude_md | TEXT | Chatovadlo | Per-project system prompt context |
| sort_order | INTEGER | Chatovadlo | UI ordering for chat |
| created_at, updated_at | TIMESTAMPTZ | both | Timestamps |

### Task Tables (from Saiduler)

- **tasks** — id, title, spec, status (7 states), priority, project_id, failure_reason, retry_count
- **task_executions** — id, task_id, attempt, started_at, finished_at, exit_code, cost_usd, duration_ms, summary, error_message
- **runner_state** — singleton row tracking runner state (running/paused/stopped), completed_count, task_limit

### Chat Tables (from Chatovadlo)

- **threads** — id, title, model, system_prompt, persona_id, project_id (was folder_id), pinned, archived, claude_session_id
- **messages** — id, thread_id, role, content, parent_id (for branching), thinking, token counts, tsv (full-text search)
- **attachments** — id, message_id, stored_name, original_name, mime_type, size
- **branch_selections** — thread_id, fork_message_id, selected_child_id (tracks active branch at each fork)

### Supporting Tables (from Chatovadlo)

- **personas** — id, name, system_prompt, default_model, icon, starter_message, sort_order
- **tags** — id, name, color
- **thread_tags** — thread_id, tag_id (many-to-many)
- **memories** — id (UUID), content (included in chat system prompts)

## API Routes

### Tasks & Runner (`/api/v1/tasks/*`, `/api/v1/runner/*`)
- Task CRUD with batch status updates and priority reordering
- Runner start/pause/stop controls
- Live task output streaming via SSE
- Usage monitoring (5h and 7d windows)

### Chat (`/api/v1/threads/*`)
- Thread CRUD with pin, archive, model selection, project assignment
- Send message → SSE streaming response (content deltas, thinking, tool calls, usage)
- Message editing → automatic branch creation
- Branch creation and switching
- Session management (clear, new)
- File upload (multipart/form-data)

### Projects (`/api/v1/projects/*`)
- List, get, update projects
- Scan filesystem for git repos

### Supporting
- Tags, Personas, Memories — standard CRUD
- Full-text search across messages (PostgreSQL tsvector)
- File serve/download
- Transcription (OpenClaw Whisper proxy)
- Active Claude process management (list, kill)
- Available models, server status

### MCP (`/mcp/*`)
- SSE transport for Model Context Protocol
- Task management tools only (create, list, get, update tasks; list projects; runner status/start)

## Claude Code Subprocess Management

### Chat Mode (Interactive)
- Spawns `claude -p --output-format stream-json --include-partial-messages --dangerously-skip-permissions`
- Session continuity via `--resume <session-id>` / `--session-id <id>`
- Context assembled from 8 layers: SOUL.md, USER.md, MEMORY.md, daily notes, app memories, persona prompt, project CLAUDE.md, conversation history
- Passed via `--append-system-prompt-file`
- Process registered in ProcessRegistry for kill functionality
- Streams parsed events to browser via SSE

### Task Mode (Batch)
- Spawns `claude --dangerously-skip-permissions --verbose --output-format stream-json -p <prompt>`
- Process group management (Setpgid) for clean termination
- 30-minute execution timeout, 5-minute verification timeout
- Spec file synced to `docs/specs/task-<id>.md` in project directory
- Automatic feature branch creation and PR creation
- Retry logic with failure classification (API errors vs crashes)
- Output captured in ring buffer for live viewing

## Frontend Structure

### Navigation (Sidebar)
1. **Dashboard** — Runner status, API usage meters, active/recent tasks
2. **Chat** — Split panel: thread list (left) + chat view (right); single-panel on mobile
3. **Tasks** — Filterable task list with drag-drop reorder, task detail with execution history
4. **Projects** — Project management with task counts and chat configuration
5. **Settings** — Personas, tags, memories, theme, model defaults, voice

### Design System
- Tailwind CSS 4 with zinc/emerald/red/amber palette
- Lucide React icons
- Status-colored badges (emerald=done, red=failed, amber=running, blue=queued, purple=needs_review)
- Dark mode toggle
- PWA with service worker caching static assets

### Key Frontend Features
- Real-time SSE streaming for both chat and task output
- Drag-and-drop task reordering (dnd-kit)
- Conversation branching UI with fork indicators
- Markdown rendering with syntax highlighting
- File upload (drag-drop + click)
- Voice input (Web Audio API + Whisper)
- Command palette (Ctrl/Cmd+K)
- Keyboard shortcuts
- Mobile bottom navigation
- Thread export (Markdown/JSON)

## Configuration

All via environment variables (`.env` file):

| Variable | Default | Description |
|----------|---------|-------------|
| PORT | 5110 | HTTP server port |
| DATABASE_URL | postgres://botka:botka@localhost:5432/botka | PostgreSQL connection |
| PROJECTS_DIR | /home/pi/projects | Git repo discovery directory |
| CLAUDE_PATH | claude | Path to Claude CLI binary |
| CLAUDE_CREDENTIALS_PATH | /home/pi/.claude/.credentials.json | For API usage monitoring |
| MAX_WORKERS | 2 | Concurrent task execution slots |
| USAGE_POLL_INTERVAL | 15m | API usage check interval |
| USAGE_THRESHOLD_5H | 0.90 | 5-hour usage warning threshold |
| USAGE_THRESHOLD_7D | 0.95 | 7-day usage warning threshold |
| OPENCLAW_URL | http://localhost:18789 | OpenClaw API for context + whisper |
| OPENCLAW_WORKSPACE | /home/pi/.openclaw/workspace | SOUL.md, USER.md, MEMORY.md location |
| CLAUDE_CONTEXT_DIR | ./data/context | Assembled context file directory |
| CLAUDE_DEFAULT_WORKDIR | /home/pi | Default working directory for chat |
| WHISPER_ENABLED | true | Enable voice transcription |
| UPLOAD_DIR | ./data/uploads | File upload storage |
| AI_MODEL | sonnet | Default AI model |
| AVAILABLE_MODELS | sonnet,opus,haiku | Models shown in picker |

## Deployment

- **Docker:** Multi-stage build (Node → Go → Alpine), port 5110, shared-db network
- **Systemd:** Service unit at `packaging/botka.service`
- **Database:** Shared PostgreSQL instance, `botka` database
