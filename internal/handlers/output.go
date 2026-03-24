package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"botka/internal/runner"
)

// RegisterOutputRoute registers the SSE live output route on the given router group.
func RegisterOutputRoute(rg *gin.RouterGroup, r *runner.Runner) {
	rg.GET("/tasks/:id/output", newOutputHandler(r))
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
		respondError(c, http.StatusNotFound, "task is not running")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// Send existing buffered data as the initial event.
	if existing := buf.ReadAll(); len(existing) > 0 {
		writeSSEData(c, existing)
	}

	// Subscribe to new data.
	ch, unsubscribe := buf.Subscribe()
	defer unsubscribe()

	ctx := c.Request.Context()

	for {
		select {
		case <-ctx.Done():
			return
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

// writeSSEData writes a base64-encoded SSE data event and flushes.
func writeSSEData(c *gin.Context, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", encoded)
	c.Writer.Flush()
}
