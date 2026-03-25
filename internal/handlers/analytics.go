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

type costByDate struct {
	Date         string  `json:"date"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

type costByThread struct {
	ThreadID int64   `json:"thread_id"`
	Title    string  `json:"title"`
	CostUSD  float64 `json:"cost_usd"`
}

type costByProject struct {
	ProjectName string  `json:"project_name"`
	CostUSD     float64 `json:"cost_usd"`
}

type costAnalyticsResponse struct {
	TotalCostUSD float64         `json:"total_cost_usd"`
	ByDate       []costByDate    `json:"by_date"`
	ByThread     []costByThread  `json:"by_thread"`
	ByProject    []costByProject `json:"by_project"`
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

	// Cost by date: combine both sources
	var byDate []costByDate
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
	`, since, since, since).Scan(&byDate)

	if byDate == nil {
		byDate = []costByDate{}
	}

	// Top 10 threads by cost
	var byThread []costByThread
	h.db.Raw(`
		SELECT m.thread_id, t.title, SUM(m.cost_usd) AS cost_usd
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

	// Top 10 projects by cost (tasks + chat)
	var byProject []costByProject
	h.db.Raw(`
		SELECT project_name, SUM(cost) AS cost_usd FROM (
			SELECT p.name AS project_name, SUM(te.cost_usd) AS cost
			FROM task_executions te
			JOIN tasks tk ON tk.id = te.task_id
			JOIN projects p ON p.id = tk.project_id
			WHERE te.cost_usd IS NOT NULL AND te.started_at >= ?
			GROUP BY p.name
			UNION ALL
			SELECT p.name AS project_name, SUM(m.cost_usd) AS cost
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
		TotalCostUSD: totalCost,
		ByDate:       byDate,
		ByThread:     byThread,
		ByProject:    byProject,
	})
}
