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
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka"
	"botka/internal/config"
	"botka/internal/database"
	"botka/internal/handlers"
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
		// TODO: implement runMCP() — will be added in a later task
		slog.Error("MCP mode not yet implemented")
		os.Exit(1)
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
		cfg.ClaudeCredentialsPath,
		cfg.UsageThreshold5h,
		cfg.UsageThreshold7d,
		cfg.UsagePollInterval,
		"",
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

// setupRouter creates the Gin router with API and frontend routes.
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
