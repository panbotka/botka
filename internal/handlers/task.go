package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

// TaskHandler handles HTTP requests for task resources.
type TaskHandler struct {
	db *gorm.DB
}

// NewTaskHandler creates a new TaskHandler with the given database connection.
func NewTaskHandler(db *gorm.DB) *TaskHandler {
	return &TaskHandler{db: db}
}

// RegisterTaskRoutes attaches task endpoints to the given router group.
func RegisterTaskRoutes(rg *gin.RouterGroup, h *TaskHandler) {
	rg.GET("/tasks", h.List)
	rg.POST("/tasks", h.Create)
	rg.POST("/tasks/batch-status", h.BatchUpdateStatus)
	rg.POST("/tasks/reorder", h.Reorder)
	rg.GET("/tasks/:id", h.Get)
	rg.PUT("/tasks/:id", h.Update)
	rg.DELETE("/tasks/:id", h.Delete)
	rg.POST("/tasks/:id/retry", h.Retry)
}

// allowedTransitions defines which status transitions are valid for task updates.
var allowedTransitions = map[models.TaskStatus]map[models.TaskStatus]bool{
	models.TaskStatusPending:     {models.TaskStatusQueued: true, models.TaskStatusCancelled: true},
	models.TaskStatusQueued:      {models.TaskStatusPending: true, models.TaskStatusCancelled: true},
	models.TaskStatusFailed:      {models.TaskStatusQueued: true},
	models.TaskStatusNeedsReview: {models.TaskStatusQueued: true, models.TaskStatusDone: true},
}

// createTaskRequest is the JSON body for creating a task.
type createTaskRequest struct {
	Title     string            `json:"title"`
	Spec      string            `json:"spec"`
	ProjectID uuid.UUID         `json:"project_id"`
	Priority  int               `json:"priority"`
	Status    models.TaskStatus `json:"status"`
}

// updateTaskRequest is the JSON body for updating a task.
type updateTaskRequest struct {
	Title    *string            `json:"title"`
	Spec     *string            `json:"spec"`
	Priority *int               `json:"priority"`
	Status   *models.TaskStatus `json:"status"`
}

// batchStatusRequest is the JSON body for batch status updates.
type batchStatusRequest struct {
	IDs    []uuid.UUID       `json:"ids"`
	Status models.TaskStatus `json:"status"`
}

// batchStatusDetail describes a single task that failed validation in a batch status update.
type batchStatusDetail struct {
	ID            uuid.UUID         `json:"id"`
	CurrentStatus models.TaskStatus `json:"current_status"`
	Reason        string            `json:"reason"`
}

// reorderItem represents a single task priority update in a reorder request.
type reorderItem struct {
	ID       uuid.UUID `json:"id"`
	Priority int       `json:"priority"`
}

