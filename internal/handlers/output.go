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

// Buffer polling parameters — package-level vars for testability.
var (
	bufferPollMaxAttempts  = 60
	bufferPollInterval     = 500 * time.Millisecond
	bufferPollDBCheckEvery = 10
)

// bufferProvider abstracts the method used to look up a task's output buffer.
// *runner.Runner satisfies this interface.
type bufferProvider interface {
	GetBuffer(taskID uuid.UUID) *runner.Buffer
}

// taskStatusQuerier looks up a task's current status from the database.
type taskStatusQuerier interface {
	QueryTaskStatus(taskID uuid.UUID) (models.TaskStatus, error)
}

// dbTaskStatusQuerier implements taskStatusQuerier using GORM.
type dbTaskStatusQuerier struct {
	db *gorm.DB
}

func (q *dbTaskStatusQuerier) QueryTaskStatus(taskID uuid.UUID) (models.TaskStatus, error) {
	var status models.TaskStatus
	err := q.db.Model(&models.Task{}).Select("status").Where("id = ?", taskID).Scan(&status).Error
	return status, err
}

// RegisterOutputRoute registers the SSE live output route on the given router group.
func RegisterOutputRoute(rg *gin.RouterGroup, r *runner.Runner, db *gorm.DB) {
	sq := &dbTaskStatusQuerier{db: db}
	rg.GET("/tasks/:id/output", newOutputHandler(r, sq))
	rg.GET("/tasks/:id/output/raw", newRawOutputHandler(db))
}

// newOutputHandler returns a Gin handler that streams live task output via SSE.
func newOutputHandler(r *runner.Runner, sq taskStatusQuerier) gin.HandlerFunc {
	return func(c *gin.Context) {
		streamTaskOutput(c, r, sq)
	}
}

// streamTaskOutput streams live task output as SSE events.
// It sends existing buffered data first, then streams new data as it arrives.
// Each data event is base64-encoded. A final "done" event is sent when the task completes.
func streamTaskOutput(c *gin.Context, bp bufferProvider, sq taskStatusQuerier) {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}

	// The buffer may not exist yet if the task was just claimed (status set to
	// "running" in DB) but the executor goroutine hasn't created the buffer.
	// Poll with periodic DB status checks — on a loaded RPi the gap between
	// status change and buffer creation can exceed 5 seconds.
	var buf *runner.Buffer
	for i := range bufferPollMaxAttempts {
		buf = bp.GetBuffer(taskID)
		if buf != nil {
			break
		}
		// Periodically re-check if the task is still running so we can
		// stop early when the task completes/fails during the wait.
		if i > 0 && i%bufferPollDBCheckEvery == 0 {
			status, dbErr := sq.QueryTaskStatus(taskID)
			if dbErr != nil || status != models.TaskStatusRunning {
				setSSEHeaders(c)
				_, _ = fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
				c.Writer.Flush()
				return
			}
		}
		time.Sleep(bufferPollInterval)
	}
	if buf == nil {
		setSSEHeaders(c)

		// Final status check — distinguish completed task from orphaned.
		status, dbErr := sq.QueryTaskStatus(taskID)
		if dbErr != nil || status != models.TaskStatusRunning {
			_, _ = fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
			c.Writer.Flush()
			return
		}

		// Task is marked running in DB but has no buffer — genuinely orphaned.
		_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: {\"message\":\"No output available — task may be orphaned\"}\n\n")
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
