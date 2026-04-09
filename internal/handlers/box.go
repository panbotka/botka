package handlers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// BoxService describes a known service running on the box server.
type BoxService struct {
	Name        string `json:"name"`
	Port        int    `json:"port"`
	Description string `json:"description"`
	Type        string `json:"type"` // "systemd" or "manual"
	VRAMUsageMB int    `json:"vram_usage_mb,omitempty"`
}

// boxServiceStatus is the status of a single service returned to the frontend.
type boxServiceStatus struct {
	Name        string `json:"name"`
	Port        int    `json:"port"`
	Description string `json:"description"`
	Type        string `json:"type"`
	VRAMUsageMB int    `json:"vram_usage_mb,omitempty"`
	Status      string `json:"status"` // "running" or "stopped"
	URL         string `json:"url"`
}

// allowedServices is the whitelist of service names that can be started/stopped.
var allowedServices = map[string]bool{
	"image-embeddings": true,
	"photo-enhancer":   true,
}

// defaultBoxServices defines the known services on the box.
var defaultBoxServices = []BoxService{
	{Name: "image-embeddings", Port: 8000, Description: "OpenCLIP + InsightFace API (GPU)", Type: "systemd", VRAMUsageMB: 1900},
	{Name: "photo-enhancer", Port: 8001, Description: "AI photo processing (GPU)", Type: "systemd", VRAMUsageMB: 3400},
	{Name: "llama.cpp", Port: 8080, Description: "LLM inference (when running)", Type: "manual", VRAMUsageMB: 0},
}

// CommandRunner abstracts command execution for testability.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execCommandRunner is the real implementation that runs OS commands.
type execCommandRunner struct{}

func (e *execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// boxProjectsRoot is the directory on Box scanned for project subdirectories.
const boxProjectsRoot = "/home/box/projects"

// boxProjectsCacheTTL bounds how long a successful project listing is cached.
const boxProjectsCacheTTL = 2 * time.Minute

// BoxHandler handles box server management endpoints.
type BoxHandler struct {
	db         *gorm.DB
	host       string
	sshUser    string
	wolCommand string
	services   []BoxService
	runner     CommandRunner

	// projectsCache stores the last successful Box project listing so
	// repeated UI polls within boxProjectsCacheTTL skip the SSH round-trip.
	projectsMu       sync.Mutex
	projectsCachedAt time.Time
	projectsCache    []boxProjectEntry
}

// boxProjectEntry represents a single remote project returned to the frontend.
type boxProjectEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// NewBoxHandler creates a new BoxHandler with the given configuration.
// The db argument may be nil for tests that only exercise non-project endpoints;
// the projects endpoint requires a database.
func NewBoxHandler(db *gorm.DB, host, sshUser, wolCommand string) *BoxHandler {
	return &BoxHandler{
		db:         db,
		host:       host,
		sshUser:    sshUser,
		wolCommand: wolCommand,
		services:   defaultBoxServices,
		runner:     &execCommandRunner{},
	}
}

// RegisterBoxRoutes attaches box endpoints to the given router group.
func RegisterBoxRoutes(rg *gin.RouterGroup, h *BoxHandler) {
	box := rg.Group("/box")
	box.GET("/status", h.Status)
	box.GET("/projects", h.ListProjects)
	box.POST("/wake", h.Wake)
	box.POST("/shutdown", h.Shutdown)
	box.POST("/services/:name/start", h.StartService)
	box.POST("/services/:name/stop", h.StopService)
}

// Status checks if the box is online and reports the status of each service.
func (h *BoxHandler) Status(c *gin.Context) {
	online := h.isOnline()

	services := make([]boxServiceStatus, 0, len(h.services))
	for _, svc := range h.services {
		status := "stopped"
		if online && h.isPortOpen(svc.Port) {
			status = "running"
		}
		services = append(services, boxServiceStatus{
			Name:        svc.Name,
			Port:        svc.Port,
			Description: svc.Description,
			Type:        svc.Type,
			VRAMUsageMB: svc.VRAMUsageMB,
			Status:      status,
			URL:         fmt.Sprintf("http://%s:%d", h.host, svc.Port),
		})
	}

	respondOK(c, gin.H{
		"online":   online,
		"host":     h.host,
		"services": services,
	})
}

// Wake sends a Wake-on-LAN magic packet to the box.
func (h *BoxHandler) Wake(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	output, err := h.runner.Run(ctx, h.wolCommand)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("wake failed: %s: %s", err, string(output)))
		return
	}

	respondOK(c, gin.H{"message": "Wake-on-LAN packet sent"})
}

