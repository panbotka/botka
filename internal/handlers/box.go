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

// sshTransportExitCode is the status ssh(1) exits with when it fails on its own
// (authentication, connection teardown) rather than relaying the remote
// command's exit status.
const sshTransportExitCode = 255

// shutdownDisconnectMarkers are the messages ssh prints when the remote host
// tears the connection down. Once `shutdown now` takes effect, sshd goes away
// mid-session and ssh reports one of these — the expected happy path.
var shutdownDisconnectMarkers = []string{
	"closed by remote host",
	"connection closed by",
	"connection reset by peer",
	"broken pipe",
}

// sshFailureMarkers indicate ssh never reached the point of running the command,
// or that sudo refused it. ssh reports these with the same exit status as a
// shutdown-induced disconnect, so they must be matched explicitly.
var sshFailureMarkers = []string{
	"permission denied",
	"a password is required",
	"authentication failure",
	"too many authentication failures",
	"host key verification failed",
	"connection refused",
	"connection timed out",
	"operation timed out",
	"no route to host",
	"network is unreachable",
	"could not resolve hostname",
	"not in the sudoers file",
	"is not allowed to run",
}

// exitCoder is satisfied by *exec.ExitError; it lets tests inject a fake error
// that carries a specific process exit status.
type exitCoder interface {
	ExitCode() int
}

// isExpectedShutdownDisconnect reports whether a failed `ssh … sudo shutdown now`
// invocation actually means the box began powering off.
//
// A poweroff kills sshd mid-session, so ssh legitimately exits non-zero even on
// success. The signal that separates the two cases is *why* ssh gave up:
//
//   - An auth/sudo/connection failure names itself in the output ("Permission
//     denied", "sudo: a password is required", …). These happen immediately,
//     before shutdown could ever start, so they are always real failures.
//   - A non-255 exit status is the remote command's own status relayed by ssh,
//     meaning the session survived long enough to report it — the box is not
//     going down, so this is a failure too.
//   - Only a transport-level teardown (exit 255 with a disconnect message and no
//     failure marker) indicates the host went away under us, i.e. success.
//
// Anything unrecognized is treated as a failure: silently reporting success is
// exactly the bug this guards against.
func isExpectedShutdownDisconnect(err error, output string) bool {
	if err == nil {
		return true
	}

	lower := strings.ToLower(output)
	for _, marker := range sshFailureMarkers {
		if strings.Contains(lower, marker) {
			return false
		}
	}

	var ec exitCoder
	if errors.As(err, &ec) && ec.ExitCode() >= 0 && ec.ExitCode() != sshTransportExitCode {
		return false
	}

	for _, marker := range shutdownDisconnectMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	return false
}

// Shutdown sends a shutdown command to the box via SSH. A failing SSH command is
// reported as an error unless the failure is the connection teardown caused by
// the box actually powering off — see isExpectedShutdownDisconnect.
func (h *BoxHandler) Shutdown(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	sshTarget := fmt.Sprintf("%s@%s", h.sshUser, h.host)
	output, err := h.runner.Run(ctx, "ssh", "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10", sshTarget, "sudo", "shutdown", "now")
	if err != nil {
		// A timed-out command never disconnected cleanly; it is always a failure.
		if ctxErr := ctx.Err(); ctxErr != nil {
			respondError(c, http.StatusInternalServerError, shutdownErrorMessage(fmt.Errorf("%w: %w", ctxErr, err), output))
			return
		}
		if !isExpectedShutdownDisconnect(err, string(output)) {
			respondError(c, http.StatusInternalServerError, shutdownErrorMessage(err, output))
			return
		}
	}

	respondOK(c, gin.H{"message": "shutdown command sent"})
}

// shutdownErrorMessage builds a human-readable failure string that carries both
// the error and whatever SSH printed, so the UI can show why shutdown failed.
func shutdownErrorMessage(err error, output []byte) string {
	msg := fmt.Sprintf("shutdown failed: %s", err)
	if out := strings.TrimSpace(string(output)); out != "" {
		msg += ": " + out
	}
	return msg
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
