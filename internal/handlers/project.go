package handlers

import (
	"errors"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
	"botka/internal/projects"
)

// ScanFunc is the signature for the project discovery scan function.
type ScanFunc func(projectsDir string) ([]projects.DiscoveredProject, error)

// SyncFunc is the signature for the project discovery database sync function.
type SyncFunc func(db *gorm.DB, discovered []projects.DiscoveredProject) error

// ProjectHandler handles HTTP requests for project resources.
type ProjectHandler struct {
	db          *gorm.DB
	projectsDir string
	scanFn      ScanFunc
	syncFn      SyncFunc
}

// NewProjectHandler creates a new ProjectHandler with the given dependencies.
func NewProjectHandler(db *gorm.DB, projectsDir string, scanFn ScanFunc, syncFn SyncFunc) *ProjectHandler {
	return &ProjectHandler{
		db:          db,
		projectsDir: projectsDir,
		scanFn:      scanFn,
		syncFn:      syncFn,
	}
}

// RegisterProjectRoutes attaches project endpoints to the given router group.
func RegisterProjectRoutes(rg *gin.RouterGroup, h *ProjectHandler) {
	rg.GET("/projects", h.List)
	rg.GET("/projects/:id", h.Get)
	rg.PUT("/projects/:id", h.Update)
	rg.POST("/projects/scan", h.Scan)
	rg.GET("/projects/:id/git-log", h.GetGitLog)
	rg.GET("/projects/:id/git-status", h.GetGitStatus)
	rg.GET("/projects/:id/stats", h.GetStats)
}

// taskCounts holds per-status task counts for a project.
type taskCounts struct {
	Pending     int64 `json:"pending"`
	Queued      int64 `json:"queued"`
	Running     int64 `json:"running"`
	Done        int64 `json:"done"`
	Failed      int64 `json:"failed"`
	NeedsReview int64 `json:"needs_review"`
	Cancelled   int64 `json:"cancelled"`
}

// projectResponse is the JSON representation of a project with task and thread counts.
type projectResponse struct {
	ID                  uuid.UUID  `json:"id"`
	Name                string     `json:"name"`
	Path                string     `json:"path"`
	BranchStrategy      string     `json:"branch_strategy"`
	VerificationCommand *string    `json:"verification_command"`
	Active              bool       `json:"active"`
	ClaudeMD            string     `json:"claude_md"`
	SortOrder           int        `json:"sort_order"`
	TaskCounts          taskCounts `json:"task_counts"`
	ThreadCount         int64      `json:"thread_count"`
}

// List returns all active projects with their task and thread counts.
func (h *ProjectHandler) List(c *gin.Context) {
	var projs []models.Project
	if err := h.db.Where("active = ?", true).Order("sort_order ASC, name ASC").Find(&projs).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list projects")
		return
	}

	responses := make([]projectResponse, 0, len(projs))
	for i := range projs {
		resp, err := h.buildProjectResponse(&projs[i])
		if err != nil {
			respondError(c, http.StatusInternalServerError, "failed to build project response")
			return
		}
		responses = append(responses, resp)
	}

	respondList(c, responses, int64(len(responses)))
}

// Get returns a single project by ID with its task and thread counts.
func (h *ProjectHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	proj, ok := h.findProject(c, id)
	if !ok {
		return
	}

	resp, err := h.buildProjectResponse(&proj)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to build project response")
		return
	}

	respondOK(c, resp)
}

// updateProjectRequest is the JSON body for updating a project.
type updateProjectRequest struct {
	BranchStrategy      *string `json:"branch_strategy"`
	VerificationCommand *string `json:"verification_command"`
	ClaudeMD            *string `json:"claude_md"`
	SortOrder           *int    `json:"sort_order"`
	Active              *bool   `json:"active"`
}

// Update modifies the configuration of a project.
func (h *ProjectHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	var req updateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := req.validate(); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	proj, ok := h.findProject(c, id)
	if !ok {
		return
	}

	if err := h.applyProjectUpdates(&proj, req); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update project")
		return
	}

	resp, err := h.buildProjectResponse(&proj)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to build project response")
		return
	}

	respondOK(c, resp)
}

// validate checks that the update request contains valid values.
func (r *updateProjectRequest) validate() error {
	if r.BranchStrategy != nil &&
		*r.BranchStrategy != "main" &&
		*r.BranchStrategy != "feature_branch" {
		return errors.New("branch_strategy must be \"main\" or \"feature_branch\"")
	}
	return nil
}

