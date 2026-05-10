package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"botka/internal/models"
)

// Bulk operation identifiers, mirroring the REST handler.
const (
	bulkOpCancel      = "cancel"
	bulkOpRequeue     = "requeue"
	bulkOpSetPending  = "set_pending"
	bulkOpDelete      = "delete"
	bulkOpSetPriority = "set_priority"
	bulkOpAddTags     = "add_tags"
	bulkOpRemoveTags  = "remove_tags"

	bulkMaxIDs = 100
)

// bulkStatusOpTargets maps each status-changing operation to its target.
var bulkStatusOpTargets = map[string]models.TaskStatus{
	bulkOpCancel:     models.TaskStatusCancelled,
	bulkOpRequeue:    models.TaskStatusQueued,
	bulkOpSetPending: models.TaskStatusPending,
}

// bulkUpdateArgs holds the arguments for the bulk_update_tasks tool. The
// payload is operation-specific and is parsed lazily based on the operation.
type bulkUpdateArgs struct {
	TaskIDs   []string        `json:"task_ids"`
	Operation string          `json:"operation"`
	Payload   json.RawMessage `json:"payload"`
}

// handleBulkUpdateTasks applies a single operation to up to bulkMaxIDs task IDs
// in their own transactions and returns a text summary. Per-task failures do
// not abort the batch; they are listed individually.
func (s *Server) handleBulkUpdateTasks(raw json.RawMessage) (interface{}, error) {
	var args bulkUpdateArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	ids, err := parseBulkTaskIDs(args.TaskIDs)
	if err != nil {
		return nil, err
	}

	apply, err := s.buildBulkApplier(args.Operation, args.Payload)
	if err != nil {
		return nil, err
	}

	type result struct {
		id  uuid.UUID
		err error
	}
	results := make([]result, 0, len(ids))
	for _, id := range ids {
		results = append(results, result{id: id, err: apply(id)})
	}

	var b strings.Builder
	succeeded := 0
	for _, r := range results {
		if r.err == nil {
			succeeded++
		}
	}
	failed := len(results) - succeeded
	fmt.Fprintf(&b, "Bulk %s: %d succeeded, %d failed\n", args.Operation, succeeded, failed)
	if failed > 0 {
		b.WriteString("\nFailures:\n")
		for _, r := range results {
			if r.err != nil {
				fmt.Fprintf(&b, "- %s: %s\n", r.id, r.err.Error())
			}
		}
	}
	return b.String(), nil
}

