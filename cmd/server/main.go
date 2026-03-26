// Package main is the entry point for the Botka server.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka"
	"botka/internal/claude"
	"botka/internal/config"
	"botka/internal/database"
	"botka/internal/handlers"
	"botka/internal/mcp"
	"botka/internal/middleware"
	"botka/internal/projects"
	"botka/internal/runner"
	"botka/internal/static"
)

const shutdownTimeout = 10 * time.Second

func main() {
	// MCP mode: stdout is reserved for the protocol, log to stderr.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		if err := runMCP(); err != nil {
			slog.Error("mcp server failed", "error", err)
			os.Exit(1)
		}
		return
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	slog.Info("botka starting")

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}

	slog.Info("botka stopped")
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get underlying sql.DB: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()
	defer claude.Sessions.Shutdown()

	// Project discovery: scan filesystem and sync to database.
	discovered, err := projects.Scan(cfg.ProjectsDir)
	if err != nil {
		return fmt.Errorf("scan projects: %w", err)
	}
	if err := projects.SyncToDatabase(db, discovered); err != nil {
		return fmt.Errorf("sync projects: %w", err)
	}
	slog.Info("project discovery complete", "count", len(discovered))

	// Usage monitor: tracks Anthropic API rate limits.
	usageMon := runner.NewUsageMonitor(
		cfg.ClaudeUsageCmd,
		cfg.UsageThreshold5h,
		cfg.UsageThreshold7d,
		cfg.UsagePollInterval,
	)
	usageMon.Start(context.Background())
	defer usageMon.Stop()

	// Task runner: scheduler loop and parallel task execution.
	taskRunner, err := runner.NewRunner(db, cfg, usageMon)
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}
	// Restore runner state on startup (starts loop if previously running).
	taskRunner.RestoreState()
	defer taskRunner.Shutdown()

	router := setupRouter(db, cfg, taskRunner)

	return startServer(router, cfg.Port)
}

// runMCP starts the MCP server in stdio mode. It connects to the database,
// runs migrations, and reads JSON-RPC messages from stdin until EOF.
func runMCP() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get underlying sql.DB: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	return mcp.RunStdio(db)
}

// setupRouter creates the Gin router with API, MCP, and frontend routes.
func setupRouter(db *gorm.DB, cfg *config.Config, taskRunner *runner.Runner) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery(), gin.Logger(), middleware.CORS())

	v1 := router.Group("/api/v1")

	projectHandler := handlers.NewProjectHandler(db, cfg.ProjectsDir, projects.Scan, projects.SyncToDatabase)
	handlers.RegisterProjectRoutes(v1, projectHandler)

	taskHandler := handlers.NewTaskHandler(db)
	handlers.RegisterTaskRoutes(v1, taskHandler)

	runnerHandler := handlers.NewRunnerHandler(taskRunner)
	handlers.RegisterRunnerRoutes(v1, runnerHandler)

	handlers.RegisterOutputRoute(v1, taskRunner)

	// Thread, chat, and file handlers.
	threadHandler := handlers.NewThreadHandler(db, cfg.AIModel, cfg.AvailableModels)
	handlers.RegisterThreadRoutes(v1, threadHandler)

	claudeCfg := claude.RunConfig{ClaudePath: cfg.ClaudePath}
	contextCfg := claude.ContextConfig{
		OpenClawWorkspace: cfg.OpenClawWorkspace,
		ContextDir:        cfg.ClaudeContextDir,
	}
	chatHandler := handlers.NewChatHandler(db, cfg.AIModel, cfg.UploadDir, claudeCfg, contextCfg, cfg.ClaudeDefaultWorkDir)
	handlers.RegisterChatRoutes(v1, chatHandler)

	fileHandler := handlers.NewFileHandler(db, cfg.UploadDir)
	handlers.RegisterFileRoutes(v1, fileHandler)

	// Supporting chat handlers: tags, personas, memories, search, transcribe, processes, status.
	tagHandler := handlers.NewTagHandler(db)
	handlers.RegisterTagRoutes(v1, tagHandler)

	personaHandler := handlers.NewPersonaHandler(db)
	handlers.RegisterPersonaRoutes(v1, personaHandler)

	memoryHandler := handlers.NewMemoryHandler(db)
	handlers.RegisterMemoryRoutes(v1, memoryHandler)

	searchHandler := handlers.NewSearchHandler(db)
	handlers.RegisterSearchRoutes(v1, searchHandler)

	analyticsHandler := handlers.NewAnalyticsHandler(db)
	handlers.RegisterAnalyticsRoutes(v1, analyticsHandler)

	transcribeHandler := handlers.NewTranscribeHandler(cfg.OpenClawURL, cfg.OpenClawToken, cfg.WhisperEnabled)
	handlers.RegisterTranscribeRoutes(v1, transcribeHandler)

	processHandler := handlers.NewProcessHandler()
	handlers.RegisterProcessRoutes(v1, processHandler)

	statusHandler := handlers.NewStatusHandler(cfg.AIModel, cfg.AvailableModels, cfg.WhisperEnabled)
	handlers.RegisterStatusRoutes(v1, statusHandler)

	settingsHandler := handlers.NewSettingsHandler(db)
	settingsHandler.SetOnChange(func(key, value string) {
		if key == "max_workers" {
			n, err := strconv.Atoi(value)
			if err == nil {
				taskRunner.SetMaxWorkers(n)
			}
		}
	})
	handlers.RegisterSettingsRoutes(v1, settingsHandler)

	// MCP SSE transport.
	mcpServer := mcp.NewServer(db, taskRunner)
	mcpSSE := mcp.NewSSEHandler(mcpServer)
	mcp.RegisterRoutes(router.Group("/mcp"), mcpSSE)

	frontendFS := initFrontendFS()
	static.SetupRoutes(router, frontendFS)

	return router
}

// startServer starts the HTTP server and blocks until a termination
// signal is received, then performs a graceful shutdown.
func startServer(handler http.Handler, port string) error {
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("server listening", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	return nil
}

// initFrontendFS extracts the frontend/dist sub-filesystem from the
// embedded assets. Returns nil if the embedded directory is unavailable.
func initFrontendFS() fs.FS {
	subFS, err := fs.Sub(botka.FrontendDist, "frontend/dist")
	if err != nil {
		slog.Warn("frontend dist not available", "error", err)
		return nil
	}
	return subFS
}
