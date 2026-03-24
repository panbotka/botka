package handlers

import (
	"github.com/gin-gonic/gin"

	"botka/internal/runner"
)

// RunnerHandler handles HTTP requests for controlling the task runner.
type RunnerHandler struct {
	runner *runner.Runner
}

// NewRunnerHandler creates a new RunnerHandler.
func NewRunnerHandler(r *runner.Runner) *RunnerHandler {
	return &RunnerHandler{runner: r}
}

// RegisterRunnerRoutes registers runner control routes on the given router group.
func RegisterRunnerRoutes(rg *gin.RouterGroup, h *RunnerHandler) {
	rg.GET("/runner/status", h.Status)
	rg.POST("/runner/start", h.Start)
	rg.POST("/runner/pause", h.Pause)
	rg.POST("/runner/stop", h.Stop)
	rg.POST("/runner/usage/refresh", h.RefreshUsage)
}

// Status returns the current runner state.
func (h *RunnerHandler) Status(c *gin.Context) {
	respondOK(c, h.runner.GetStatus())
}

// Start starts or resumes the scheduler. Accepts an optional JSON body with
// a "count" field to auto-stop after that many tasks complete.
func (h *RunnerHandler) Start(c *gin.Context) {
	var body struct {
		Count int `json:"count"`
	}
	_ = c.ShouldBindJSON(&body)

	if body.Count > 0 {
		h.runner.StartN(body.Count)
	} else {
		h.runner.Resume()
	}
	respondOK(c, h.runner.GetStatus())
}

// RefreshUsage triggers an immediate usage poll and returns the updated info.
func (h *RunnerHandler) RefreshUsage(c *gin.Context) {
	respondOK(c, h.runner.RefreshUsage())
}

// Pause pauses the scheduler. Running tasks continue to completion.
func (h *RunnerHandler) Pause(c *gin.Context) {
	h.runner.Pause()
	respondOK(c, h.runner.GetStatus())
}

// Stop immediately kills all running tasks and stops the scheduler.
func (h *RunnerHandler) Stop(c *gin.Context) {
	h.runner.HardStop()
	respondOK(c, h.runner.GetStatus())
}
