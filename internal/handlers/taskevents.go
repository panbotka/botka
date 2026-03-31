package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"botka/internal/runner"
)

// RegisterTaskEventsRoute registers the SSE endpoint for live task status changes.
func RegisterTaskEventsRoute(rg *gin.RouterGroup, hub *runner.TaskEventHub) {
	rg.GET("/tasks/events", taskEventsHandler(hub))
}

func taskEventsHandler(hub *runner.TaskEventHub) gin.HandlerFunc {
	return func(c *gin.Context) {
		setSSEHeaders(c)

		ch, unsubscribe := hub.Subscribe()
		defer unsubscribe()

		ctx := c.Request.Context()
		keepalive := time.NewTicker(keepaliveInterval)
		defer keepalive.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-keepalive.C:
				_, _ = fmt.Fprintf(c.Writer, ": keepalive\n\n")
				c.Writer.Flush()
			case event := <-ch:
				data, _ := json.Marshal(event)
				_, _ = fmt.Fprintf(c.Writer, "event: task_status\ndata: %s\n\n", data)
				c.Writer.Flush()
			}
		}
	}
}