// Shutdown sends a shutdown command to the box via SSH.
func (h *BoxHandler) Shutdown(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	sshTarget := fmt.Sprintf("%s@%s", h.sshUser, h.host)
	output, err := h.runner.Run(ctx, "ssh", "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10", sshTarget, "sudo", "shutdown", "now")
	// SSH to a shutting-down host often returns an error (connection closed), so we
	// treat it as success if the command was at least sent.
	if err != nil {
		// Check if it's just a connection reset (expected during shutdown)
		if ctx.Err() == nil {
			// Command ran but SSH connection dropped — likely successful shutdown
			respondOK(c, gin.H{"message": "shutdown command sent"})
			return
		}
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("shutdown failed: %s: %s", err, string(output)))
		return
	}

	respondOK(c, gin.H{"message": "shutdown command sent"})
}

// StartService starts a systemd service on the box via SSH.
func (h *BoxHandler) StartService(c *gin.Context) {
	name := c.Param("name")
	if !allowedServices[name] {
		respondError(c, http.StatusBadRequest, fmt.Sprintf("service %q is not allowed", name))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	sshTarget := fmt.Sprintf("%s@%s", h.sshUser, h.host)
	output, err := h.runner.Run(ctx, "ssh", "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10", sshTarget, "sudo", "systemctl", "start", name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("start %s failed: %s: %s", name, err, string(output)))
		return
	}

	respondOK(c, gin.H{"message": fmt.Sprintf("service %s started", name)})
}

// StopService stops a systemd service on the box via SSH.
func (h *BoxHandler) StopService(c *gin.Context) {
	name := c.Param("name")
	if !allowedServices[name] {
		respondError(c, http.StatusBadRequest, fmt.Sprintf("service %q is not allowed", name))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	sshTarget := fmt.Sprintf("%s@%s", h.sshUser, h.host)
	output, err := h.runner.Run(ctx, "ssh", "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10", sshTarget, "sudo", "systemctl", "stop", name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("stop %s failed: %s: %s", name, err, string(output)))
		return
	}

	respondOK(c, gin.H{"message": fmt.Sprintf("service %s stopped", name)})
}

