package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/runner"
)

const keepaliveInterval = 15 * time.Second

// RegisterOutputRoute registers the SSE live output route on the given router group.
func RegisterOutputRoute(rg *gin.RouterGroup, r *runner.Runner, db *gorm.DB) {
	rg.GET("/tasks/:id/output", newOutputHandler(r))
	rg.GET("/tasks/:id/output/raw", newRawOutputHandler(db))
}

// newOutputHandler returns a Gin handler that streams live task output via SSE.
func newOutputHandler(r *runner.Runner) gin.HandlerFunc {
	return func(c *gin.Context) {
		streamTaskOutput(c, r)
	}
}

// streamTaskOutput streams live task output as SSE events.
// It sends existing buffered data first, then streams new data as it arrives.
// Each data event is base64-encoded. A final "done" event is sent when the task completes.
func streamTaskOutput(c *gin.Context, r *runner.Runner) {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}

	buf := r.GetBuffer(taskID)
	if buf == nil {
		// Task is not currently running (already completed or not started).
		// Send a "done" event via SSE so the frontend stops reconnecting.
		setSSEHeaders(c)
		_, _ = fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
		c.Writer.Flush()
		return
	}

	setSSEHeaders(c)

	// Send existing buffered data as the initial event.
	if existing := buf.ReadAll(); len(existing) > 0 {
		writeSSEData(c, existing)
	}

	// Subscribe to new data.
	ch, unsubscribe := buf.Subscribe()
	defer unsubscribe()

	ctx := c.Request.Context()
	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			// SSE comment keeps the connection alive through proxies.
			_, _ = fmt.Fprintf(c.Writer, ": keepalive\n\n")
			c.Writer.Flush()
		case chunk, ok := <-ch:
			if !ok {
				// Buffer closed — task completed.
				_, _ = fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
				c.Writer.Flush()
				return
			}
			writeSSEData(c, chunk)
		}
	}
}

// newRawOutputHandler returns a Gin handler that serves stored raw output
// for a completed task execution (latest attempt).
func newRawOutputHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid task id")
			return
		}

		var exec models.TaskExecution
		err = db.Where("task_id = ? AND raw_output IS NOT NULL", taskID).
			Order("attempt DESC").
			First(&exec).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				respondError(c, http.StatusNotFound, "no output available")
				return
			}
			respondError(c, http.StatusInternalServerError, "failed to fetch output")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"execution_id": exec.ID,
				"attempt":      exec.Attempt,
				"raw_output":   *exec.RawOutput,
			},
		})
	}
}

// setSSEHeaders sets the standard headers for an SSE response.
func setSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
}

// writeSSEData writes a base64-encoded SSE data event and flushes.
func writeSSEData(c *gin.Context, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", encoded)
	c.Writer.Flush()
}
