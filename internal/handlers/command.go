package handlers

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

// RunningCommand tracks a background command process.
type RunningCommand struct {
	PID         int       `json:"pid"`
	Port        int       `json:"port,omitempty"`
	CommandType string    `json:"command_type"`
	Command     string    `json:"command"`
	ProjectID   uuid.UUID `json:"project_id"`
	StartedAt   time.Time `json:"started_at"`
}

// CommandTracker manages in-memory tracking of running background commands.
type CommandTracker struct {
	mu       sync.Mutex
	commands map[int]*RunningCommand // keyed by PID
}

// NewCommandTracker creates a new CommandTracker.
func NewCommandTracker() *CommandTracker {
	return &CommandTracker{
		commands: make(map[int]*RunningCommand),
	}
}

// CommandHandler handles HTTP requests for project command execution.
type CommandHandler struct {
	db      *gorm.DB
	tracker *CommandTracker
}

// NewCommandHandler creates a new CommandHandler.
func NewCommandHandler(db *gorm.DB, tracker *CommandTracker) *CommandHandler {
	return &CommandHandler{db: db, tracker: tracker}
}

// RegisterCommandRoutes attaches command endpoints to the given router group.
func RegisterCommandRoutes(rg *gin.RouterGroup, h *CommandHandler) {
	rg.POST("/projects/:id/run-command", h.RunCommand)
	rg.GET("/projects/:id/commands", h.ListCommands)
	rg.DELETE("/projects/:id/commands/:pid", h.KillCommand)
}

// runCommandRequest is the JSON body for running a project command.
type runCommandRequest struct {
	Command string `json:"command"`
}

// RunCommand executes a project's dev or deploy command in the background.
func (h *CommandHandler) RunCommand(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	var req runCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Command != "dev" && req.Command != "deploy" {
		respondError(c, http.StatusBadRequest, "command must be \"dev\" or \"deploy\"")
		return
	}

	var proj models.Project
	if err := h.db.First(&proj, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "project not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to get project")
		return
	}

	rc, err := h.tracker.Run(&proj, req.Command)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	respondOK(c, gin.H{
		"pid":          rc.PID,
		"command_type": rc.CommandType,
	})
}

// CommandStatus represents a running command's status.
type CommandStatus struct {
	PID         int       `json:"pid"`
	Port        int       `json:"port,omitempty"`
	CommandType string    `json:"command_type"`
	Command     string    `json:"command"`
	StartedAt   time.Time `json:"started_at"`
	Alive       bool      `json:"alive"`
}

// ListCommands returns currently tracked commands for a project, cleaning up dead ones.
func (h *CommandHandler) ListCommands(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	respondOK(c, h.tracker.List(id))
}

