package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/runner"
)

// boxAutoOffSettingKey is the app_settings row holding the Box auto-off
// switch. The switch is a setting rather than an env var because it is
// something the user flips, not a deployment parameter.
const boxAutoOffSettingKey = "box_auto_off"

// SettingsHandler handles server-side configuration endpoints.
type SettingsHandler struct {
	db       *gorm.DB
	onChange func(key, value string)
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(db *gorm.DB) *SettingsHandler {
	return &SettingsHandler{db: db}
}

// SetOnChange registers a callback invoked whenever a setting is updated.
func (h *SettingsHandler) SetOnChange(fn func(key, value string)) {
	h.onChange = fn
}

// RegisterSettingsRoutes attaches settings endpoints to the given router group.
func RegisterSettingsRoutes(rg *gin.RouterGroup, h *SettingsHandler) {
	rg.GET("/settings", h.Get)
	rg.PUT("/settings", h.Update)
	rg.DELETE("/settings/task-outputs", h.PurgeTaskOutputs)
	rg.DELETE("/settings/cron-executions", h.PurgeCronExecutions)
}

// Get returns all settings as a key→value map. Values are stored as strings;
// max_workers is returned as an integer and box_auto_off as a boolean so the
// frontend does not have to parse them.
func (h *SettingsHandler) Get(c *gin.Context) {
	var rows []models.Setting
	if err := h.db.Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to read settings")
		return
	}

	result := gin.H{}
	for _, row := range rows {
		switch row.Key {
		case "max_workers":
			n, err := strconv.Atoi(row.Value)
			if err == nil {
				result["max_workers"] = n
			} else {
				result["max_workers"] = row.Value
			}
		case boxAutoOffSettingKey:
			result[boxAutoOffSettingKey] = row.Value == "true"
		default:
			result[row.Key] = row.Value
		}
	}

	respondOK(c, result)
}

// settingsUpdateRequest is the request body for PUT /settings.
type settingsUpdateRequest struct {
	MaxWorkers *int  `json:"max_workers"`
	BoxAutoOff *bool `json:"box_auto_off"`
}

// Update accepts a partial settings payload, validates it, persists the
// changes, and returns the current settings after the update.
func (h *SettingsHandler) Update(c *gin.Context) {
	var req settingsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MaxWorkers != nil {
		n := *req.MaxWorkers
		if n < 0 || n > 10 {
			respondError(c, http.StatusBadRequest, "max_workers must be between 0 and 10")
			return
		}

		val := strconv.Itoa(n)
		if err := h.db.Save(&models.Setting{Key: "max_workers", Value: val}).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to save setting")
			return
		}

		if h.onChange != nil {
			h.onChange("max_workers", val)
		}
	}

	if req.BoxAutoOff != nil {
		val := strconv.FormatBool(*req.BoxAutoOff)
		if err := h.db.Save(&models.Setting{Key: boxAutoOffSettingKey, Value: val}).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to save setting")
			return
		}

		if h.onChange != nil {
			h.onChange(boxAutoOffSettingKey, val)
		}
	}

	h.Get(c)
}

// PurgeTaskOutputs sets raw_output to NULL on all task_executions rows where
// raw_output IS NOT NULL and returns the number of affected rows.
func (h *SettingsHandler) PurgeTaskOutputs(c *gin.Context) {
	result := h.db.Model(&models.TaskExecution{}).
		Where("raw_output IS NOT NULL").
		Update("raw_output", nil)
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to purge task outputs")
		return
	}

	respondOK(c, gin.H{"purged": result.RowsAffected})
}

// PurgeCronExecutions deletes cron execution records older than the given
// retention period (default: 30 days). Accepts an optional "days" query param.
func (h *SettingsHandler) PurgeCronExecutions(c *gin.Context) {
	days := 30
	if d := c.Query("days"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n < 1 {
			respondError(c, http.StatusBadRequest, "days must be a positive integer")
			return
		}
		days = n
	}

	retention := time.Duration(days) * 24 * time.Hour
	purged, err := runner.PurgeCronExecutions(h.db, retention)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to purge cron executions")
		return
	}

	respondOK(c, gin.H{"purged": purged, "retention_days": days})
}
