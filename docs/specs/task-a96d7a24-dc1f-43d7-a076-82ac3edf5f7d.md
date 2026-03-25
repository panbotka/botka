## Task 1: Go Project Scaffold

Create the foundational Go project for Botka — a merged app combining Saiduler (AI task scheduler) and Chatovadlo (Claude Code chat UI).

### Context
Botka merges two existing Go+React projects into one. This task creates the skeleton that all subsequent tasks build on. Use Saiduler's patterns as the primary reference.

### Reference Files
- `/home/pi/projects/saiduler/cmd/server/main.go` — server entry point pattern
- `/home/pi/projects/saiduler/internal/config/config.go` — config loading pattern
- `/home/pi/projects/saiduler/internal/database/database.go` — GORM connection
- `/home/pi/projects/saiduler/internal/database/migrate.go` — golang-migrate
- `/home/pi/projects/saiduler/internal/middleware/cors.go` — CORS middleware
- `/home/pi/projects/saiduler/internal/static/static.go` — SPA file serving
- `/home/pi/projects/saiduler/frontend_embed.go` — embed directive
- `/home/pi/projects/saiduler/Makefile` — build targets
- `/home/pi/projects/saiduler/.gitignore`

### What to Create

1. **`go.mod`** — `module botka`, Go 1.25+. Dependencies: gin, gorm, gorm postgres driver, golang-migrate, google/uuid, slog.

2. **`cmd/server/main.go`** — Entry point modeled on Saiduler's:
   - `main()`: check for "mcp" subcommand (log to stderr), otherwise call `run()`
   - `run()`: load config → run migrations → connect DB → setup Gin router → start server
   - `setupRouter()`: create Gin engine with Recovery, Logger, CORS middleware. Register `/api/health` endpoint returning `{"status": "ok"}`. Setup frontend static file serving.
   - `startServer()`: HTTP server on configured port with graceful shutdown (SIGINT/SIGTERM, 10s timeout)
   - `initFrontendFS()`: extract embedded frontend/dist sub-filesystem
   - NOTE: The `runMCP()` function and runner initialization should be stubbed out with TODO comments for now — they will be implemented in later tasks.

3. **`internal/config/config.go`** — Merged config struct loading from environment:
   ```go
   type Config struct {
       Port                  string // default: "5110"
       DatabaseURL           string // default: "postgres://botka:botka@localhost:5432/botka?sslmode=disable"
       ProjectsDir           string // default: "/home/pi/projects"
       ClaudePath            string // default: "claude"
       ClaudeCredentialsPath string // default: "/home/pi/.claude/.credentials.json"
       MaxWorkers            int    // default: 2
       UsagePollInterval     time.Duration // default: 15m
       UsageThreshold5h      float64 // default: 0.90
       UsageThreshold7d      float64 // default: 0.95
       OpenClawURL           string // default: "http://localhost:18789"
       OpenClawToken         string
       OpenClawWorkspace     string // default: "/home/pi/.openclaw/workspace"
       ClaudeContextDir      string // default: "./data/context"
       ClaudeDefaultWorkDir  string // default: "/home/pi"
       WhisperEnabled        bool   // default: true
       UploadDir             string // default: "./data/uploads"
       AIModel               string // default: "sonnet"
       AvailableModels       []string // default: ["sonnet","opus","haiku"]
   }
   ```
   Use `os.Getenv` with defaults. Parse duration, int, float, bool, CSV as needed. Load `.env` file if present (use godotenv or manual parsing like Saiduler does).

4. **`internal/database/database.go`** — GORM Connect function (copy from Saiduler, adjust module path)

5. **`internal/database/migrate.go`** — golang-migrate with embedded SQL files from `internal/migrations/` (copy pattern from Saiduler)

6. **`internal/migrations/`** — Create empty directory with a `.gitkeep` file. The actual migration SQL will be created in Task 2.

7. **`internal/middleware/cors.go`** — CORS middleware allowing all origins (copy from Saiduler)

8. **`internal/static/static.go`** — Gin SPA file server that serves embedded frontend assets and falls back to index.html for client-side routing (copy from Saiduler, adjust)

9. **`frontend_embed.go`** at project root:
   ```go
   package botka
   import "embed"
   //go:embed all:frontend/dist
   var FrontendDist embed.FS
   ```
   Also create `frontend/dist/.gitkeep` so the embed doesn't fail before frontend is built.

10. **`Makefile`** with targets: run, build, frontend-dev, dev-backend, test, lint, fmt, check, prod-build, deploy

11. **`.env.example`** with all config variables and their defaults

12. **`.gitignore`** — Go + Node ignores (bin/, build/, frontend/dist/, frontend/node_modules/, .env, data/)

13. **`CLAUDE.md`** — Project documentation for Botka

### Verification
- `go mod tidy` succeeds
- `go build ./cmd/server/` compiles without errors
- Running the server shows startup log and listens on :5110
- `/api/health` returns 200 with `{"status": "ok"}`