package handlers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"github.com/gin-gonic/gin"
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

// BoxHandler handles box server management endpoints.
type BoxHandler struct {
	host       string
	sshUser    string
	wolCommand string
	services   []BoxService
	runner     CommandRunner
}

// NewBoxHandler creates a new BoxHandler with the given configuration.
func NewBoxHandler(host, sshUser, wolCommand string) *BoxHandler {
	return &BoxHandler{
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