// ListProjects returns the set of project directories discovered on Box.
// When Box is offline or unreachable, it returns an empty list and sets
// `online: false` with a human-readable note. Successful results are cached
// in memory for boxProjectsCacheTTL to avoid hammering SSH on repeat polls.
//
// As a side effect, discovered directories are upserted into the projects
// table with paths prefixed by the remote marker ("box:") so they can be
// referenced as normal Project rows elsewhere in the app (e.g. by threads).
func (h *BoxHandler) ListProjects(c *gin.Context) {
	if h.db == nil {
		respondError(c, http.StatusInternalServerError, "box handler not configured with database")
		return
	}

	// Cache hit — serve stored entries without touching Box.
	if cached, ok := h.cachedProjects(); ok {
		respondOK(c, gin.H{
			"data":   cached,
			"online": true,
			"cached": true,
		})
		return
	}

	if !h.isOnline() {
		respondOK(c, gin.H{
			"data":   []boxProjectEntry{},
			"online": false,
			"note":   "Box is offline — start it to browse remote projects.",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	sshTarget := fmt.Sprintf("%s@%s", h.sshUser, h.host)
	// Use -1 for one entry per line; -p marks directories with a trailing slash;
	// BatchMode avoids interactive prompts. The remote command lists immediate
	// children of boxProjectsRoot and filters to directories (trailing slash).
	output, err := h.runner.Run(ctx,
		"ssh",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		sshTarget,
		"ls", "-1p", boxProjectsRoot,
	)
	if err != nil {
		respondError(c, http.StatusBadGateway, fmt.Sprintf("failed to list box projects: %s: %s", err, strings.TrimSpace(string(output))))
		return
	}

	names := parseBoxProjectNames(string(output))
	entries, err := h.upsertBoxProjects(names)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("failed to sync box projects: %s", err))
		return
	}

	h.storeCachedProjects(entries)

	respondOK(c, gin.H{
		"data":   entries,
		"online": true,
		"cached": false,
	})
}

// parseBoxProjectNames extracts plain directory names from `ls -1p` output.
// Directories are marked with a trailing slash; hidden entries (dot-prefixed)
// and non-directories are discarded. The result is sorted for stability.
func parseBoxProjectNames(output string) []string {
	var names []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ".") {
			continue
		}
		if !strings.HasSuffix(line, "/") {
			continue
		}
		name := strings.TrimSuffix(line, "/")
		if name == "" || strings.ContainsAny(name, "/\\") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// upsertBoxProjects creates or reactivates Project rows for each discovered
// remote directory and returns the corresponding entries. Existing box: rows
// that are no longer present on Box are left alone (not auto-deactivated),
// since a transient filesystem issue should not invalidate them.
func (h *BoxHandler) upsertBoxProjects(names []string) ([]boxProjectEntry, error) {
	entries := make([]boxProjectEntry, 0, len(names))
	for _, name := range names {
		path := "box:" + boxProjectsRoot + "/" + name

		var existing models.Project
		err := h.db.Where("path = ?", path).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			project := models.Project{
				Name:           name,
				Path:           path,
				BranchStrategy: "main",
				Active:         true,
			}
			if createErr := h.db.Create(&project).Error; createErr != nil {
				return nil, fmt.Errorf("creating box project %q: %w", name, createErr)
			}
			entries = append(entries, boxProjectEntry{
				ID:   project.ID.String(),
				Name: project.Name,
				Path: project.Path,
			})
		case err != nil:
			return nil, fmt.Errorf("querying box project %q: %w", name, err)
		default:
			// Reactivate if previously deactivated and keep the name in sync.
			updates := map[string]any{}
			if !existing.Active {
				updates["active"] = true
			}
			if existing.Name != name {
				updates["name"] = name
			}
			if len(updates) > 0 {
				if updateErr := h.db.Model(&existing).Updates(updates).Error; updateErr != nil {
					return nil, fmt.Errorf("updating box project %q: %w", name, updateErr)
				}
			}
			entries = append(entries, boxProjectEntry{
				ID:   existing.ID.String(),
				Name: name,
				Path: path,
			})
		}
	}
	return entries, nil
}

// cachedProjects returns the last cached box project list if still fresh.
func (h *BoxHandler) cachedProjects() ([]boxProjectEntry, bool) {
	h.projectsMu.Lock()
	defer h.projectsMu.Unlock()
	if h.projectsCachedAt.IsZero() {
		return nil, false
	}
	if time.Since(h.projectsCachedAt) >= boxProjectsCacheTTL {
		return nil, false
	}
	// Return a copy so callers can't mutate the cache.
	cp := make([]boxProjectEntry, len(h.projectsCache))
	copy(cp, h.projectsCache)
	return cp, true
}

// storeCachedProjects saves the given entries as the cached project list.
func (h *BoxHandler) storeCachedProjects(entries []boxProjectEntry) {
	h.projectsMu.Lock()
	defer h.projectsMu.Unlock()
	h.projectsCache = make([]boxProjectEntry, len(entries))
	copy(h.projectsCache, entries)
	h.projectsCachedAt = time.Now()
}

// isOnline checks if the box is reachable via TCP on port 22 (SSH).
func (h *BoxHandler) isOnline() bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(h.host, "22"), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// isPortOpen checks if a specific port is reachable on the box.
func (h *BoxHandler) isPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(h.host, fmt.Sprintf("%d", port)), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
