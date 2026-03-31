package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

const timeFmt = "02/01/2006 15:04"

// allowedTransitions defines valid status transitions for task updates.
// Mirrors the transition rules in the REST API handlers.
var allowedTransitions = map[models.TaskStatus]map[models.TaskStatus]bool{
	models.TaskStatusPending:     {models.TaskStatusQueued: true, models.TaskStatusCancelled: true},
	models.TaskStatusQueued:      {models.TaskStatusPending: true, models.TaskStatusCancelled: true},
	models.TaskStatusFailed:      {models.TaskStatusQueued: true, models.TaskStatusCancelled: true},
	models.TaskStatusNeedsReview: {models.TaskStatusQueued: true, models.TaskStatusDone: true},
	models.TaskStatusDeleted:     {models.TaskStatusPending: true},
}

// createTaskArgs holds the arguments for the create_task tool.
type createTaskArgs struct {
	Title       string `json:"title"`
	ProjectName string `json:"project_name"`
	Spec        string `json:"spec"`
	Priority    int    `json:"priority"`
	Status      string `json:"status"`
}

// validate checks required fields and returns the resolved task status.
func (a *createTaskArgs) validate() (models.TaskStatus, error) {
	if a.Title == "" {
		return "", errors.New("title is required")
	}
	if a.ProjectName == "" {
		return "", errors.New("project_name is required")
	}
	if a.Spec == "" {
		return "", errors.New("spec is required")
	}
	status := models.TaskStatusQueued
	if a.Status != "" {
		status = models.TaskStatus(a.Status)
		if status != models.TaskStatusPending && status != models.TaskStatusQueued {
			return "", errors.New("status must be pending or queued")
		}
	}
	return status, nil
}

// findTask looks up a task by UUID string.
func (s *Server) findTask(taskID string) (models.Task, error) {
	id, err := uuid.Parse(taskID)
	if err != nil {
		return models.Task{}, errors.New("invalid task_id")
	}
	var task models.Task
	if err := s.db.First(&task, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return task, errors.New("task not found")
		}
		return task, fmt.Errorf("failed to get task: %w", err)
	}
	return task, nil
}

// findProjectByName looks up an active project by case-insensitive name.
func (s *Server) findProjectByName(name string) (models.Project, error) {
	var project models.Project
	err := s.db.Where("LOWER(name) = LOWER(?) AND active = ?", name, true).First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return project, fmt.Errorf("project %q not found or inactive", name)
	}
	if err != nil {
		return project, fmt.Errorf("failed to look up project: %w", err)
	}
	return project, nil
}

// handleCreateTask creates a new task for the specified project.
func (s *Server) handleCreateTask(raw json.RawMessage) (interface{}, error) {
	var args createTaskArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	status, err := args.validate()
	if err != nil {
		return nil, err
	}

	project, err := s.findProjectByName(args.ProjectName)
	if err != nil {
		return nil, err
	}

	task := models.Task{
		Title:     args.Title,
		Spec:      args.Spec,
		Status:    status,
		Priority:  args.Priority,
		ProjectID: project.ID,
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return fmt.Sprintf("Created task %s '%s' for project %s with status %s and priority %d",
		task.ID, task.Title, project.Name, task.Status, task.Priority), nil
}

// listTasksArgs holds the arguments for the list_tasks tool.
type listTasksArgs struct {
	ProjectName string `json:"project_name"`
	Status      string `json:"status"`
	Limit       int    `json:"limit"`
}

// handleListTasks lists tasks with optional filtering by status or project.
func (s *Server) handleListTasks(raw json.RawMessage) (interface{}, error) {
	var args listTasksArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	limit := 20
	if args.Limit > 0 {
		limit = args.Limit
	}

	query := s.db.Preload("Project").Order("priority DESC, created_at ASC")

	if args.Status != "" {
		query = query.Where("status = ?", args.Status)
	} else {
		query = query.Where("status != ?", models.TaskStatusDeleted)
	}
	if args.ProjectName != "" {
		project, err := s.findProjectForFilter(args.ProjectName)
		if err != nil {
			return err, nil //nolint:nilerr // return message as result, not error
		}
		query = query.Where("project_id = ?", project.ID)
	}

	var tasks []models.Task
	if err := query.Limit(limit).Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(tasks) == 0 {
		return "No tasks found.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Tasks (%d results):\n", len(tasks))
	for _, t := range tasks {
		fmt.Fprintf(&b, "\n- [%s] %s (priority: %d)\n", t.Status, t.Title, t.Priority)
		fmt.Fprintf(&b, "  Project: %s | ID: %s | Created: %s\n",
			t.Project.Name, t.ID, t.CreatedAt.Format(timeFmt))
	}
	return b.String(), nil
}

// findProjectForFilter looks up a project for list filtering.
// Returns a string message (not error) when the project is not found,
// so the caller can return it as a tool result.
func (s *Server) findProjectForFilter(name string) (*models.Project, interface{}) {
	var project models.Project
	err := s.db.Where("LOWER(name) = LOWER(?)", name).First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "No tasks found (project not found)."
	}
	if err != nil {
		return nil, fmt.Sprintf("Failed to look up project: %s", err)
	}
	return &project, nil
}