// taskListItem is the JSON representation of a task in list responses.
type taskListItem struct {
	ID            uuid.UUID         `json:"id"`
	Title         string            `json:"title"`
	Spec          string            `json:"spec"`
	Status        models.TaskStatus `json:"status"`
	Priority      int               `json:"priority"`
	ProjectID     uuid.UUID         `json:"project_id"`
	ProjectName   string            `json:"project_name"`
	FailureReason *string           `json:"failure_reason"`
	RetryCount    int               `json:"retry_count"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// List returns tasks with optional filtering and pagination.
func (h *TaskHandler) List(c *gin.Context) {
	status := c.Query("status")
	projectID := c.Query("project_id")
	if projectID != "" {
		if _, err := uuid.Parse(projectID); err != nil {
			respondError(c, http.StatusBadRequest, "invalid project_id")
			return
		}
	}

	filter := taskFilter(status, projectID)
	var total int64
	if err := filter(h.db.Model(&models.Task{})).Count(&total).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to count tasks")
		return
	}

	limit, offset := parsePagination(c)
	var tasks []models.Task
	q := filter(h.db.Preload("Project")).
		Order("priority DESC, created_at ASC").
		Limit(limit).Offset(offset)
	if err := q.Find(&tasks).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list tasks")
		return
	}

	items := make([]taskListItem, 0, len(tasks))
	for i := range tasks {
		items = append(items, toTaskListItem(&tasks[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total})
}

// Create creates a new task.
func (h *TaskHandler) Create(c *gin.Context) {
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateCreateRequest(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if !h.projectIsActive(c, req.ProjectID) {
		return
	}

	task := models.Task{
		Title: req.Title, Spec: req.Spec,
		Status: req.Status, Priority: req.Priority,
		ProjectID: req.ProjectID,
	}
	if err := h.db.Create(&task).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create task")
		return
	}
	h.db.Preload("Project").First(&task, "id = ?", task.ID)
	c.JSON(http.StatusCreated, gin.H{"data": task})
}

// Get returns a single task with execution history.
func (h *TaskHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}

	var task models.Task
	q := h.db.Preload("Project").
		Preload("Executions", func(db *gorm.DB) *gorm.DB {
			return db.Order("attempt DESC")
		})
	if err := q.First(&task, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "task not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to get task")
		return
	}
	respondOK(c, task)
}

// Update modifies a task's fields and/or status.
func (h *TaskHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	var req updateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	task, ok := h.findTask(c, id)
	if !ok {
		return
	}
	if errMsg := validateUpdate(task, req); errMsg != "" {
		status := http.StatusBadRequest
		if task.Status == models.TaskStatusRunning {
			status = http.StatusConflict
		}
		respondError(c, status, errMsg)
		return
	}

	updates := buildTaskUpdates(req)
	if len(updates) == 0 {
		respondOK(c, task)
		return
	}
	if err := h.db.Model(&task).Updates(updates).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update task")
		return
	}
	h.db.Preload("Project").First(&task, "id = ?", task.ID)
	respondOK(c, task)
}

// Delete cancels or hard-deletes a task depending on its status.
func (h *TaskHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	task, ok := h.findTask(c, id)
	if !ok {
		return
	}
	if task.Status == models.TaskStatusRunning {
		respondError(c, http.StatusConflict, "cannot delete a running task")
		return
	}

	if task.Status == models.TaskStatusPending || task.Status == models.TaskStatusCancelled {
		if err := h.db.Delete(&task).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to delete task")
			return
		}
	} else {
		err := h.db.Model(&task).Update("status", models.TaskStatusCancelled).Error
		if err != nil {
			respondError(c, http.StatusInternalServerError, "failed to cancel task")
			return
		}
	}
	c.Status(http.StatusNoContent)
}

// Reorder batch-updates task priorities in a single transaction.
func (h *TaskHandler) Reorder(c *gin.Context) {
	var items []reorderItem
	if err := c.ShouldBindJSON(&items); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(items) == 0 {
		respondError(c, http.StatusBadRequest, "empty reorder list")
		return
	}

	if err := h.applyReorder(items); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// BatchUpdateStatus updates the status of multiple tasks in a single transaction.
func (h *TaskHandler) BatchUpdateStatus(c *gin.Context) {
	var req batchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if errMsg := validateBatchStatusRequest(req); errMsg != "" {
		respondError(c, http.StatusBadRequest, errMsg)
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		return h.applyBatchStatus(c, tx, req)
	})

	if errors.Is(err, errBatchAborted) {
		return // response already written
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update tasks")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"updated": len(req.IDs)}})
}

// errBatchAborted is a sentinel error used to roll back a transaction after
// the response has already been written to the client.
var errBatchAborted = errors.New("batch aborted")

// validateBatchStatusRequest checks that a batch status request is well-formed.
func validateBatchStatusRequest(req batchStatusRequest) string {
	if len(req.IDs) == 0 {
		return "ids must not be empty"
	}
	seen := make(map[uuid.UUID]bool, len(req.IDs))
	for _, id := range req.IDs {
		if seen[id] {
			return fmt.Sprintf("duplicate id: %s", id)
		}
		seen[id] = true
	}
	if !req.Status.IsValid() {
		return fmt.Sprintf("invalid status: %s", req.Status)
	}
	return ""
}

// applyBatchStatus performs the transactional batch status update, writing
// error responses directly to c and returning errBatchAborted when it does.
func (h *TaskHandler) applyBatchStatus(c *gin.Context, tx *gorm.DB, req batchStatusRequest) error {
	var tasks []models.Task
	if err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id IN ?", req.IDs).Find(&tasks).Error; err != nil {
		return err
	}

	if missing := findMissingID(req.IDs, tasks); missing != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("task not found: %s", *missing),
		})
		return errBatchAborted
	}

	if invalid := validateBatchTransitions(tasks, req.Status); len(invalid) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid status transitions",
			"details": invalid,
		})
		return errBatchAborted
	}

	return tx.Model(&models.Task{}).Where("id IN ?", req.IDs).
		Update("status", req.Status).Error
}

// findMissingID returns the first ID from ids that is not present in tasks, or nil.
func findMissingID(ids []uuid.UUID, tasks []models.Task) *uuid.UUID {
	if len(tasks) == len(ids) {
		return nil
	}
	found := make(map[uuid.UUID]bool, len(tasks))
	for i := range tasks {
		found[tasks[i].ID] = true
	}
	for _, id := range ids {
		if !found[id] {
			return &id
		}
	}
	return nil
}

// validateBatchTransitions checks that all task status transitions are valid,
// returning details for any invalid ones.
func validateBatchTransitions(tasks []models.Task, target models.TaskStatus) []batchStatusDetail {
	var invalid []batchStatusDetail
	for i := range tasks {
		if tasks[i].Status == target {
			continue
		}
		allowed, ok := allowedTransitions[tasks[i].Status]
		if !ok || !allowed[target] {
			invalid = append(invalid, batchStatusDetail{
				ID:            tasks[i].ID,
				CurrentStatus: tasks[i].Status,
				Reason: fmt.Sprintf("cannot transition from %s to %s",
					tasks[i].Status, target),
			})
		}
	}
	return invalid
}

// Retry resets a failed or needs_review task to queued status.
func (h *TaskHandler) Retry(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	task, ok := h.findTask(c, id)
	if !ok {
		return
	}
	if task.Status != models.TaskStatusFailed && task.Status != models.TaskStatusNeedsReview {
		respondError(c, http.StatusBadRequest, "can only retry failed or needs_review tasks")
		return
	}

	updates := map[string]interface{}{
		"status":         models.TaskStatusQueued,
		"failure_reason": nil,
	}
	if err := h.db.Model(&task).Updates(updates).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to retry task")
		return
	}
	h.db.Preload("Project").First(&task, "id = ?", task.ID)
	respondOK(c, task)
}

// findTask looks up a task by ID and writes the appropriate error response if not found.
func (h *TaskHandler) findTask(c *gin.Context, id uuid.UUID) (models.Task, bool) {
	var task models.Task
	if err := h.db.First(&task, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "task not found")
			return task, false
		}
		respondError(c, http.StatusInternalServerError, "failed to get task")
		return task, false
	}
	return task, true
}

// projectIsActive validates that the project exists and is active, writing errors to c.
func (h *TaskHandler) projectIsActive(c *gin.Context, projectID uuid.UUID) bool {
	var proj models.Project
	if err := h.db.First(&proj, "id = ?", projectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusBadRequest, "project not found")
			return false
		}
		respondError(c, http.StatusInternalServerError, "failed to validate project")
		return false
	}
	if !proj.Active {
		respondError(c, http.StatusBadRequest, "project is not active")
		return false
	}
	return true
}

// applyReorder updates task priorities in a single transaction.
func (h *TaskHandler) applyReorder(items []reorderItem) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			if item.ID == uuid.Nil {
				return errors.New("invalid task id")
			}
			res := tx.Model(&models.Task{}).Where("id = ?", item.ID).
				Update("priority", item.Priority)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("task not found: %s", item.ID)
			}
		}
		return nil
	})
}

// validateCreateRequest checks that a create request has all required fields.
func validateCreateRequest(req *createTaskRequest) error {
	if req.Title == "" {
		return errors.New("title is required")
	}
	if req.ProjectID == uuid.Nil {
		return errors.New("project_id is required")
	}
	if req.Status == "" {
		req.Status = models.TaskStatusQueued
	}
	if req.Status != models.TaskStatusPending && req.Status != models.TaskStatusQueued {
		return errors.New("status must be pending or queued")
	}
	return nil
}

// validateUpdate checks whether the update is allowed for the given task.
func validateUpdate(task models.Task, req updateTaskRequest) string {
	if task.Status == models.TaskStatusRunning {
		return "cannot update a running task"
	}
	if req.Status != nil && *req.Status != task.Status {
		allowed, ok := allowedTransitions[task.Status]
		if !ok || !allowed[*req.Status] {
			return "invalid status transition"
		}
	}
	return ""
}

// buildTaskUpdates converts non-nil update request fields into a GORM update map.
func buildTaskUpdates(req updateTaskRequest) map[string]interface{} {
	updates := map[string]interface{}{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Spec != nil {
		updates["spec"] = *req.Spec
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	return updates
}

// taskFilter returns a function that applies status and project_id WHERE clauses.
// Status may be a single value or comma-separated list (e.g. "done,failed,needs_review").
func taskFilter(status, projectID string) func(*gorm.DB) *gorm.DB {
	return func(q *gorm.DB) *gorm.DB {
		if status != "" {
			if statuses := strings.Split(status, ","); len(statuses) > 1 {
				q = q.Where("status IN ?", statuses)
			} else {
				q = q.Where("status = ?", status)
			}
		}
		if projectID != "" {
			q = q.Where("project_id = ?", projectID)
		}
		return q
	}
}

// parsePagination extracts limit and offset from query params with defaults.
func parsePagination(c *gin.Context) (limit, offset int) {
	limit = 50
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = o
	}
	return
}

// toTaskListItem converts a Task model into a list response item.
func toTaskListItem(t *models.Task) taskListItem {
	return taskListItem{
		ID:            t.ID,
		Title:         t.Title,
		Spec:          t.Spec,
		Status:        t.Status,
		Priority:      t.Priority,
		ProjectID:     t.ProjectID,
		ProjectName:   t.Project.Name,
		FailureReason: t.FailureReason,
		RetryCount:    t.RetryCount,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}