// parseBulkTaskIDs parses, validates, and deduplicates the task ID list.
// Returns an error if the list is empty, exceeds bulkMaxIDs, contains an
// invalid UUID, or contains duplicates.
func parseBulkTaskIDs(raw []string) ([]uuid.UUID, error) {
	if len(raw) == 0 {
		return nil, errors.New("task_ids must not be empty")
	}
	if len(raw) > bulkMaxIDs {
		return nil, fmt.Errorf("task_ids exceeds maximum of %d", bulkMaxIDs)
	}
	seen := make(map[uuid.UUID]struct{}, len(raw))
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("invalid task_id: %s", s)
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("duplicate task_id: %s", id)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

// buildBulkApplier returns a closure that applies the operation to one task ID.
// The closure transactionally locks the row and enforces the same status
// transitions used by single-task updates.
func (s *Server) buildBulkApplier(
	operation string, payload json.RawMessage,
) (func(uuid.UUID) error, error) {
	if target, ok := bulkStatusOpTargets[operation]; ok {
		return func(id uuid.UUID) error {
			return bulkSetStatus(s.db, id, target)
		}, nil
	}

	switch operation {
	case bulkOpDelete:
		return func(id uuid.UUID) error {
			return bulkDelete(s.db, id)
		}, nil

	case bulkOpSetPriority:
		var p struct {
			Priority int `json:"priority"`
		}
		if len(payload) == 0 || json.Unmarshal(payload, &p) != nil {
			return nil, errors.New(`payload must be {"priority": <int>}`)
		}
		return func(id uuid.UUID) error {
			return bulkSetPriority(s.db, id, p.Priority)
		}, nil

	case bulkOpAddTags, bulkOpRemoveTags:
		tagIDs, err := s.parseTagsPayload(payload)
		if err != nil {
			return nil, err
		}
		op := operation
		return func(id uuid.UUID) error {
			if op == bulkOpAddTags {
				return bulkAddTags(s.db, id, tagIDs)
			}
			return bulkRemoveTags(s.db, id, tagIDs)
		}, nil

	default:
		return nil, fmt.Errorf("invalid operation: %s", operation)
	}
}

// parseTagsPayload decodes and validates the {"tag_ids": [...]} payload.
// The tag_ids list must be non-empty and reference real tags.
func (s *Server) parseTagsPayload(raw json.RawMessage) ([]int64, error) {
	if len(raw) == 0 {
		return nil, errors.New(`payload must be {"tag_ids": [<int>, ...]}`)
	}
	var p struct {
		TagIDs []int64 `json:"tag_ids"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, errors.New(`payload must be {"tag_ids": [<int>, ...]}`)
	}
	if len(p.TagIDs) == 0 {
		return nil, errors.New("tag_ids must not be empty")
	}
	seen := make(map[int64]struct{}, len(p.TagIDs))
	for _, id := range p.TagIDs {
		seen[id] = struct{}{}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	var count int64
	if err := s.db.Model(&models.TaskTag{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("failed to validate tag_ids: %w", err)
	}
	if count != int64(len(ids)) {
		return nil, errors.New("one or more tag_ids do not exist")
	}
	return ids, nil
}

// bulkLockTask fetches a task with FOR UPDATE locking and a stable not-found error.
func bulkLockTask(tx *gorm.DB, id uuid.UUID) (models.Task, error) {
	var task models.Task
	err := tx.Set("gorm:query_option", "FOR UPDATE").First(&task, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return task, errors.New("task not found")
		}
		return task, err
	}
	return task, nil
}

// bulkSetStatus changes a task's status, enforcing transition rules.
func bulkSetStatus(db *gorm.DB, id uuid.UUID, status models.TaskStatus) error {
	return db.Transaction(func(tx *gorm.DB) error {
		task, err := bulkLockTask(tx, id)
		if err != nil {
			return err
		}
		if task.Status == status {
			return nil
		}
		if task.Status == models.TaskStatusRunning {
			return errors.New("cannot change status of a running task")
		}
		allowed, ok := allowedTransitions[task.Status]
		if !ok || !allowed[status] {
			return fmt.Errorf("cannot transition from %s to %s", task.Status, status)
		}
		return tx.Model(&task).Update("status", status).Error
	})
}

// bulkDelete soft-deletes a task by setting status to deleted.
func bulkDelete(db *gorm.DB, id uuid.UUID) error {
	return db.Transaction(func(tx *gorm.DB) error {
		task, err := bulkLockTask(tx, id)
		if err != nil {
			return err
		}
		if task.Status == models.TaskStatusRunning {
			return errors.New("cannot delete a running task")
		}
		if task.Status == models.TaskStatusDeleted {
			return nil
		}
		return tx.Model(&task).Update("status", models.TaskStatusDeleted).Error
	})
}

// bulkSetPriority sets a task's priority unconditionally.
func bulkSetPriority(db *gorm.DB, id uuid.UUID, priority int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		task, err := bulkLockTask(tx, id)
		if err != nil {
			return err
		}
		return tx.Model(&task).Update("priority", priority).Error
	})
}

// bulkAddTags attaches tag IDs to a task; existing assignments are kept.
func bulkAddTags(db *gorm.DB, taskID uuid.UUID, tagIDs []int64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var task models.Task
		if err := tx.Select("id").First(&task, "id = ?", taskID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("task not found")
			}
			return err
		}
		assignments := make([]models.TaskTagAssignment, 0, len(tagIDs))
		for _, tagID := range tagIDs {
			assignments = append(assignments, models.TaskTagAssignment{TaskID: taskID, TagID: tagID})
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&assignments).Error
	})
}

// bulkRemoveTags detaches tag IDs from a task; missing assignments are ignored.
func bulkRemoveTags(db *gorm.DB, taskID uuid.UUID, tagIDs []int64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var task models.Task
		if err := tx.Select("id").First(&task, "id = ?", taskID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("task not found")
			}
			return err
		}
		return tx.Where("task_id = ? AND tag_id IN ?", taskID, tagIDs).
			Delete(&models.TaskTagAssignment{}).Error
	})
}
