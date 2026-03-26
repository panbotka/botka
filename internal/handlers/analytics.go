package handlers

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AnalyticsHandler provides endpoints for cost analytics.
type AnalyticsHandler struct {
	db *gorm.DB
}

// NewAnalyticsHandler creates a new AnalyticsHandler.
func NewAnalyticsHandler(db *gorm.DB) *AnalyticsHandler {
	return &AnalyticsHandler{db: db}
}

// RegisterAnalyticsRoutes registers analytics endpoints on the given router group.
func RegisterAnalyticsRoutes(rg *gin.RouterGroup, h *AnalyticsHandler) {
	rg.GET("/analytics/cost", h.CostAnalytics)
}

type modelTokens struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

type costByDate struct {
	Date         string                 `json:"date"`
	CostUSD      float64                `json:"cost_usd"`
	InputTokens  int64                  `json:"input_tokens"`
	OutputTokens int64                  `json:"output_tokens"`
	ByModel      map[string]modelTokens `json:"by_model"`
}

type costByThread struct {
	ThreadID     int64   `json:"thread_id"`
	Title        string  `json:"title"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

type costByProject struct {
	ProjectName  string  `json:"project_name"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

type costByModel struct {
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	MessageCount int64   `json:"message_count"`
}

type costAnalyticsResponse struct {
	TotalCostUSD      float64         `json:"total_cost_usd"`
	TotalInputTokens  int64           `json:"total_input_tokens"`
	TotalOutputTokens int64           `json:"total_output_tokens"`
	ByDate            []costByDate    `json:"by_date"`
	ByModel           []costByModel   `json:"by_model"`
	ByThread          []costByThread  `json:"by_thread"`
	ByProject         []costByProject `json:"by_project"`
}

// CostAnalytics returns aggregated cost data from chat messages and task executions.
func (h *AnalyticsHandler) CostAnalytics(c *gin.Context) {
	daysStr := c.DefaultQuery("days", "30")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 || days > 365 {
		days = 30
	}

	since := time.Now().AddDate(0, 0, -days)

	// Total cost: sum messages + task executions
	var msgTotal struct{ Sum *float64 }
	h.db.Raw(`SELECT SUM(cost_usd) as sum FROM messages WHERE cost_usd IS NOT NULL AND created_at >= ?`, since).Scan(&msgTotal)

	var execTotal struct{ Sum *float64 }
	h.db.Raw(`SELECT SUM(cost_usd) as sum FROM task_executions WHERE cost_usd IS NOT NULL AND started_at >= ?`, since).Scan(&execTotal)

	totalCost := 0.0
	if msgTotal.Sum != nil {
		totalCost += *msgTotal.Sum
	}
	if execTotal.Sum != nil {
		totalCost += *execTotal.Sum
	}

	// By model: JOIN messages with threads, GROUP BY model
	var byModelRows []struct {
		Model        string  `gorm:"column:model"`
		InputTokens  int64   `gorm:"column:input_tokens"`
		OutputTokens int64   `gorm:"column:output_tokens"`
		CostUSD      float64 `gorm:"column:cost_usd"`
		MessageCount int64   `gorm:"column:message_count"`
	}
	h.db.Raw(`
		SELECT COALESCE(t.model, 'unknown') AS model,
			SUM(COALESCE(m.prompt_tokens, 0)) AS input_tokens,
			SUM(COALESCE(m.completion_tokens, 0)) AS output_tokens,
			SUM(COALESCE(m.cost_usd, 0)) AS cost_usd,
			COUNT(*) AS message_count
		FROM messages m
		JOIN threads t ON t.id = m.thread_id
		WHERE m.cost_usd IS NOT NULL AND m.created_at >= ?
		GROUP BY COALESCE(t.model, 'unknown')
		ORDER BY input_tokens DESC
	`, since).Scan(&byModelRows)

	byModel := make([]costByModel, 0, len(byModelRows))
	var totalInputTokens, totalOutputTokens int64
	for _, r := range byModelRows {
		byModel = append(byModel, costByModel{
			Model:        r.Model,
			InputTokens:  r.InputTokens,
			OutputTokens: r.OutputTokens,
			CostUSD:      r.CostUSD,
			MessageCount: r.MessageCount,
		})
		totalInputTokens += r.InputTokens
		totalOutputTokens += r.OutputTokens
	}

	// Cost by date: combine both sources
	var byDateBase []struct {
		Date         string  `gorm:"column:date"`
		CostUSD      float64 `gorm:"column:cost_usd"`
		InputTokens  int64   `gorm:"column:input_tokens"`
		OutputTokens int64   `gorm:"column:output_tokens"`
	}
	h.db.Raw(`
		SELECT d.date,
			COALESCE(m.cost, 0) + COALESCE(e.cost, 0) AS cost_usd,
			COALESCE(m.input_tokens, 0) AS input_tokens,
			COALESCE(m.output_tokens, 0) AS output_tokens
		FROM (
			SELECT to_char(generate_series(?::date, CURRENT_DATE, '1 day'), 'YYYY-MM-DD') AS date
		) d
		LEFT JOIN (
			SELECT to_char(created_at, 'YYYY-MM-DD') AS date,
				SUM(cost_usd) AS cost,
				SUM(COALESCE(prompt_tokens, 0)) AS input_tokens,
				SUM(COALESCE(completion_tokens, 0)) AS output_tokens
			FROM messages
			WHERE cost_usd IS NOT NULL AND created_at >= ?
			GROUP BY to_char(created_at, 'YYYY-MM-DD')
		) m ON m.date = d.date
		LEFT JOIN (
			SELECT to_char(started_at, 'YYYY-MM-DD') AS date,
				SUM(cost_usd) AS cost
			FROM task_executions
			WHERE cost_usd IS NOT NULL AND started_at >= ?
			GROUP BY to_char(started_at, 'YYYY-MM-DD')
		) e ON e.date = d.date
		ORDER BY d.date
	`, since, since, since).Scan(&byDateBase)

	// By date + model breakdown
	var dateModelRows []struct {
		Date         string `gorm:"column:date"`
		Model        string `gorm:"column:model"`
		InputTokens  int64  `gorm:"column:input_tokens"`
		OutputTokens int64  `gorm:"column:output_tokens"`
	}
	h.db.Raw(`
		SELECT to_char(m.created_at, 'YYYY-MM-DD') AS date,
			COALESCE(t.model, 'unknown') AS model,
			SUM(COALESCE(m.prompt_tokens, 0)) AS input_tokens,
			SUM(COALESCE(m.completion_tokens, 0)) AS output_tokens
		FROM messages m
		JOIN threads t ON t.id = m.thread_id
		WHERE m.cost_usd IS NOT NULL AND m.created_at >= ?
		GROUP BY to_char(m.created_at, 'YYYY-MM-DD'), COALESCE(t.model, 'unknown')
	`, since).Scan(&dateModelRows)

	// Build date->model map
	dateModelMap := make(map[string]map[string]modelTokens)
	for _, r := range dateModelRows {
		if dateModelMap[r.Date] == nil {
			dateModelMap[r.Date] = make(map[string]modelTokens)
		}
		dateModelMap[r.Date][r.Model] = modelTokens{
			Input:  r.InputTokens,
			Output: r.OutputTokens,
		}
	}

	byDate := make([]costByDate, 0, len(byDateBase))
	for _, d := range byDateBase {
		dm := dateModelMap[d.Date]
		if dm == nil {
			dm = make(map[string]modelTokens)
		}
		byDate = append(byDate, costByDate{
			Date:         d.Date,
			CostUSD:      d.CostUSD,
			InputTokens:  d.InputTokens,
			OutputTokens: d.OutputTokens,
			ByModel:      dm,
		})
	}

	// Top 10 threads by cost (with tokens)
	var byThread []costByThread
	h.db.Raw(`
		SELECT m.thread_id, t.title, SUM(m.cost_usd) AS cost_usd,
			SUM(COALESCE(m.prompt_tokens, 0)) AS input_tokens,
			SUM(COALESCE(m.completion_tokens, 0)) AS output_tokens
		FROM messages m
		JOIN threads t ON t.id = m.thread_id
		WHERE m.cost_usd IS NOT NULL AND m.created_at >= ?
		GROUP BY m.thread_id, t.title
		ORDER BY cost_usd DESC
		LIMIT 10
	`, since).Scan(&byThread)

	if byThread == nil {
		byThread = []costByThread{}
	}

	// Top 10 projects by cost (with tokens)
	var byProject []costByProject
	h.db.Raw(`
		SELECT project_name, SUM(cost) AS cost_usd,
			SUM(input_tokens) AS input_tokens,
			SUM(output_tokens) AS output_tokens
		FROM (
			SELECT p.name AS project_name, SUM(te.cost_usd) AS cost,
				0 AS input_tokens, 0 AS output_tokens
			FROM task_executions te
			JOIN tasks tk ON tk.id = te.task_id
			JOIN projects p ON p.id = tk.project_id
			WHERE te.cost_usd IS NOT NULL AND te.started_at >= ?
			GROUP BY p.name
			UNION ALL
			SELECT p.name AS project_name, SUM(m.cost_usd) AS cost,
				SUM(COALESCE(m.prompt_tokens, 0)) AS input_tokens,
				SUM(COALESCE(m.completion_tokens, 0)) AS output_tokens
			FROM messages m
			JOIN threads t ON t.id = m.thread_id
			JOIN projects p ON p.id = t.project_id
			WHERE m.cost_usd IS NOT NULL AND m.created_at >= ? AND t.project_id IS NOT NULL
			GROUP BY p.name
		) combined
		GROUP BY project_name
		ORDER BY cost_usd DESC
		LIMIT 10
	`, since, since).Scan(&byProject)

	if byProject == nil {
		byProject = []costByProject{}
	}

	respondOK(c, costAnalyticsResponse{
		TotalCostUSD:      totalCost,
		TotalInputTokens:  totalInputTokens,
		TotalOutputTokens: totalOutputTokens,
		ByDate:            byDate,
		ByModel:           byModel,
		ByThread:          byThread,
		ByProject:         byProject,
	})
}
