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

// emptyDiffResult is returned (with HTTP 200) when a task has no recorded
// commit range yet — either because it never ran or because it was cancelled
// before producing commits. Front-ends use this to render an empty Changes
// section rather than an error.
var emptyDiffResult = runner.DiffResult{}

// Diff returns the unified git diff between a task's recorded base and head
// commits along with summary stats and a `truncated` flag. The endpoint:
//
//   - returns 404 if the task itself does not exist;
//   - returns 200 with an empty result if either SHA is missing on the task
//     row, or if base equals head (the task ran but committed nothing);
//   - returns 404 with "commit not found in repository" if either SHA exists
//     on the task row but cannot be resolved by git (e.g. the branch was
//     rebased away);
//   - otherwise returns the unified diff capped at runner.MaxDiffBytes,
//     setting truncated=true when the cap was hit.
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
		respondOK(c, emptyDiffResult)
		return
	}
	if *task.BaseCommitSHA == *task.HeadCommitSHA {
		respondOK(c, emptyDiffResult)
		return
	}

	result, err := runner.GitDiff(
		&task.Project, h.boxWaker, h.boxSSHTarget,
		*task.BaseCommitSHA, *task.HeadCommitSHA,
	)
	if err != nil {
		var missing *runner.CommitMissingError
		if errors.As(err, &missing) {
			respondError(c, http.StatusNotFound, "commit not found in repository")
			return
		}
		respondError(c, http.StatusInternalServerError, "failed to compute diff: "+err.Error())
		return
	}

	respondOK(c, result)
}
