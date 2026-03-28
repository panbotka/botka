package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
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

	var cmdStr string
	switch req.Command {
	case "dev":
		if proj.DevCommand == nil || *proj.DevCommand == "" {
			respondError(c, http.StatusBadRequest, "dev command not configured for this project")
			return
		}
		cmdStr = *proj.DevCommand
	case "deploy":
		if proj.DeployCommand == nil || *proj.DeployCommand == "" {
			respondError(c, http.StatusBadRequest, "deploy command not configured for this project")
			return
		}
		cmdStr = *proj.DeployCommand
	}

	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = proj.Path
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("failed to start command: %v", err))
		return
	}

	rc := &RunningCommand{
		PID:         cmd.Process.Pid,
		CommandType: req.Command,
		Command:     cmdStr,
		ProjectID:   proj.ID,
		StartedAt:   time.Now(),
	}

	h.tracker.mu.Lock()
	h.tracker.commands[rc.PID] = rc
	h.tracker.mu.Unlock()

	// Reap the process in background to avoid zombies.
	go func() { _ = cmd.Wait() }()

	respondOK(c, gin.H{
		"pid":          rc.PID,
		"command_type": rc.CommandType,
	})
}

// commandStatus represents a running command's status.
type commandStatus struct {
	PID         int       `json:"pid"`
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

	h.tracker.mu.Lock()
	defer h.tracker.mu.Unlock()

	var result []commandStatus
	var deadPIDs []int

	for pid, rc := range h.tracker.commands {
		if rc.ProjectID != id {
			continue
		}
		alive := isProcessAlive(pid)
		if !alive {
			deadPIDs = append(deadPIDs, pid)
			continue
		}
		result = append(result, commandStatus{
			PID:         rc.PID,
			CommandType: rc.CommandType,
			Command:     rc.Command,
			StartedAt:   rc.StartedAt,
			Alive:       alive,
		})
	}

	// Clean up dead processes.
	for _, pid := range deadPIDs {
		delete(h.tracker.commands, pid)
	}

	if result == nil {
		result = []commandStatus{}
	}

	respondOK(c, result)
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

	h.tracker.mu.Lock()
	rc, exists := h.tracker.commands[pid]
	if exists {
		delete(h.tracker.commands, pid)
	}
	h.tracker.mu.Unlock()

	if !exists {
		respondError(c, http.StatusNotFound, "command not found")
		return
	}

	// Kill the process group (negative PID kills all children too).
	_ = syscall.Kill(-rc.PID, syscall.SIGTERM)

	// Give it a moment then force kill if still alive.
	go func() {
		time.Sleep(3 * time.Second)
		_ = syscall.Kill(-rc.PID, syscall.SIGKILL)
	}()

	c.Status(http.StatusNoContent)
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
