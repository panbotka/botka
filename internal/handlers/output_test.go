package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"botka/internal/models"
	"botka/internal/runner"
)

// mockBufferProvider is a test double for bufferProvider.
// It can be configured to return nil for a number of calls before returning a buffer.
type mockBufferProvider struct {
	mu          sync.Mutex
	buf         *runner.Buffer
	callCount   int
	returnAfter int // return buf after this many calls (0 = always return buf)
}

func (m *mockBufferProvider) GetBuffer(_ uuid.UUID) *runner.Buffer {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.returnAfter > 0 && m.callCount <= m.returnAfter {
		return nil
	}
	return m.buf
}

// mockTaskStatusQuerier is a test double for taskStatusQuerier.
type mockTaskStatusQuerier struct {
	status models.TaskStatus
	err    error
}

func (m *mockTaskStatusQuerier) QueryTaskStatus(_ uuid.UUID) (models.TaskStatus, error) {
	return m.status, m.err
}

func TestStreamTaskOutput_BufferAvailableImmediately(t *testing.T) {
	buf := runner.NewBuffer(1024)
	buf.Write([]byte("hello"))
	buf.Close()

	provider := &mockBufferProvider{buf: buf}
	sq := &mockTaskStatusQuerier{status: models.TaskStatusRunning}

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)
	router.GET("/tasks/:id/output", func(c *gin.Context) {
		streamTaskOutput(c, provider, sq)
	})

	taskID := uuid.New()
	c.Request = httptest.NewRequest(http.MethodGet, "/tasks/"+taskID.String()+"/output", nil)
	router.ServeHTTP(w, c.Request)

	body := w.Body.String()
	if !strings.Contains(body, "data: ") {
		t.Errorf("expected base64-encoded data event, got: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected done event, got: %s", body)
	}
}

func TestStreamTaskOutput_PollsForBuffer(t *testing.T) {
	buf := runner.NewBuffer(1024)
	buf.Write([]byte("delayed"))
	buf.Close()

	// Return nil for first 3 calls, then return the buffer.
	provider := &mockBufferProvider{buf: buf, returnAfter: 3}
	sq := &mockTaskStatusQuerier{status: models.TaskStatusRunning}

	w := httptest.NewRecorder()
	_, router := gin.CreateTestContext(w)
	router.GET("/tasks/:id/output", func(c *gin.Context) {
		streamTaskOutput(c, provider, sq)
	})

	taskID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskID.String()+"/output", nil)

	start := time.Now()
	router.ServeHTTP(w, req)
	elapsed := time.Since(start)

	// Should have polled at least 3 times (3 x 500ms = 1.5s minimum).
	provider.mu.Lock()
	calls := provider.callCount
	provider.mu.Unlock()

	if calls < 4 {
		t.Errorf("expected at least 4 GetBuffer calls (3 nil + 1 success), got %d", calls)
	}
	if elapsed < 1400*time.Millisecond {
		t.Errorf("expected polling to take at least ~1.5s, took %v", elapsed)
	}

	body := w.Body.String()
	if !strings.Contains(body, "data: ") {
		t.Errorf("expected data event after polling, got: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected done event, got: %s", body)
	}
}

func TestStreamTaskOutput_NoBuffer_TaskNotRunning_SendsDone(t *testing.T) {
	// Provider always returns nil, task is done in DB.
	provider := &mockBufferProvider{buf: nil}
	sq := &mockTaskStatusQuerier{status: models.TaskStatusDone}

	w := httptest.NewRecorder()
	_, router := gin.CreateTestContext(w)
	router.GET("/tasks/:id/output", func(c *gin.Context) {
		streamTaskOutput(c, provider, sq)
	})

	taskID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskID.String()+"/output", nil)
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected done event when task is not running, got: %s", body)
	}

	// Should have polled 10 times.
	provider.mu.Lock()
	calls := provider.callCount
	provider.mu.Unlock()

	if calls != 10 {
		t.Errorf("expected 10 GetBuffer poll attempts, got %d", calls)
	}
}

func TestStreamTaskOutput_NoBuffer_TaskRunning_SendsError(t *testing.T) {
	// Provider always returns nil, but task is still running in DB — orphaned.
	provider := &mockBufferProvider{buf: nil}
	sq := &mockTaskStatusQuerier{status: models.TaskStatusRunning}

	w := httptest.NewRecorder()
	_, router := gin.CreateTestContext(w)
	router.GET("/tasks/:id/output", func(c *gin.Context) {
		streamTaskOutput(c, provider, sq)
	})

	taskID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/tasks/"+taskID.String()+"/output", nil)
	router.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected error event for orphaned task, got: %s", body)
	}
	if !strings.Contains(body, "orphaned") {
		t.Errorf("expected orphaned message in error event, got: %s", body)
	}
}

func TestStreamTaskOutput_InvalidID(t *testing.T) {
	provider := &mockBufferProvider{}
	sq := &mockTaskStatusQuerier{}

	w := httptest.NewRecorder()
	_, router := gin.CreateTestContext(w)
	router.GET("/tasks/:id/output", func(c *gin.Context) {
		streamTaskOutput(c, provider, sq)
	})

	req := httptest.NewRequest(http.MethodGet, "/tasks/not-a-uuid/output", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
