package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/runner"
)

// Diff returns the unified diff between a task's recorded base and head
// commits. Returns 404 when either SHA is missing (task never produced
// commits) or when the two SHAs are equal (task ran but committed nothing).
func (h *TaskHandler) Diff(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}

	var task models.Task
	if err := h.db.Preload("Project").First(&task, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "task not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to get task")
		return
	}

	if task.BaseCommitSHA == nil || task.HeadCommitSHA == nil {
		respondError(c, http.StatusNotFound, "task did not produce a recorded commit range")
		return
	}
	if *task.BaseCommitSHA == *task.HeadCommitSHA {
		respondError(c, http.StatusNotFound, "task produced no commits")
		return
	}

	result, err := runner.GitDiff(
		&task.Project, h.boxWaker, h.boxSSHTarget,
		*task.BaseCommitSHA, *task.HeadCommitSHA,
	)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to compute diff: "+err.Error())
		return
	}

	respondOK(c, result)
}
