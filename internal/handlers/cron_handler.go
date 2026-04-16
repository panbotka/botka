package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/runner"
)

// CronHandler handles HTTP requests for cron job resources.
type CronHandler struct {
	db        *gorm.DB
	scheduler *runner.CronScheduler
}

// NewCronHandler creates a new CronHandler.
func NewCronHandler(db *gorm.DB, scheduler *runner.CronScheduler) *CronHandler {
	return &CronHandler{db: db, scheduler: scheduler}
}

// RegisterCronRoutes attaches cron job endpoints to the given router group.
func RegisterCronRoutes(rg *gin.RouterGroup, h *CronHandler) {
	rg.GET("/cron-jobs", h.List)
	rg.POST("/cron-jobs", h.Create)
	rg.GET("/cron-jobs/:id", h.Get)
	rg.PATCH("/cron-jobs/:id", h.Update)
	rg.DELETE("/cron-jobs/:id", h.Delete)
	rg.GET("/cron-jobs/:id/executions", h.ListExecutions)
	rg.POST("/cron-jobs/:id/run", h.Run)
}

// createCronJobRequest is the JSON body for creating a cron job.
type createCronJobRequest struct {
	Name           string    `json:"name"`
	Schedule       string    `json:"schedule"`
	Prompt         string    `json:"prompt"`
	ProjectID      uuid.UUID `json:"project_id"`
	Enabled        *bool     `json:"enabled"`
	TimeoutMinutes *int      `json:"timeout_minutes"`
	Model          *string   `json:"model"`
}

// updateCronJobRequest is the JSON body for partially updating a cron job.
type updateCronJobRequest struct {
	Name           *string `json:"name"`
	Schedule       *string `json:"schedule"`
	Prompt         *string `json:"prompt"`
	Enabled        *bool   `json:"enabled"`
	TimeoutMinutes *int    `json:"timeout_minutes"`
	Model          *string `json:"model"`
}

// List returns all cron jobs ordered by name with project info.
func (h *CronHandler) List(c *gin.Context) {
	var jobs []models.CronJob
	var total int64

	if err := h.db.Model(&models.CronJob{}).Count(&total).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to count cron jobs")
		return
	}

	if err := h.db.Preload("Project").Order("name ASC").Find(&jobs).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list cron jobs")
		return
	}

	respondList(c, jobs, total)
}

// Get returns a single cron job with project info.
func (h *CronHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid cron job id")
		return
	}

	var job models.CronJob
	if err := h.db.Preload("Project").First(&job, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "cron job not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to get cron job")
		return
	}

	respondOK(c, job)
}

// Create creates a new cron job.
func (h *CronHandler) Create(c *gin.Context) {
	var req createCronJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if errMsg := validateCreateCronJob(&req); errMsg != "" {
		respondError(c, http.StatusBadRequest, errMsg)
		return
	}

	// Validate project exists.
	var proj models.Project
	if err := h.db.First(&proj, "id = ?", req.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusBadRequest, "project not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to validate project")
		return
	}

	job := models.CronJob{
		Name:      req.Name,
		Schedule:  req.Schedule,
		Prompt:    req.Prompt,
		ProjectID: req.ProjectID,
		Enabled:   true,
	}
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}
	if req.TimeoutMinutes != nil {
		job.TimeoutMinutes = *req.TimeoutMinutes
	}
	if req.Model != nil && *req.Model != "" {
		job.Model = req.Model
	}

	if err := h.db.Create(&job).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create cron job")
		return
	}
	// GORM skips zero-value bools with default:true during Create, so if the
	// caller explicitly set enabled=false, apply it with a separate update.
	if req.Enabled != nil && !*req.Enabled {
		h.db.Model(&job).Update("enabled", false)
	}

	h.db.Preload("Project").First(&job, job.ID)
	c.JSON(http.StatusCreated, gin.H{"data": job})
}

// Update partially updates a cron job.
func (h *CronHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid cron job id")
		return
	}

	var job models.CronJob
	if err := h.db.First(&job, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "cron job not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to get cron job")
		return
	}

	var req updateCronJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Schedule != nil {
		if _, err := cron.ParseStandard(*req.Schedule); err != nil {
			respondError(c, http.StatusBadRequest, "invalid cron schedule expression")
			return
		}
	}

	updates := buildCronJobUpdates(req)
	if len(updates) == 0 {
		h.db.Preload("Project").First(&job, job.ID)
		respondOK(c, job)
		return
	}

	if err := h.db.Model(&job).Updates(updates).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update cron job")
		return
	}

	h.db.Preload("Project").First(&job, job.ID)
	respondOK(c, job)
}

// Delete removes a cron job and all its executions (via CASCADE).
func (h *CronHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid cron job id")
		return
	}

	result := h.db.Delete(&models.CronJob{}, id)
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete cron job")
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "cron job not found")
		return
	}

	c.Status(http.StatusNoContent)
}

// ListExecutions returns executions for a cron job with pagination.
func (h *CronHandler) ListExecutions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid cron job id")
		return
	}

	// Verify the cron job exists.
	var job models.CronJob
	if err := h.db.First(&job, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "cron job not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to get cron job")
		return
	}

	limit := 20
	offset := 0
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = o
	}

	var total int64
	h.db.Model(&models.CronExecution{}).Where("cron_job_id = ?", id).Count(&total)

	var executions []models.CronExecution
	if err := h.db.Where("cron_job_id = ?", id).
		Order("started_at DESC").
		Limit(limit).Offset(offset).
		Find(&executions).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list executions")
		return
	}

	respondList(c, executions, total)
}

// Run manually triggers a cron job execution immediately.
func (h *CronHandler) Run(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid cron job id")
		return
	}

	if h.scheduler == nil {
		respondError(c, http.StatusInternalServerError, "cron scheduler not available")
		return
	}

	executionID, err := h.scheduler.TriggerJob(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "cron job not found")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"execution_id": executionID}})
}

// validateCreateCronJob validates the create request fields.
func validateCreateCronJob(req *createCronJobRequest) string {
	if msg := firstError(
		validateRequired("name", req.Name),
		validateRequired("schedule", req.Schedule),
		validateRequired("prompt", req.Prompt),
	); msg != "" {
		return msg
	}

	if req.ProjectID == uuid.Nil {
		return "project_id is required"
	}

	if _, err := cron.ParseStandard(req.Schedule); err != nil {
		return "invalid cron schedule expression"
	}

	if req.TimeoutMinutes != nil && *req.TimeoutMinutes < 1 {
		return "timeout_minutes must be at least 1"
	}

	return ""
}

// buildCronJobUpdates converts non-nil update request fields into a GORM update map.
func buildCronJobUpdates(req updateCronJobRequest) map[string]interface{} {
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Schedule != nil {
		updates["schedule"] = *req.Schedule
	}
	if req.Prompt != nil {
		updates["prompt"] = *req.Prompt
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.TimeoutMinutes != nil {
		updates["timeout_minutes"] = *req.TimeoutMinutes
	}
	if req.Model != nil {
		updates["model"] = *req.Model
	}
	return updates
}
