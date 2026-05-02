package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"botka/internal/models"
	"botka/internal/push"
)

// PushNotifier is the subset of internal/push.Sender used by the runner to
// notify users of task lifecycle events. Defined here (not imported directly)
// so tests can substitute a fake without depending on internal/push.
type PushNotifier interface {
	Send(ctx context.Context, userID int64, payload push.PushPayload) error
	Broadcast(ctx context.Context, payload push.PushPayload) error
}

// pushSendTimeout caps how long a single push delivery batch may take.
const pushSendTimeout = 30 * time.Second

// pushBodyMaxRunes bounds the body of a push notification so the OS
// notification center does not silently truncate it mid-word.
const pushBodyMaxRunes = 100

// truncateRunes returns s if it has no more than maxRunes runes, otherwise
// the first maxRunes runes followed by an ellipsis. Operates on runes (not
// bytes) so multibyte UTF-8 input is not chopped mid-character.
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// notifyTaskTransition fires a Web Push notification for a task that has just
// transitioned to a terminal state worth notifying about. It is best-effort:
// errors are logged and never propagated.
//
// Recipient: neither Task nor Project carry an owner relationship, so we
// Broadcast to every subscription. When per-task ownership is added, switch
// to Send(ownerUserID, …).
func (r *Runner) notifyTaskTransition(task *models.Task, status models.TaskStatus) {
	if r.pushNotifier == nil {
		return
	}
	if r.config != nil && !r.config.PushNotificationsEnabled {
		return
	}

	payload, ok := buildTaskPushPayload(task, status)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), pushSendTimeout)
	defer cancel()
	if err := r.pushNotifier.Broadcast(ctx, payload); err != nil {
		slog.Warn("task push notification failed",
			"task_id", task.ID, "status", status, "error", err)
	}
}

// buildTaskPushPayload returns the push payload for a task transition, or
// ok=false when the status does not warrant a notification.
func buildTaskPushPayload(task *models.Task, status models.TaskStatus) (push.PushPayload, bool) {
	switch status {
	case models.TaskStatusFailed:
		return push.PushPayload{
			Title: fmt.Sprintf("Task failed: %s", task.Title),
			Body:  failedPushBody(task),
			URL:   fmt.Sprintf("/tasks/%s", task.ID),
			Tag:   fmt.Sprintf("task-%s", task.ID),
		}, true
	case models.TaskStatusNeedsReview:
		return push.PushPayload{
			Title: fmt.Sprintf("Task needs review: %s", task.Title),
			Body:  "Verification command failed",
			URL:   fmt.Sprintf("/tasks/%s", task.ID),
			Tag:   fmt.Sprintf("task-%s", task.ID),
		}, true
	default:
		return push.PushPayload{}, false
	}
}

// failedPushBody picks the most informative description available for a
// failed task: the LLM-generated failure_summary if present, otherwise the
// raw failure_reason truncated to 100 chars, otherwise a generic fallback.
func failedPushBody(task *models.Task) string {
	if task.FailureSummary != nil && *task.FailureSummary != "" {
		return *task.FailureSummary
	}
	if task.FailureReason != nil && *task.FailureReason != "" {
		return truncateRunes(*task.FailureReason, pushBodyMaxRunes)
	}
	return "Task execution failed"
}
