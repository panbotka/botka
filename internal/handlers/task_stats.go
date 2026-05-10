package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// TaskStatsHandler serves the aggregated /stats/tasks endpoint, which returns
// task-level token and cost rollups grouped by project, model, or day.
type TaskStatsHandler struct {
	db *gorm.DB
}

// NewTaskStatsHandler constructs a handler bound to the given database.
func NewTaskStatsHandler(db *gorm.DB) *TaskStatsHandler {
	return &TaskStatsHandler{db: db}
}

// RegisterTaskStatsRoutes attaches the /stats/tasks route to rg.
func RegisterTaskStatsRoutes(rg *gin.RouterGroup, h *TaskStatsHandler) {
	rg.GET("/stats/tasks", h.Aggregate)
}

// taskStatsBucket is one row of the aggregated response. The grouping key is
// emitted in the field that corresponds to the requested group_by value; the
// other key fields are left as zero values so consumers can switch on them.
type taskStatsBucket struct {
	Day                 *string `json:"day,omitempty"`
	Project             *string `json:"project,omitempty"`
	ProjectID           *string `json:"project_id,omitempty"`
	Model               *string `json:"model,omitempty"`
	TaskCount           int64   `json:"task_count"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CostUSD             float64 `json:"cost_usd"`
}

// taskStatsResponse wraps the aggregation result.
type taskStatsResponse struct {
	From    string            `json:"from"`
	To      string            `json:"to"`
	GroupBy string            `json:"group_by"`
	Buckets []taskStatsBucket `json:"buckets"`
}

// Aggregate handles GET /stats/tasks. It accepts:
//   - from=YYYY-MM-DD (default: 30 days ago)
//   - to=YYYY-MM-DD   (default: today, inclusive)
//   - group_by=project|model|day or "day,project" / "day,model" (default: day)
//
// Aggregation is performed in a single SQL query against the tasks table using
// completed_at as the time anchor — only tasks with a completed_at within the
// window contribute to the totals. The composite "day,project" / "day,model"
// shapes power stacked-bar charts on the frontend without forcing the client
// to make N queries.
func (h *TaskStatsHandler) Aggregate(c *gin.Context) {
	groupBy := c.DefaultQuery("group_by", "day")
	if !validGroupBy(groupBy) {
		respondError(c, http.StatusBadRequest,
			"group_by must be one of: day, project, model, day,project, day,model")
		return
	}

	from, to, err := parseDateRange(
		c.Query("from"), c.Query("to"),
		time.Now().AddDate(0, 0, -30), time.Now(),
	)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	buckets, err := h.queryBuckets(groupBy, from, to)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to aggregate task stats")
		return
	}

	respondOK(c, taskStatsResponse{
		From:    from.Format("2006-01-02"),
		To:      to.Format("2006-01-02"),
		GroupBy: groupBy,
		Buckets: buckets,
	})
}

// validGroupBy reports whether s names a supported grouping shape.
func validGroupBy(s string) bool {
	switch s {
	case "day", "project", "model", "day,project", "day,model":
		return true
	}
	return false
}

// queryBuckets dispatches to the appropriate per-group aggregation. The
// queries differ enough (date_trunc vs join vs COALESCE) that templating one
// SQL string would be more confusing than keeping the explicit shapes.
func (h *TaskStatsHandler) queryBuckets(
	groupBy string, from, to time.Time,
) ([]taskStatsBucket, error) {
	// Use end of day for `to` so the window includes the full day.
	toEnd := time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 999_999_999, to.Location())

	switch groupBy {
	case "day":
		return h.queryByDay(from, toEnd)
	case "project":
		return h.queryByProject(from, toEnd)
	case "model":
		return h.queryByModel(from, toEnd)
	case "day,project":
		return h.queryByDayAndProject(from, toEnd)
	case "day,model":
		return h.queryByDayAndModel(from, toEnd)
	}
	return nil, fmt.Errorf("unsupported group_by: %s", groupBy)
}

// queryByDay returns one bucket per UTC day with completed tasks in the window.
func (h *TaskStatsHandler) queryByDay(from, to time.Time) ([]taskStatsBucket, error) {
	type row struct {
		Day                 string
		TaskCount           int64
		InputTokens         int64
		OutputTokens        int64
		CacheReadTokens     int64
		CacheCreationTokens int64
		CostUSD             float64
	}
	var rows []row
	if err := h.db.Raw(`
		SELECT to_char(date_trunc('day', completed_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
			COUNT(*) AS task_count,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(SUM(cost_usd), 0) AS cost_usd
		FROM tasks
		WHERE completed_at IS NOT NULL
			AND completed_at >= ? AND completed_at <= ?
			AND status != ?
		GROUP BY day
		ORDER BY day ASC
	`, from, to, models.TaskStatusDeleted).Scan(&rows).Error; err != nil {
		return nil, err
	}
	buckets := make([]taskStatsBucket, 0, len(rows))
	for i := range rows {
		day := rows[i].Day
		buckets = append(buckets, taskStatsBucket{
			Day:                 &day,
			TaskCount:           rows[i].TaskCount,
			InputTokens:         rows[i].InputTokens,
			OutputTokens:        rows[i].OutputTokens,
			CacheReadTokens:     rows[i].CacheReadTokens,
			CacheCreationTokens: rows[i].CacheCreationTokens,
			CostUSD:             rows[i].CostUSD,
		})
	}
	return buckets, nil
}

// queryByProject groups by project, joining tasks → projects to surface the
// project name. Costs are sorted descending so consumers can render top-spend
// charts without re-sorting.
func (h *TaskStatsHandler) queryByProject(from, to time.Time) ([]taskStatsBucket, error) {
	type row struct {
		ProjectID           string
		ProjectName         string
		TaskCount           int64
		InputTokens         int64
		OutputTokens        int64
		CacheReadTokens     int64
		CacheCreationTokens int64
		CostUSD             float64
	}
	var rows []row
	if err := h.db.Raw(`
		SELECT t.project_id::text AS project_id,
			p.name AS project_name,
			COUNT(*) AS task_count,
			COALESCE(SUM(t.input_tokens), 0) AS input_tokens,
			COALESCE(SUM(t.output_tokens), 0) AS output_tokens,
			COALESCE(SUM(t.cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(t.cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(SUM(t.cost_usd), 0) AS cost_usd
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE t.completed_at IS NOT NULL
			AND t.completed_at >= ? AND t.completed_at <= ?
			AND t.status != ?
		GROUP BY t.project_id, p.name
		ORDER BY cost_usd DESC, task_count DESC
	`, from, to, models.TaskStatusDeleted).Scan(&rows).Error; err != nil {
		return nil, err
	}
	buckets := make([]taskStatsBucket, 0, len(rows))
	for i := range rows {
		pid := rows[i].ProjectID
		name := rows[i].ProjectName
		buckets = append(buckets, taskStatsBucket{
			Project:             &name,
			ProjectID:           &pid,
			TaskCount:           rows[i].TaskCount,
			InputTokens:         rows[i].InputTokens,
			OutputTokens:        rows[i].OutputTokens,
			CacheReadTokens:     rows[i].CacheReadTokens,
			CacheCreationTokens: rows[i].CacheCreationTokens,
			CostUSD:             rows[i].CostUSD,
		})
	}
	return buckets, nil
}

// queryByModel groups by the model column, COALESCE'd to "unknown" for the
// (common) case where pre-tracking tasks ran without a captured model name.
func (h *TaskStatsHandler) queryByModel(from, to time.Time) ([]taskStatsBucket, error) {
	type row struct {
		Model               string
		TaskCount           int64
		InputTokens         int64
		OutputTokens        int64
		CacheReadTokens     int64
		CacheCreationTokens int64
		CostUSD             float64
	}
	var rows []row
	if err := h.db.Raw(`
		SELECT COALESCE(model, 'unknown') AS model,
			COUNT(*) AS task_count,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(SUM(cost_usd), 0) AS cost_usd
		FROM tasks
		WHERE completed_at IS NOT NULL
			AND completed_at >= ? AND completed_at <= ?
			AND status != ?
		GROUP BY COALESCE(model, 'unknown')
		ORDER BY cost_usd DESC
	`, from, to, models.TaskStatusDeleted).Scan(&rows).Error; err != nil {
		return nil, err
	}
	buckets := make([]taskStatsBucket, 0, len(rows))
	for i := range rows {
		m := rows[i].Model
		buckets = append(buckets, taskStatsBucket{
			Model:               &m,
			TaskCount:           rows[i].TaskCount,
			InputTokens:         rows[i].InputTokens,
			OutputTokens:        rows[i].OutputTokens,
			CacheReadTokens:     rows[i].CacheReadTokens,
			CacheCreationTokens: rows[i].CacheCreationTokens,
			CostUSD:             rows[i].CostUSD,
		})
	}
	return buckets, nil
}

// queryByDayAndProject groups by both UTC day and project, ordered for
// deterministic stacked-bar rendering on the client.
func (h *TaskStatsHandler) queryByDayAndProject(from, to time.Time) ([]taskStatsBucket, error) {
	type row struct {
		Day                 string
		ProjectID           string
		ProjectName         string
		TaskCount           int64
		InputTokens         int64
		OutputTokens        int64
		CacheReadTokens     int64
		CacheCreationTokens int64
		CostUSD             float64
	}
	var rows []row
	if err := h.db.Raw(`
		SELECT to_char(date_trunc('day', t.completed_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
			t.project_id::text AS project_id,
			p.name AS project_name,
			COUNT(*) AS task_count,
			COALESCE(SUM(t.input_tokens), 0) AS input_tokens,
			COALESCE(SUM(t.output_tokens), 0) AS output_tokens,
			COALESCE(SUM(t.cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(t.cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(SUM(t.cost_usd), 0) AS cost_usd
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE t.completed_at IS NOT NULL
			AND t.completed_at >= ? AND t.completed_at <= ?
			AND t.status != ?
		GROUP BY day, t.project_id, p.name
		ORDER BY day ASC, p.name ASC
	`, from, to, models.TaskStatusDeleted).Scan(&rows).Error; err != nil {
		return nil, err
	}
	buckets := make([]taskStatsBucket, 0, len(rows))
	for i := range rows {
		day := rows[i].Day
		pid := rows[i].ProjectID
		name := rows[i].ProjectName
		buckets = append(buckets, taskStatsBucket{
			Day:                 &day,
			Project:             &name,
			ProjectID:           &pid,
			TaskCount:           rows[i].TaskCount,
			InputTokens:         rows[i].InputTokens,
			OutputTokens:        rows[i].OutputTokens,
			CacheReadTokens:     rows[i].CacheReadTokens,
			CacheCreationTokens: rows[i].CacheCreationTokens,
			CostUSD:             rows[i].CostUSD,
		})
	}
	return buckets, nil
}

// queryByDayAndModel groups by both UTC day and model.
func (h *TaskStatsHandler) queryByDayAndModel(from, to time.Time) ([]taskStatsBucket, error) {
	type row struct {
		Day                 string
		Model               string
		TaskCount           int64
		InputTokens         int64
		OutputTokens        int64
		CacheReadTokens     int64
		CacheCreationTokens int64
		CostUSD             float64
	}
	var rows []row
	if err := h.db.Raw(`
		SELECT to_char(date_trunc('day', completed_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
			COALESCE(model, 'unknown') AS model,
			COUNT(*) AS task_count,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(SUM(cost_usd), 0) AS cost_usd
		FROM tasks
		WHERE completed_at IS NOT NULL
			AND completed_at >= ? AND completed_at <= ?
			AND status != ?
		GROUP BY day, COALESCE(model, 'unknown')
		ORDER BY day ASC, model ASC
	`, from, to, models.TaskStatusDeleted).Scan(&rows).Error; err != nil {
		return nil, err
	}
	buckets := make([]taskStatsBucket, 0, len(rows))
	for i := range rows {
		day := rows[i].Day
		m := rows[i].Model
		buckets = append(buckets, taskStatsBucket{
			Day:                 &day,
			Model:               &m,
			TaskCount:           rows[i].TaskCount,
			InputTokens:         rows[i].InputTokens,
			OutputTokens:        rows[i].OutputTokens,
			CacheReadTokens:     rows[i].CacheReadTokens,
			CacheCreationTokens: rows[i].CacheCreationTokens,
			CostUSD:             rows[i].CostUSD,
		})
	}
	return buckets, nil
}

// parseDateRange parses optional from/to query params as YYYY-MM-DD, falling
// back to the supplied defaults. Returns an error for malformed dates or when
// `from` is after `to`.
func parseDateRange(fromStr, toStr string, defFrom, defTo time.Time) (time.Time, time.Time, error) {
	from := defFrom
	to := defTo
	if fromStr != "" {
		t, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'from' date: %w", err)
		}
		from = t
	}
	if toStr != "" {
		t, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'to' date: %w", err)
		}
		to = t
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("'from' must be on or before 'to'")
	}
	return from, to, nil
}