// getTaskArgs holds the arguments for the get_task tool.
type getTaskArgs struct {
	TaskID string `json:"task_id"`
}

// handleGetTask returns detailed information about a task including executions.
func (s *Server) handleGetTask(raw json.RawMessage) (interface{}, error) {
	var args getTaskArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	id, err := uuid.Parse(args.TaskID)
	if err != nil {
		return nil, errors.New("invalid task_id")
	}

	var task models.Task
	query := s.db.Preload("Project").Preload("Executions", func(db *gorm.DB) *gorm.DB {
		return db.Order("attempt DESC")
	})
	if err := query.First(&task, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("task not found")
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return formatTaskDetail(&task), nil
}

// formatTaskDetail formats a task with all its details and executions.
func formatTaskDetail(task *models.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task: %s\n", task.Title)
	fmt.Fprintf(&b, "ID: %s\n", task.ID)
	fmt.Fprintf(&b, "Status: %s\n", task.Status)
	fmt.Fprintf(&b, "Priority: %d\n", task.Priority)
	fmt.Fprintf(&b, "Project: %s\n", task.Project.Name)
	fmt.Fprintf(&b, "Created: %s\n", task.CreatedAt.Format(timeFmt))
	fmt.Fprintf(&b, "Updated: %s\n", task.UpdatedAt.Format(timeFmt))
	if task.FailureReason != nil {
		fmt.Fprintf(&b, "Failure Reason: %s\n", *task.FailureReason)
	}
	fmt.Fprintf(&b, "\nSpec:\n%s\n", task.Spec)

	if len(task.Executions) > 0 {
		fmt.Fprintf(&b, "\nExecutions (%d):\n", len(task.Executions))
		for i := range task.Executions {
			formatExecution(&b, &task.Executions[i])
		}
	}
	return b.String()
}

// formatExecution appends a single execution's details to the builder.
func formatExecution(b *strings.Builder, e *models.TaskExecution) {
	fmt.Fprintf(b, "- Attempt %d", e.Attempt)
	if e.ExitCode != nil {
		fmt.Fprintf(b, ", exit_code=%d", *e.ExitCode)
	}
	if e.DurationMs != nil {
		fmt.Fprintf(b, ", duration=%dms", *e.DurationMs)
	}
	if e.CostUSD != nil {
		fmt.Fprintf(b, ", cost=$%.4f", *e.CostUSD)
	}
	fmt.Fprintln(b)
	if !e.StartedAt.IsZero() {
		fmt.Fprintf(b, "  Started: %s", e.StartedAt.Format(timeFmt))
		if e.FinishedAt != nil {
			fmt.Fprintf(b, " | Finished: %s", e.FinishedAt.Format(timeFmt))
		}
		fmt.Fprintln(b)
	}
	if e.Summary != nil {
		fmt.Fprintf(b, "  Summary: %s\n", *e.Summary)
	}
	if e.ErrorMessage != nil {
		fmt.Fprintf(b, "  Error: %s\n", *e.ErrorMessage)
	}
}

// updateTaskArgs holds the arguments for the update_task tool.
type updateTaskArgs struct {
	TaskID   string  `json:"task_id"`
	Title    *string `json:"title"`
	Spec     *string `json:"spec"`
	Priority *int    `json:"priority"`
	Status   *string `json:"status"`
}

// buildUpdates constructs the update map and list of changed fields.
// Returns an error if the status transition is not allowed.
func (a *updateTaskArgs) buildUpdates(
	current models.TaskStatus,
) (map[string]interface{}, []string, error) {
	updates := map[string]interface{}{}
	var changed []string

	if a.Title != nil {
		updates["title"] = *a.Title
		changed = append(changed, "title")
	}
	if a.Spec != nil {
		updates["spec"] = *a.Spec
		changed = append(changed, "spec")
	}
	if a.Priority != nil {
		updates["priority"] = *a.Priority
		changed = append(changed, "priority")
	}
	if a.Status != nil {
		newStatus := models.TaskStatus(*a.Status)
		if newStatus != current {
			allowed := allowedTransitions[current]
			if !allowed[newStatus] {
				return nil, nil, fmt.Errorf(
					"invalid status transition from %s to %s",
					current, newStatus,
				)
			}
		}
		updates["status"] = newStatus
		changed = append(changed, "status")
	}
	return updates, changed, nil
}

// handleUpdateTask updates a task's title, spec, priority, or status.
func (s *Server) handleUpdateTask(raw json.RawMessage) (interface{}, error) {
	var args updateTaskArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	task, err := s.findTask(args.TaskID)
	if err != nil {
		return nil, err
	}

	if task.Status == models.TaskStatusRunning {
		return nil, errors.New("cannot update a running task")
	}

	updates, changed, err := args.buildUpdates(task.Status)
	if err != nil {
		return nil, err
	}
	if len(updates) == 0 {
		return "No changes specified.", nil
	}

	if err := s.db.Model(&task).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update task: %w", err)
	}

	return fmt.Sprintf("Updated task %s: %s",
		task.ID, strings.Join(changed, ", ")), nil
}

