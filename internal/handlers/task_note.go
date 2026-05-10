package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

// maxNoteBodyLength caps note bodies. Notes are free-form prose intended for
// triage notes and follow-ups, not full specs, so the limit is well below the
// task spec limit.
const maxNoteBodyLength = 10000

// TaskNoteHandler handles HTTP requests for the per-task notes resource.
type TaskNoteHandler struct {
	db *gorm.DB
}

// NewTaskNoteHandler creates a new TaskNoteHandler with the given database connection.
func NewTaskNoteHandler(db *gorm.DB) *TaskNoteHandler {
	return &TaskNoteHandler{db: db}
}

// RegisterTaskNoteRoutes attaches per-task notes endpoints to the given router group.
func RegisterTaskNoteRoutes(rg *gin.RouterGroup, h *TaskNoteHandler) {
	rg.GET("/tasks/:id/notes", h.List)
	rg.POST("/tasks/:id/notes", h.Create)
	rg.PATCH("/tasks/:id/notes/:note_id", h.Update)
	rg.DELETE("/tasks/:id/notes/:note_id", h.Delete)
}

// noteRequest is the JSON body for creating or updating a note.
type noteRequest struct {
	Body *string `json:"body"`
}

// List returns all non-deleted notes for a task ordered by created_at ASC.
func (h *TaskNoteHandler) List(c *gin.Context) {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	if !h.taskExists(c, taskID) {
		return
	}
	var notes []models.TaskNote
	if err := h.db.Where("task_id = ?", taskID).
		Order("created_at ASC, id ASC").
		Find(&notes).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list notes")
		return
	}
	if notes == nil {
		notes = []models.TaskNote{}
	}
	respondOK(c, notes)
}

// Create appends a new note to the task.
func (h *TaskNoteHandler) Create(c *gin.Context) {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	if !h.taskExists(c, taskID) {
		return
	}

	var req noteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	body, errMsg := validateNoteBody(req.Body)
	if errMsg != "" {
		respondError(c, http.StatusBadRequest, errMsg)
		return
	}

	note := models.TaskNote{TaskID: taskID, Body: body, Author: "user"}
	if err := h.db.Create(&note).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create note")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": note})
}

// Update edits an existing note's body, refreshing updated_at.
func (h *TaskNoteHandler) Update(c *gin.Context) {
	taskID, noteID, ok := h.parseTaskAndNoteID(c)
	if !ok {
		return
	}

	var req noteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	body, errMsg := validateNoteBody(req.Body)
	if errMsg != "" {
		respondError(c, http.StatusBadRequest, errMsg)
		return
	}

	var note models.TaskNote
	if err := h.db.Where("id = ? AND task_id = ?", noteID, taskID).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "note not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to load note")
		return
	}
	if err := h.db.Model(&note).Update("body", body).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update note")
		return
	}
	respondOK(c, note)
}

// Delete soft-deletes a note (sets deleted_at via GORM's soft-delete plugin).
func (h *TaskNoteHandler) Delete(c *gin.Context) {
	taskID, noteID, ok := h.parseTaskAndNoteID(c)
	if !ok {
		return
	}
	res := h.db.Where("id = ? AND task_id = ?", noteID, taskID).Delete(&models.TaskNote{})
	if res.Error != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete note")
		return
	}
	if res.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "note not found")
		return
	}
	c.Status(http.StatusNoContent)
}

// taskExists returns true and writes nothing when the task exists; otherwise
// it writes a 404/500 error response and returns false.
func (h *TaskNoteHandler) taskExists(c *gin.Context, taskID uuid.UUID) bool {
	var count int64
	if err := h.db.Model(&models.Task{}).Where("id = ?", taskID).Count(&count).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to look up task")
		return false
	}
	if count == 0 {
		respondError(c, http.StatusNotFound, "task not found")
		return false
	}
	return true
}

// parseTaskAndNoteID extracts both route params and writes an error response
// on failure. The boolean indicates whether the caller may proceed.
func (h *TaskNoteHandler) parseTaskAndNoteID(c *gin.Context) (uuid.UUID, int64, bool) {
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return uuid.Nil, 0, false
	}
	noteID, err := paramInt64(c, "note_id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid note id")
		return uuid.Nil, 0, false
	}
	return taskID, noteID, true
}

// validateNoteBody trims the request body and enforces non-emptiness and the
// length cap. Returns the trimmed body and an empty string on success.
func validateNoteBody(body *string) (string, string) {
	if body == nil {
		return "", "body is required"
	}
	trimmed := strings.TrimSpace(*body)
	if trimmed == "" {
		return "", "body is required"
	}
	if len(trimmed) > maxNoteBodyLength {
		return "", "body must be at most 10000 characters"
	}
	return trimmed, ""
}