// KillCommand kills a running command process and its process group.
func (h *CommandHandler) KillCommand(c *gin.Context) {
	_, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	pid, err := strconv.Atoi(c.Param("pid"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid pid")
		return
	}

	if !h.tracker.Kill(pid) {
		respondError(c, http.StatusNotFound, "command not found")
		return
	}

	c.Status(http.StatusNoContent)
}

// Run starts a project command in the background and tracks it.
// commandType must be "dev" or "deploy". Returns the running command or an error.
func (ct *CommandTracker) Run(proj *models.Project, commandType string) (*RunningCommand, error) {
	var cmdStr string
	var port int
	switch commandType {
	case "dev":
		if proj.DevCommand == nil || *proj.DevCommand == "" {
			return nil, fmt.Errorf("dev command not configured for project %s", proj.Name)
		}
		cmdStr = *proj.DevCommand
		if proj.DevPort != nil {
			port = *proj.DevPort
		}
	case "deploy":
		if proj.DeployCommand == nil || *proj.DeployCommand == "" {
			return nil, fmt.Errorf("deploy command not configured for project %s", proj.Name)
		}
		cmdStr = *proj.DeployCommand
		if proj.DeployPort != nil {
			port = *proj.DeployPort
		}
	default:
		return nil, fmt.Errorf("command must be \"dev\" or \"deploy\"")
	}

	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = proj.Path
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	rc := &RunningCommand{
		PID:         cmd.Process.Pid,
		Port:        port,
		CommandType: commandType,
		Command:     cmdStr,
		ProjectID:   proj.ID,
		StartedAt:   time.Now(),
	}

	ct.mu.Lock()
	ct.commands[rc.PID] = rc
	ct.mu.Unlock()

	// Wait for the command to finish, then try to detect the real process on the port.
	go func() {
		_ = cmd.Wait()
		if port == 0 {
			return
		}
		// The bash script exited. Try to find the actual server process on the port.
		realPID := waitForPort(port, 5*time.Second, 500*time.Millisecond)
		ct.mu.Lock()
		defer ct.mu.Unlock()
		if realPID > 0 {
			// Replace the tracker entry: remove old PID key, add new one.
			delete(ct.commands, rc.PID)
			rc.PID = realPID
			ct.commands[realPID] = rc
		} else {
			// Nothing on port after bash exited — command is done/failed.
			delete(ct.commands, rc.PID)
		}
	}()

	return rc, nil
}

// List returns running commands for the given project, cleaning up dead processes.
func (ct *CommandTracker) List(projectID uuid.UUID) []CommandStatus {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	var result []CommandStatus
	var deadPIDs []int

	for pid, rc := range ct.commands {
		if rc.ProjectID != projectID {
			continue
		}
		alive := isCommandAlive(rc)
		if !alive {
			deadPIDs = append(deadPIDs, pid)
			continue
		}
		result = append(result, CommandStatus{
			PID:         rc.PID,
			Port:        rc.Port,
			CommandType: rc.CommandType,
			Command:     rc.Command,
			StartedAt:   rc.StartedAt,
			Alive:       alive,
		})
	}

	for _, pid := range deadPIDs {
		delete(ct.commands, pid)
	}

	if result == nil {
		result = []CommandStatus{}
	}
	return result
}

// Kill terminates a running command by PID. Returns true if found and killed.
func (ct *CommandTracker) Kill(pid int) bool {
	ct.mu.Lock()
	rc, exists := ct.commands[pid]
	if exists {
		delete(ct.commands, pid)
	}
	ct.mu.Unlock()

	if !exists {
		return false
	}

	// Kill the tracked PID's process group.
	_ = syscall.Kill(-rc.PID, syscall.SIGTERM)

	// If there's a port, also kill whatever is currently on that port
	// (handles PID changes after restarts).
	if rc.Port > 0 {
		if portPID := findPIDOnPort(rc.Port); portPID > 0 && portPID != rc.PID {
			_ = syscall.Kill(-portPID, syscall.SIGTERM)
			go func(p int) {
				time.Sleep(3 * time.Second)
				_ = syscall.Kill(-p, syscall.SIGKILL)
			}(portPID)
		}
	}

	go func() {
		time.Sleep(3 * time.Second)
		_ = syscall.Kill(-rc.PID, syscall.SIGKILL)
	}()

	return true
}

// isCommandAlive checks if a command is still running.
// If the command has a port, it checks port availability; otherwise falls back to PID signal-0.
func isCommandAlive(rc *RunningCommand) bool {
	if rc.Port > 0 {
		return isPortInUse(rc.Port)
	}
	return isProcessAlive(rc.PID)
}

// isProcessAlive checks if a process is still running using signal 0.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// isPortInUse checks if something is listening on the given TCP port.
func isPortInUse(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// findPIDOnPort uses lsof to find the PID of the process listening on a port.
func findPIDOnPort(port int) int {
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port)).Output()
	if err != nil {
		return 0
	}
	// lsof may return multiple PIDs (one per line). Take the first.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return 0
	}
	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return 0
	}
	return pid
}

// waitForPort polls until something is listening on the port or timeout expires.
// Returns the PID on that port, or 0 if nothing appeared.
func waitForPort(port int, timeout, interval time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isPortInUse(port) {
			return findPIDOnPort(port)
		}
		time.Sleep(interval)
	}
	return 0
}
