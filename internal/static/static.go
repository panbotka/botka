// Package static serves the embedded React frontend and provides the
// health check endpoint for the Botka server.
package static

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// SetupRoutes registers the health check endpoint and configures the Gin
// router to serve embedded frontend static files with SPA fallback.
// The distFS parameter should be the frontend/dist sub-filesystem
// (already stripped of the "frontend/dist" prefix). If distFS is nil or
// does not contain index.html, static file serving is disabled and only
// the health endpoint is registered.
func SetupRoutes(router *gin.Engine, distFS fs.FS) {
	router.GET("/api/health", healthHandler)

	if distFS == nil {
		slog.Warn("no frontend filesystem provided, static file serving disabled")
		return
	}

	indexHTML, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		slog.Warn("index.html not found in frontend dist, static file serving disabled")
		return
	}

	slog.Info("embedded frontend loaded, serving static files")

	fileServer := http.FileServer(http.FS(distFS))

	router.NoRoute(noRouteHandler(distFS, fileServer, indexHTML))
}

// healthHandler responds with a JSON health status.
func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// noRouteHandler returns a Gin handler that serves static files from the
// embedded filesystem or falls back to index.html for SPA routing. API
// routes that don't match any registered endpoint receive a JSON 404.
func noRouteHandler(distFS fs.FS, fileServer http.Handler, indexHTML []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "not_found",
					"message": "endpoint not found",
				},
			})
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "index.html" {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
			return
		}
		if cleanPath != "" {
			if fi, statErr := fs.Stat(distFS, cleanPath); statErr == nil && !fi.IsDir() {
				setCacheHeaders(c, path)
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}

		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	}
}

// setCacheHeaders sets Cache-Control headers based on the request path.
// Hashed assets under /assets/ get immutable caching with a one-year max-age.
func setCacheHeaders(c *gin.Context, path string) {
	if strings.HasPrefix(path, "/assets/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	}
}