// statusCount holds a status string and its count for aggregation queries.
type statusCount struct {
	Status string
	Count  int64
}

// handleListProjects lists all active projects with task count summaries.
func (s *Server) handleListProjects(_ json.RawMessage) (interface{}, error) {
	var projects []models.Project
	if err := s.db.Where("active = ?", true).Order("name ASC").Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	if len(projects) == 0 {
		return "No active projects.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Projects (%d):\n", len(projects))

	for _, p := range projects {
		var counts []statusCount
		s.db.Model(&models.Task{}).
			Select("status, COUNT(*) as count").
			Where("project_id = ?", p.ID).
			Group("status").
			Scan(&counts)

		fmt.Fprintf(&b, "\n- %s (%s)\n", p.Name, p.Path)
		fmt.Fprintf(&b, "  Branch: %s", p.BranchStrategy)

		countMap := make(map[string]int64)
		for _, c := range counts {
			countMap[c.Status] = c.Count
		}
		var parts []string
		for _, status := range []string{"queued", "running", "pending", "done", "failed", "needs_review"} {
			if n := countMap[status]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, status))
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(&b, " | Tasks: %s", strings.Join(parts, ", "))
		}
		fmt.Fprintln(&b)
	}

	return b.String(), nil
}

// handleGetRunnerStatus returns current task counts and running task details.
func (s *Server) handleGetRunnerStatus(_ json.RawMessage) (interface{}, error) {
	var counts []statusCount
	if err := s.db.Model(&models.Task{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&counts).Error; err != nil {
		return nil, fmt.Errorf("failed to count tasks: %w", err)
	}

	var b strings.Builder
	b.WriteString("Runner Status:\n\n")

	if len(counts) == 0 {
		b.WriteString("No tasks in the system.\n")
		return b.String(), nil
	}

	b.WriteString("Tasks by status: ")
	var parts []string
	for _, c := range counts {
		parts = append(parts, fmt.Sprintf("%d %s", c.Count, c.Status))
	}
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString("\n")

	var running []models.Task
	s.db.Preload("Project").Where("status = ?", "running").Find(&running)

	if len(running) > 0 {
		fmt.Fprintf(&b, "\nCurrently running (%d):\n", len(running))
		for _, t := range running {
			fmt.Fprintf(&b, "- %q (project: %s, started: %s)\n",
				t.Title, t.Project.Name, t.UpdatedAt.Format(timeFmt))
		}
	}

	return b.String(), nil
}

// handleKillTask kills a running task by ID.
func (s *Server) handleKillTask(raw json.RawMessage) (interface{}, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("runner control is not available in stdio mode")
	}

	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.TaskID == "" {
		return nil, errors.New("task_id is required")
	}

	id, err := uuid.Parse(args.TaskID)
	if err != nil {
		return nil, errors.New("invalid task_id")
	}

	if err := s.runner.KillTask(id); err != nil {
		return nil, err
	}
	return fmt.Sprintf("Kill initiated for task %s. Git changes will be reverted.", id), nil
}

// handleStartRunner starts or resumes the task runner.
// Returns an error in stdio mode where the runner is not available.
func (s *Server) handleStartRunner(args json.RawMessage) (interface{}, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("runner control is not available in stdio mode")
	}

	var params struct {
		Count int `json:"count"`
	}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &params) //nolint:errcheck // best-effort parse
	}

	if params.Count > 0 {
		s.runner.StartN(params.Count)
		return fmt.Sprintf("Runner started for %d tasks.", params.Count), nil
	}
	s.runner.Resume()
	return "Runner started (unlimited).", nil
}
