package handlers

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

// hexColorRegex enforces a 6-digit hex color string (e.g. #FF5733).
var hexColorRegex = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

const defaultTaskTagColor = "#6B7280"

// TaskTagHandler handles HTTP requests for task tag resources.
type TaskTagHandler struct {
	db *gorm.DB
}

// NewTaskTagHandler creates a new TaskTagHandler with the given database connection.
func NewTaskTagHandler(db *gorm.DB) *TaskTagHandler {
	return &TaskTagHandler{db: db}
}

// RegisterTaskTagRoutes attaches task tag endpoints to the given router group.
func RegisterTaskTagRoutes(rg *gin.RouterGroup, h *TaskTagHandler) {
	rg.GET("/task-tags", h.List)
	rg.POST("/task-tags", h.Create)
	rg.PATCH("/task-tags/:id", h.Update)
	rg.DELETE("/task-tags/:id", h.Delete)
	rg.POST("/tasks/:id/tags", h.Assign)
}

// taskTagRequest is the JSON body for creating or updating a task tag.
type taskTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

// assignTagsRequest is the JSON body for replacing the tags on a task.
type assignTagsRequest struct {
	TagIDs []int64 `json:"tag_ids"`
}

// List returns all task tags ordered by name.
func (h *TaskTagHandler) List(c *gin.Context) {
	var tags []models.TaskTag
	if err := h.db.Order("name ASC").Find(&tags).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list task tags")
		return
	}
	if tags == nil {
		tags = []models.TaskTag{}
	}
	respondOK(c, tags)
}

// Create creates a new task tag.
func (h *TaskTagHandler) Create(c *gin.Context) {
	var req taskTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	name := ""
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	if name == "" || len(name) > 50 {
		respondError(c, http.StatusBadRequest, "name is required (max 50 chars)")
		return
	}

	color := defaultTaskTagColor
	if req.Color != nil && strings.TrimSpace(*req.Color) != "" {
		color = strings.TrimSpace(*req.Color)
		if !hexColorRegex.MatchString(color) {
			respondError(c, http.StatusBadRequest, "color must be a 6-digit hex string like #FF5733")
			return
		}
	}

	tag := models.TaskTag{Name: name, Color: color}
	if err := h.db.Create(&tag).Error; err != nil {
		if isUniqueViolation(err) {
			respondError(c, http.StatusConflict, "tag name already exists")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to create tag")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": tag})
}

// Update partially updates a task tag's name and/or color.
func (h *TaskTagHandler) Update(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid tag id")
		return
	}

	var req taskTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 50 {
			respondError(c, http.StatusBadRequest, "name must be non-empty (max 50 chars)")
			return
		}
		updates["name"] = name
	}
	if req.Color != nil {
		color := strings.TrimSpace(*req.Color)
		if color != "" && !hexColorRegex.MatchString(color) {
			respondError(c, http.StatusBadRequest, "color must be a 6-digit hex string like #FF5733")
			return
		}
		if color == "" {
			color = defaultTaskTagColor
		}
		updates["color"] = color
	}
	if len(updates) == 0 {
		respondError(c, http.StatusBadRequest, "no fields to update")
		return
	}

	result := h.db.Model(&models.TaskTag{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		if isUniqueViolation(result.Error) {
			respondError(c, http.StatusConflict, "tag name already exists")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to update tag")
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "tag not found")
		return
	}

	var updated models.TaskTag
	if err := h.db.First(&updated, id).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to load updated tag")
		return
	}
	respondOK(c, updated)
}

// Delete removes a task tag and cascade-removes its task assignments.
func (h *TaskTagHandler) Delete(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid tag id")
		return
	}

	result := h.db.Delete(&models.TaskTag{}, id)
	if result.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete tag")
		return
	}
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "tag not found")
		return
	}

	c.Status(http.StatusNoContent)
}

// Assign replaces the set of tags on a task with the given list of tag IDs.
func (h *TaskTagHandler) Assign(c *gin.Context) {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}

	var req assignTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	// Deduplicate while preserving insertion order isn't important; just dedupe.
	uniqueIDs := make(map[int64]struct{}, len(req.TagIDs))
	for _, id := range req.TagIDs {
		uniqueIDs[id] = struct{}{}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		var task models.Task
		if err := tx.First(&task, "id = ?", taskID).Error; err != nil {
			return err
		}

		if len(uniqueIDs) > 0 {
			ids := make([]int64, 0, len(uniqueIDs))
			for id := range uniqueIDs {
				ids = append(ids, id)
			}
			var count int64
			if err := tx.Model(&models.TaskTag{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return errTagNotFound
			}
		}

		if err := tx.Where("task_id = ?", taskID).Delete(&models.TaskTagAssignment{}).Error; err != nil {
			return err
		}
		if len(uniqueIDs) == 0 {
			return nil
		}
		assignments := make([]models.TaskTagAssignment, 0, len(uniqueIDs))
		for id := range uniqueIDs {
			assignments = append(assignments, models.TaskTagAssignment{TaskID: taskID, TagID: id})
		}
		return tx.Create(&assignments).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			respondError(c, http.StatusNotFound, "task not found")
		case errors.Is(err, errTagNotFound):
			respondError(c, http.StatusBadRequest, "one or more tag IDs do not exist")
		default:
			respondError(c, http.StatusInternalServerError, "failed to assign tags")
		}
		return
	}

	var tags []models.TaskTag
	if err := h.db.
		Joins("JOIN task_tag_assignments tta ON tta.tag_id = task_tags.id").
		Where("tta.task_id = ?", taskID).
		Order("task_tags.name ASC").
		Find(&tags).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to load tags")
		return
	}
	if tags == nil {
		tags = []models.TaskTag{}
	}
	respondOK(c, tags)
}

// errTagNotFound is a sentinel signaling that an assignment referenced an
// unknown tag id. It is translated to a 400 response by Assign.
var errTagNotFound = errors.New("tag not found")
