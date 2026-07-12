package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

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
	rg.POST("/runner/clear-rate-limit", h.ClearRateLimit)
	rg.POST("/tasks/:id/kill", h.KillTask)
	rg.POST("/tasks/:id/force-run", h.ForceRun)
	rg.POST("/tasks/:id/regenerate-summary", h.RegenerateFailureSummary)
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

// ClearRateLimit clears the rate-limit gate immediately. Returns 204 with no
// body — the caller refreshes the runner status to see the cleared fields.
func (h *RunnerHandler) ClearRateLimit(c *gin.Context) {
	gate := h.runner.RateLimitGate()
	if gate == nil {
		respondError(c, http.StatusServiceUnavailable, "rate limit gate is not initialized")
		return
	}
	gate.Clear()
	c.Status(http.StatusNoContent)
}

// KillTask terminates a single running task and reverts its git changes.
func (h *RunnerHandler) KillTask(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	if err := h.runner.KillTask(id); err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "Task kill initiated"}})
}

// ForceRun launches a specific queued task immediately, bypassing the rate-limit
// gates. Backs the "Spustit teď" button for tasks stuck behind an exhausted 5h
// limit.
func (h *RunnerHandler) ForceRun(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	task, err := h.runner.ForceRunTask(id)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			respondError(c, http.StatusNotFound, "task not found")
		case errors.Is(err, runner.ErrTaskNotQueued):
			respondError(c, http.StatusConflict, "task is not queued")
		case errors.Is(err, runner.ErrRunnerStopped):
			respondError(c, http.StatusConflict, "runner is stopped; start it first")
		case errors.Is(err, runner.ErrWorkersBusy):
			respondError(c, http.StatusConflict, "all workers are busy")
		case errors.Is(err, runner.ErrProjectBusy):
			respondError(c, http.StatusConflict, "another task on this project is already running")
		case errors.Is(err, runner.ErrLaunchRace):
			respondError(c, http.StatusConflict, "could not launch the task right now; try again")
		default:
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	respondOK(c, task)
}

// regenerateFailureSummaryTimeout caps how long a synchronous regenerate
// request may run. Slightly longer than the underlying claude timeout so the
// HTTP client gets a clean error rather than a context deadline race.
const regenerateFailureSummaryTimeout = 2 * time.Minute

// RegenerateFailureSummary re-runs the haiku summarization for a failed task
// and returns the produced summary. Synchronous so the UI can show the
// updated text without polling.
func (h *RunnerHandler) RegenerateFailureSummary(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), regenerateFailureSummaryTimeout)
	defer cancel()

	summary, err := h.runner.RegenerateFailureSummary(ctx, id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"failure_summary": summary}})
}
