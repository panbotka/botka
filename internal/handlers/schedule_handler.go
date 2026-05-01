package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/runner"
)

// ScheduleHandler handles HTTP requests for the recurring task schedule
// resource.
type ScheduleHandler struct {
	db        *gorm.DB
	scheduler *runner.ScheduleScheduler
}

// NewScheduleHandler creates a new ScheduleHandler.
func NewScheduleHandler(db *gorm.DB, scheduler *runner.ScheduleScheduler) *ScheduleHandler {
	return &ScheduleHandler{db: db, scheduler: scheduler}
}

// RegisterScheduleRoutes attaches schedule endpoints to the given router group.
func RegisterScheduleRoutes(rg *gin.RouterGroup, h *ScheduleHandler) {
	rg.GET("/schedules", h.List)
	rg.POST("/schedules", h.Create)
	rg.GET("/schedules/:id", h.Get)
	rg.PUT("/schedules/:id", h.Update)
	rg.DELETE("/schedules/:id", h.Delete)
	rg.POST("/schedules/:id/run-now", h.RunNow)
}

// createScheduleRequest is the JSON body for creating a task schedule.
type createScheduleRequest struct {
	ProjectID      uuid.UUID `json:"project_id"`
	Title          string    `json:"title"`
	Spec           string    `json:"spec"`
	CronExpression string    `json:"cron_expression"`
	Priority       int       `json:"priority"`
	Enabled        *bool     `json:"enabled"`
}

// updateScheduleRequest is the JSON body for updating a task schedule.
type updateScheduleRequest struct {
	Title          *string `json:"title"`
	Spec           *string `json:"spec"`
	CronExpression *string `json:"cron_expression"`
	Priority       *int    `json:"priority"`
	Enabled        *bool   `json:"enabled"`
}

// List returns all schedules ordered by title.
func (h *ScheduleHandler) List(c *gin.Context) {
	var schedules []models.TaskSchedule
	q := h.db.Preload("Project")
	if pid := c.Query("project_id"); pid != "" {
		if _, err := uuid.Parse(pid); err != nil {
			respondError(c, http.StatusBadRequest, "invalid project_id")
			return
		}
		q = q.Where("project_id = ?", pid)
	}

	var total int64
	if err := q.Model(&models.TaskSchedule{}).Count(&total).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to count schedules")
		return
	}
	if err := q.Order("title ASC").Find(&schedules).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list schedules")
		return
	}
	respondList(c, schedules, total)
}

// Get returns a single schedule.
func (h *ScheduleHandler) Get(c *gin.Context) {
	id, ok := parseScheduleID(c)
	if !ok {
		return
	}
	var sched models.TaskSchedule
	if err := h.db.Preload("Project").First(&sched, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "schedule not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to load schedule")
		return
	}
	respondOK(c, sched)
}

// Create creates a new task schedule.
func (h *ScheduleHandler) Create(c *gin.Context) {
	var req createScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if errMsg := validateCreateSchedule(&req); errMsg != "" {
		respondError(c, http.StatusBadRequest, errMsg)
		return
	}

	var proj models.Project
	if err := h.db.First(&proj, "id = ?", req.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusBadRequest, "project not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to validate project")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	next, err := runner.ComputeNextRun(req.CronExpression, time.Now())
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid cron expression")
		return
	}

	sched := models.TaskSchedule{
		ProjectID:      req.ProjectID,
		Title:          req.Title,
		Spec:           req.Spec,
		CronExpression: req.CronExpression,
		Priority:       req.Priority,
		Enabled:        enabled,
		NextRunAt:      &next,
	}
	if err := h.db.Create(&sched).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create schedule")
		return
	}
	// GORM skips false bools with default:true on Create.
	if !enabled {
		h.db.Model(&sched).Update("enabled", false)
	}
	h.db.Preload("Project").First(&sched, sched.ID)
	c.JSON(http.StatusCreated, gin.H{"data": sched})
}

// Update partially updates a schedule. Changing the cron expression
// recomputes next_run_at from now.
func (h *ScheduleHandler) Update(c *gin.Context) {
	id, ok := parseScheduleID(c)
	if !ok {
		return
	}
	var req updateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	var sched models.TaskSchedule
	if err := h.db.First(&sched, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "schedule not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to load schedule")
		return
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		if *req.Title == "" {
			respondError(c, http.StatusBadRequest, "title is required")
			return
		}
		updates["title"] = *req.Title
	}
	if req.Spec != nil {
		updates["spec"] = *req.Spec
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.CronExpression != nil {
		if _, err := cron.ParseStandard(*req.CronExpression); err != nil {
			respondError(c, http.StatusBadRequest, "invalid cron expression")
			return
		}
		next, err := runner.ComputeNextRun(*req.CronExpression, time.Now())
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid cron expression")
			return
		}
		updates["cron_expression"] = *req.CronExpression
		updates["next_run_at"] = next
	}
	if len(updates) == 0 {
		h.db.Preload("Project").First(&sched, sched.ID)
		respondOK(c, sched)
		return
	}

	if err := h.db.Model(&sched).Updates(updates).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update schedule")
		return
	}
	h.db.Preload("Project").First(&sched, sched.ID)
	respondOK(c, sched)
}

// Delete removes a schedule. Tasks created by the schedule keep their
// schedule_id NULLed via ON DELETE SET NULL.
func (h *ScheduleHandler) Delete(c *gin.Context) {
	id, ok := parseScheduleID(c)
	if !ok {
		return
	}
	res := h.db.Delete(&models.TaskSchedule{}, id)
	if res.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete schedule")
		return
	}
	if res.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "schedule not found")
		return
	}
	c.Status(http.StatusNoContent)
}

// RunNow immediately creates a pending task from the schedule, regardless of
// next_run_at or enabled state. Returns the new task's id.
func (h *ScheduleHandler) RunNow(c *gin.Context) {
	id, ok := parseScheduleID(c)
	if !ok {
		return
	}
	if h.scheduler == nil {
		respondError(c, http.StatusInternalServerError, "schedule scheduler not available")
		return
	}
	taskID, err := h.scheduler.RunNow(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "schedule not found")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"task_id": taskID}})
}

// parseScheduleID extracts the :id path parameter as an int64 and writes a
// 400 response on failure.
func parseScheduleID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		respondError(c, http.StatusBadRequest, "invalid schedule id")
		return 0, false
	}
	return id, true
}

// validateCreateSchedule validates a create request and returns an empty
// string on success.
func validateCreateSchedule(req *createScheduleRequest) string {
	if msg := firstError(
		validateRequired("title", req.Title),
		validateMaxLength("title", req.Title, maxTitleLength),
		validateMaxLength("spec", req.Spec, maxSpecLength),
		validateRequired("cron_expression", req.CronExpression),
	); msg != "" {
		return msg
	}
	if req.ProjectID == uuid.Nil {
		return "project_id is required"
	}
	if _, err := cron.ParseStandard(req.CronExpression); err != nil {
		return "invalid cron expression"
	}
	return ""
}
