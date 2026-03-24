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

	"botka"
	"botka/internal/config"
	"botka/internal/database"
	"botka/internal/middleware"
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

	// TODO: project discovery — will be added in a later task
	// TODO: usage monitor initialization — will be added in a later task
	// TODO: task runner initialization — will be added in a later task

	_ = db  // used by handlers registered in later tasks
	_ = cfg // used by handlers registered in later tasks

	router := setupRouter()

	return startServer(router, cfg.Port)
}

// setupRouter creates the Gin router with health check and frontend routes.
func setupRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery(), gin.Logger(), middleware.CORS())

	// TODO: register API v1 routes — will be added in later tasks

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
