package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"botka/internal/models"
)

// addTaskNoteArgs holds the arguments for the add_task_note tool.
type addTaskNoteArgs struct {
	TaskID string `json:"task_id"`
	Body   string `json:"body"`
}

// listTaskNotesArgs holds the arguments for the list_task_notes tool.
type listTaskNotesArgs struct {
	TaskID string `json:"task_id"`
}

// handleAddTaskNote appends a free-form note to a task. The author is fixed
// to "user" for now — see internal/models/task_note.go for forward plans.
func (s *Server) handleAddTaskNote(raw json.RawMessage) (interface{}, error) {
	var args addTaskNoteArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	body := strings.TrimSpace(args.Body)
	if body == "" {
		return nil, errors.New("body is required")
	}

	task, err := s.findTask(args.TaskID)
	if err != nil {
		return nil, err
	}

	note := models.TaskNote{TaskID: task.ID, Body: body, Author: "user"}
	if err := s.db.Create(&note).Error; err != nil {
		return nil, fmt.Errorf("failed to create note: %w", err)
	}
	return fmt.Sprintf("Added note %d to task %s", note.ID, task.ID), nil
}

// handleListTaskNotes returns the non-deleted notes attached to a task,
// ordered by created_at ASC.
func (s *Server) handleListTaskNotes(raw json.RawMessage) (interface{}, error) {
	var args listTaskNotesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	task, err := s.findTask(args.TaskID)
	if err != nil {
		return nil, err
	}

	var notes []models.TaskNote
	if err := s.db.Where("task_id = ?", task.ID).
		Order("created_at ASC, id ASC").
		Find(&notes).Error; err != nil {
		return nil, fmt.Errorf("failed to list notes: %w", err)
	}

	if len(notes) == 0 {
		return fmt.Sprintf("No notes on task %s.", task.ID), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Notes on task %s (%d):\n", task.ID, len(notes))
	for i := range notes {
		n := &notes[i]
		fmt.Fprintf(&b, "\n#%d by %s — %s\n%s\n",
			n.ID, n.Author, n.CreatedAt.Format(timeFmt), n.Body)
	}
	return b.String(), nil
}