// applyProjectUpdates persists the requested field changes and re-reads the project.
func (h *ProjectHandler) applyProjectUpdates(proj *models.Project, req updateProjectRequest) error {
	updates := map[string]interface{}{}
	if req.BranchStrategy != nil {
		updates["branch_strategy"] = *req.BranchStrategy
	}
	if req.VerificationCommand != nil {
		updates["verification_command"] = *req.VerificationCommand
	}
	if req.ClaudeMD != nil {
		updates["claude_md"] = *req.ClaudeMD
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}
	if req.Active != nil {
		updates["active"] = *req.Active
	}

	if len(updates) == 0 {
		return nil
	}

	if err := h.db.Model(proj).Updates(updates).Error; err != nil {
		return err
	}
	return h.db.First(proj, "id = ?", proj.ID).Error
}

// scanResponse is the JSON body returned after a project scan.
type scanResponse struct {
	Discovered  int `json:"discovered"`
	New         int `json:"new"`
	Deactivated int `json:"deactivated"`
}

// Scan triggers project re-discovery and database sync.
func (h *ProjectHandler) Scan(c *gin.Context) {
	existingPaths, err := h.activeProjectPaths()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to query projects")
		return
	}

	discovered, err := h.scanFn(h.projectsDir)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "scan failed: "+err.Error())
		return
	}

	if err := h.syncFn(h.db, discovered); err != nil {
		respondError(c, http.StatusInternalServerError, "sync failed: "+err.Error())
		return
	}

	discoveredPaths := make(map[string]struct{}, len(discovered))
	newCount := 0
	for _, dp := range discovered {
		discoveredPaths[dp.Path] = struct{}{}
		if _, existed := existingPaths[dp.Path]; !existed {
			newCount++
		}
	}

	deactivatedCount := 0
	for path := range existingPaths {
		if _, found := discoveredPaths[path]; !found {
			deactivatedCount++
		}
	}

	respondOK(c, scanResponse{
		Discovered:  len(discovered),
		New:         newCount,
		Deactivated: deactivatedCount,
	})
}

// findProject looks up a project by ID and writes the appropriate error response if not found.
func (h *ProjectHandler) findProject(c *gin.Context, id uuid.UUID) (models.Project, bool) {
	var proj models.Project
	if err := h.db.First(&proj, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusNotFound, "project not found")
			return proj, false
		}
		respondError(c, http.StatusInternalServerError, "failed to get project")
		return proj, false
	}
	return proj, true
}

// activeProjectPaths returns a set of paths for all currently active projects.
func (h *ProjectHandler) activeProjectPaths() (map[string]struct{}, error) {
	var projs []models.Project
	if err := h.db.Where("active = ?", true).Find(&projs).Error; err != nil {
		return nil, err
	}
	paths := make(map[string]struct{}, len(projs))
	for _, p := range projs {
		paths[p.Path] = struct{}{}
	}
	return paths, nil
}

// countTasks returns task counts grouped by status for the given project ID.
func (h *ProjectHandler) countTasks(projectID uuid.UUID) (taskCounts, error) {
	type statusCount struct {
		Status string
		Count  int64
	}

	var rows []statusCount
	err := h.db.Model(&models.Task{}).
		Select("status, count(*) as count").
		Where("project_id = ?", projectID).
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return taskCounts{}, err
	}

	var counts taskCounts
	for _, row := range rows {
		switch models.TaskStatus(row.Status) {
		case models.TaskStatusPending:
			counts.Pending = row.Count
		case models.TaskStatusQueued:
			counts.Queued = row.Count
		case models.TaskStatusRunning:
			counts.Running = row.Count
		case models.TaskStatusDone:
			counts.Done = row.Count
		case models.TaskStatusFailed:
			counts.Failed = row.Count
		case models.TaskStatusNeedsReview:
			counts.NeedsReview = row.Count
		case models.TaskStatusCancelled:
			counts.Cancelled = row.Count
		}
	}

	return counts, nil
}

// countThreads returns the number of threads associated with a project.
func (h *ProjectHandler) countThreads(projectID uuid.UUID) (int64, error) {
	var count int64
	err := h.db.Model(&models.Thread{}).
		Where("project_id = ?", projectID).
		Count(&count).Error
	return count, err
}

// gitCommit represents a single git log entry.
type gitCommit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

// GetGitLog returns recent commit history for a project's git repository.
func (h *ProjectHandler) GetGitLog(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	proj, ok := h.findProject(c, id)
	if !ok {
		return
	}

	cmd := exec.Command("git", "log", "--format=%H\t%an\t%aI\t%s", "-50")
	cmd.Dir = proj.Path
	out, err := cmd.Output()
	if err != nil {
		respondOK(c, []gitCommit{})
		return
	}

	var commits []gitCommit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, gitCommit{
			Hash:    parts[0][:12],
			Author:  parts[1],
			Date:    parts[2],
			Message: parts[3],
		})
	}

	if commits == nil {
		commits = []gitCommit{}
	}
	respondOK(c, commits)
}

