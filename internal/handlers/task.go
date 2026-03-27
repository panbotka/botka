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
	rg.GET("/tasks/stats", h.Stats)
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
	models.TaskStatusDeleted:     {models.TaskStatusPending: true},
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
	StartedAt     *time.Time        `json:"started_at"`
	CompletedAt   *time.Time        `json:"completed_at"`
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
	order := "priority DESC, created_at ASC"
	if isCompletedFilter(status) {
		order = "completed_at DESC NULLS LAST, created_at DESC"
	}
	q := filter(h.db.Preload("Project")).
		Order(order).
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
			return db.Select("id, task_id, attempt, started_at, finished_at, exit_code, cost_usd, duration_ms, summary, error_message, created_at").Order("attempt DESC")
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

// Delete soft-deletes a task by setting its status to deleted.
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
	if task.Status == models.TaskStatusDeleted {
		c.Status(http.StatusNoContent)
		return
	}

	if err := h.db.Model(&task).Update("status", models.TaskStatusDeleted).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete task")
		return
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
	if msg := firstError(
		validateRequired("title", req.Title),
		validateMaxLength("title", req.Title, maxTitleLength),
		validateMaxLength("spec", req.Spec, maxSpecLength),
	); msg != "" {
		return errors.New(msg)
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
	if req.Title != nil {
		if msg := validateMaxLength("title", *req.Title, maxTitleLength); msg != "" {
			return msg
		}
	}
	if req.Spec != nil {
		if msg := validateMaxLength("spec", *req.Spec, maxSpecLength); msg != "" {
			return msg
		}
	}
	if req.Status != nil && *req.Status != task.Status {
		if !req.Status.IsValid() {
			return "invalid status value"
		}
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

// isCompletedFilter returns true if the status filter contains only completed statuses
// (done, failed, needs_review, cancelled), indicating results should be sorted by completion time.
func isCompletedFilter(status string) bool {
	if status == "" {
		return false
	}
	completedStatuses := map[string]bool{
		"done": true, "failed": true, "needs_review": true, "cancelled": true,
	}
	for _, s := range strings.Split(status, ",") {
		if !completedStatuses[strings.TrimSpace(s)] {
			return false
		}
	}
	return true
}

// taskFilter returns a function that applies status and project_id WHERE clauses.
// Status may be a single value or comma-separated list (e.g. "done,failed,needs_review").
// When no status filter is provided, deleted tasks are excluded by default.
func taskFilter(status, projectID string) func(*gorm.DB) *gorm.DB {
	return func(q *gorm.DB) *gorm.DB {
		if status != "" {
			if statuses := strings.Split(status, ","); len(statuses) > 1 {
				q = q.Where("status IN ?", statuses)
			} else {
				q = q.Where("status = ?", status)
			}
		} else {
			q = q.Where("status != ?", models.TaskStatusDeleted)
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
		StartedAt:     t.StartedAt,
		CompletedAt:   t.CompletedAt,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

// globalTaskStats is the JSON response for the global task stats endpoint.
type globalTaskStats struct {
	Total          int64           `json:"total"`
	ByStatus       taskCounts      `json:"by_status"`
	CompletedToday int64           `json:"completed_today"`
	CompletedWeek  int64           `json:"completed_week"`
	SuccessRate    *float64        `json:"success_rate"`
	AvgDurationMs  *float64        `json:"avg_duration_ms"`
	TotalCostUSD   *float64        `json:"total_cost_usd"`
	TopProject     *topProjectInfo `json:"top_project"`
}

// topProjectInfo holds the most active project by task count.
type topProjectInfo struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Count int64     `json:"count"`
}

// Stats returns aggregate task statistics across all projects.
func (h *TaskHandler) Stats(c *gin.Context) {
	// Count tasks by status (excluding deleted)
	type statusCount struct {
		Status string
		Count  int64
	}
	var rows []statusCount
	if err := h.db.Model(&models.Task{}).
		Select("status, count(*) as count").
		Where("status != ?", models.TaskStatusDeleted).
		Group("status").
		Scan(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to count tasks")
		return
	}

	var counts taskCounts
	for _, row := range rows {
		switch models.TaskStatus(row.Status) {
		case models.TaskStatusPending:
			counts.Pending = row.Count
		case models.TaskStatusQueued:
			counts.Queued = row.Count
		case models.TaskStatusRunning:
			counts.Running = row.Count
		case models.TaskStatusDone:
			counts.Done = row.Count
		case models.TaskStatusFailed:
			counts.Failed = row.Count
		case models.TaskStatusNeedsReview:
			counts.NeedsReview = row.Count
		case models.TaskStatusCancelled:
			counts.Cancelled = row.Count
		}
	}

	total := counts.Pending + counts.Queued + counts.Running + counts.Done +
		counts.Failed + counts.NeedsReview + counts.Cancelled

	stats := globalTaskStats{
		Total:    total,
		ByStatus: counts,
	}

	// Tasks completed today
	today := time.Now().Truncate(24 * time.Hour)
	h.db.Model(&models.Task{}).
		Where("status = ? AND updated_at >= ?", models.TaskStatusDone, today).
		Count(&stats.CompletedToday)

	// Tasks completed this week (last 7 days)
	weekAgo := time.Now().AddDate(0, 0, -7)
	h.db.Model(&models.Task{}).
		Where("status = ? AND updated_at >= ?", models.TaskStatusDone, weekAgo).
		Count(&stats.CompletedWeek)

	// Success rate
	completed := counts.Done + counts.Failed
	if completed > 0 {
		rate := float64(counts.Done) / float64(completed)
		stats.SuccessRate = &rate
	}

	// Average duration from executions
	var avgDuration struct{ Avg *float64 }
	h.db.Model(&models.TaskExecution{}).
		Select("AVG(duration_ms) as avg").
		Where("duration_ms IS NOT NULL").
		Scan(&avgDuration)
	stats.AvgDurationMs = avgDuration.Avg

	// Total cost: task executions + chat messages (must match analytics endpoint)
	var execCost struct{ Sum *float64 }
	h.db.Model(&models.TaskExecution{}).
		Select("SUM(cost_usd) as sum").
		Where("cost_usd IS NOT NULL").
		Scan(&execCost)

	var msgCost struct{ Sum *float64 }
	h.db.Raw(`SELECT SUM(cost_usd) as sum FROM messages WHERE cost_usd IS NOT NULL`).Scan(&msgCost)

	totalCost := 0.0
	if execCost.Sum != nil {
		totalCost += *execCost.Sum
	}
	if msgCost.Sum != nil {
		totalCost += *msgCost.Sum
	}
	if totalCost > 0 {
		stats.TotalCostUSD = &totalCost
	}

	// Most active project (by non-deleted task count)
	var topProj struct {
		ProjectID uuid.UUID
		Name      string
		Count     int64
	}
	h.db.Model(&models.Task{}).
		Select("tasks.project_id, projects.name, count(*) as count").
		Joins("JOIN projects ON projects.id = tasks.project_id").
		Where("tasks.status != ?", models.TaskStatusDeleted).
		Group("tasks.project_id, projects.name").
		Order("count DESC").
		Limit(1).
		Scan(&topProj)
	if topProj.Count > 0 {
		stats.TopProject = &topProjectInfo{
			ID:    topProj.ProjectID,
			Name:  topProj.Name,
			Count: topProj.Count,
		}
	}

	respondOK(c, stats)
}
