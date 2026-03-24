# Botka

Merged app combining **Saiduler** (AI task scheduler) and **Chatovadlo** (Claude Code chat UI) into a single application for managing all Claude Code interactions.

## Tech Stack

- **Backend:** Go 1.25+, Gin router, GORM ORM, PostgreSQL 17
- **Frontend:** React 19, TypeScript, Vite 6, Tailwind CSS 4, Lucide icons
- **Database:** `botka` on shared PostgreSQL at `localhost:5432`
- **Port:** 5110
- **PWA:** vite-plugin-pwa with service worker

## Development

```bash
make run            # Run Go backend on :5110
make frontend-dev   # Run Vite dev server on :5173 (proxies /api to :5110)
make test           # Run Go tests with race detector
make lint           # Run golangci-lint
make build          # Production build (frontend + Go binary)
make deploy         # Build, stop service, copy binary, restart
```

## Architecture

### Two Claude Code Spawn Paths

1. **Chat mode** (`internal/claude/runner.go`): Interactive sessions with `--resume`, streams to browser SSE. Used by chat handlers.
2. **Task mode** (`internal/runner/executor.go`): Batch execution with process groups, timeout, retry, verification, PR creation. Used by task scheduler.

Both coexist — different lifecycle requirements justify separate implementations.

### Project/Folder Unification

Saiduler's `projects` and Chatovadlo's `folders` are merged into a single `projects` table. Projects serve dual roles:
- **For tasks:** Git repos with branch_strategy, verification_command
- **For chat:** Workspace directories with claude_md context, assigned to threads

### API Pattern

All endpoints under `/api/v1`. Response envelope: `{"data": T}` for items, `{"data": T[], "total": N}` for lists. SSE streaming for chat and task output.

### Key Packages

| Package | Purpose |
|---------|---------|
| `internal/config` | Environment config loading |
| `internal/database` | GORM connection + golang-migrate |
| `internal/models` | All GORM models |
| `internal/handlers` | Gin HTTP handlers |
| `internal/claude` | Chat subprocess runner + context assembly |
| `internal/runner` | Task scheduler + batch executor |
| `internal/projects` | Git repo discovery |
| `internal/mcp` | MCP server (tasks only) |

### Source Projects

Port code from these existing projects:
- **Saiduler** (`/home/pi/projects/saiduler`): Task scheduling, runner, MCP, frontend design system
- **Chatovadlo** (`/home/pi/projects/chatovadlo`): Chat UI, Claude subprocess, context assembly, personas, tags, memories