// gitStatusResponse represents the current git status of a project directory.
type gitStatusResponse struct {
	Branch       string        `json:"branch"`
	Clean        bool          `json:"clean"`
	ChangedFiles []changedFile `json:"changed_files"`
	DiffStat     string        `json:"diff_stat"`
}

// changedFile represents a single file in the git status output.
type changedFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// GetGitStatus returns the current git status of a project directory.
func (h *ProjectHandler) GetGitStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	proj, ok := h.findProject(c, id)
	if !ok {
		return
	}

	resp := gitStatusResponse{ChangedFiles: []changedFile{}}

	// Get current branch
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = proj.Path
	branchOut, err := branchCmd.Output()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "not a git repository")
		return
	}
	resp.Branch = strings.TrimSpace(string(branchOut))

	// Get porcelain status
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = proj.Path
	statusOut, err := statusCmd.Output()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to get git status")
		return
	}

	statusStr := strings.TrimSpace(string(statusOut))
	resp.Clean = statusStr == ""
	if !resp.Clean {
		for _, line := range strings.Split(statusStr, "\n") {
			if len(line) < 4 {
				continue
			}
			resp.ChangedFiles = append(resp.ChangedFiles, changedFile{
				Status: strings.TrimSpace(line[:2]),
				Path:   line[3:],
			})
		}
	}

	// Get diff stat
	diffCmd := exec.Command("git", "diff", "--stat")
	diffCmd.Dir = proj.Path
	diffOut, _ := diffCmd.Output()
	resp.DiffStat = strings.TrimSpace(string(diffOut))

	respondOK(c, resp)
}

// projectStats holds aggregate task statistics for a project.
type projectStats struct {
	Total         int64      `json:"total"`
	ByStatus      taskCounts `json:"by_status"`
	AvgDurationMs *float64   `json:"avg_duration_ms"`
	SuccessRate   *float64   `json:"success_rate"`
	TotalCostUSD  *float64   `json:"total_cost_usd"`
}

// GetStats returns aggregate task statistics for a project.
func (h *ProjectHandler) GetStats(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	_, ok := h.findProject(c, id)
	if !ok {
		return
	}

	counts, err := h.countTasks(id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to count tasks")
		return
	}

	total := counts.Pending + counts.Queued + counts.Running + counts.Done + counts.Failed + counts.NeedsReview + counts.Cancelled

	stats := projectStats{
		Total:    total,
		ByStatus: counts,
	}

	// Compute average duration from completed executions
	var avgDuration struct {
		Avg *float64
	}
	h.db.Model(&models.TaskExecution{}).
		Select("AVG(duration_ms) as avg").
		Joins("JOIN tasks ON tasks.id = task_executions.task_id").
		Where("tasks.project_id = ? AND task_executions.duration_ms IS NOT NULL", id).
		Scan(&avgDuration)
	stats.AvgDurationMs = avgDuration.Avg

	// Compute success rate (done / (done + failed))
	completed := counts.Done + counts.Failed
	if completed > 0 {
		rate := float64(counts.Done) / float64(completed)
		stats.SuccessRate = &rate
	}

	// Compute total cost
	var totalCost struct {
		Sum *float64
	}
	h.db.Model(&models.TaskExecution{}).
		Select("SUM(cost_usd) as sum").
		Joins("JOIN tasks ON tasks.id = task_executions.task_id").
		Where("tasks.project_id = ? AND task_executions.cost_usd IS NOT NULL", id).
		Scan(&totalCost)
	stats.TotalCostUSD = totalCost.Sum

	respondOK(c, stats)
}

// buildProjectResponse assembles a projectResponse with task and thread counts.
func (h *ProjectHandler) buildProjectResponse(p *models.Project) (projectResponse, error) {
	counts, err := h.countTasks(p.ID)
	if err != nil {
		return projectResponse{}, err
	}

	threadCount, err := h.countThreads(p.ID)
	if err != nil {
		return projectResponse{}, err
	}

	return projectResponse{
		ID:                  p.ID,
		Name:                p.Name,
		Path:                p.Path,
		BranchStrategy:      p.BranchStrategy,
		VerificationCommand: p.VerificationCommand,
		Active:              p.Active,
		ClaudeMD:            p.ClaudeMD,
		SortOrder:           p.SortOrder,
		TaskCounts:          counts,
		ThreadCount:         threadCount,
	}, nil
}
