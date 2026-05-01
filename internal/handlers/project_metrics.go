package handlers

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"botka/internal/models"
)

// minTasksForMetrics is the threshold below which the metrics endpoint
// reports `enough_data: false` and the UI shows a placeholder instead of charts.
const minTasksForMetrics = 5

// metricsCacheTTL is how long an aggregated metrics payload is reused
// before a fresh DB query is run.
const metricsCacheTTL = 60 * time.Second

// metricsLookbackDays is the rolling window used by the time-bucketed metrics
// (success rate, average duration, tasks-per-day, top failures).
const metricsLookbackDays = 30

// metricsLastNDurations is the number of recent task durations included for
// the sparkline.
const metricsLastNDurations = 10

// projectMetrics is the JSON payload returned by GET /projects/:id/metrics.
type projectMetrics struct {
	EnoughData       bool             `json:"enough_data"`
	Total            int64            `json:"total"`
	ByStatus         taskCounts       `json:"by_status"`
	SuccessRate30d   *float64         `json:"success_rate_30d"`
	AvgDurationMs30d *float64         `json:"avg_duration_ms_30d"`
	TasksPerDay      []metricsDay     `json:"tasks_per_day"`
	TopFailures      []metricsFailure `json:"top_failures"`
	LastDurations    []metricsDur     `json:"last_durations"`
	GeneratedAt      time.Time        `json:"generated_at"`
}

// metricsDay is one entry in the tasks-per-day series.
// Date is an UTC YYYY-MM-DD string; clients convert to local time for display.
type metricsDay struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// metricsFailure is one entry in the top-failures list.
type metricsFailure struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

// metricsDur is one entry in the recent-durations sparkline.
type metricsDur struct {
	TaskID      string    `json:"task_id"`
	DurationMs  int64     `json:"duration_ms"`
	CompletedAt time.Time `json:"completed_at"`
}

// metricsCacheEntry holds a cached metrics payload and the time it was generated.
type metricsCacheEntry struct {
	payload   projectMetrics
	expiresAt time.Time
}

// metricsCache is a per-project, time-bounded in-memory cache for the metrics
// endpoint. The metrics queries scan the entire tasks table for the project,
// so caching the result for a short window avoids repeated heavy aggregation
// when several browser tabs poll the dashboard.
type metricsCache struct {
	mu      sync.Mutex
	entries map[uuid.UUID]metricsCacheEntry
}

// newMetricsCache constructs an empty cache.
func newMetricsCache() *metricsCache {
	return &metricsCache{entries: make(map[uuid.UUID]metricsCacheEntry)}
}

// get returns a cached payload if still fresh.
func (c *metricsCache) get(id uuid.UUID, now time.Time) (projectMetrics, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	if !ok || now.After(e.expiresAt) {
		return projectMetrics{}, false
	}
	return e.payload, true
}

// put stores a freshly computed payload in the cache.
func (c *metricsCache) put(id uuid.UUID, payload projectMetrics, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = metricsCacheEntry{
		payload:   payload,
		expiresAt: now.Add(metricsCacheTTL),
	}
}

// GetMetrics returns aggregated task metrics for a project's dashboard.
// The result is cached in-memory for metricsCacheTTL per project.
func (h *ProjectHandler) GetMetrics(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid project id")
		return
	}

	if _, ok := h.findProject(c, id); !ok {
		return
	}

	now := time.Now().UTC()
	if cached, ok := h.metricsCache.get(id, now); ok {
		respondOK(c, cached)
		return
	}

	payload, err := h.computeProjectMetrics(id, now)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to compute metrics")
		return
	}

	h.metricsCache.put(id, payload, now)
	respondOK(c, payload)
}

// computeProjectMetrics runs all metric queries for a project and assembles
// the response payload. Each metric is a single SQL aggregation (no N+1).
func (h *ProjectHandler) computeProjectMetrics(projectID uuid.UUID, now time.Time) (projectMetrics, error) {
	counts, err := h.countTasks(projectID)
	if err != nil {
		return projectMetrics{}, err
	}
	total := counts.Pending + counts.Queued + counts.Running +
		counts.Done + counts.Failed + counts.NeedsReview + counts.Cancelled

	out := projectMetrics{
		Total:       total,
		ByStatus:    counts,
		EnoughData:  total >= minTasksForMetrics,
		GeneratedAt: now,
		// Initialize empty slices so the JSON shape is stable.
		TasksPerDay:   []metricsDay{},
		TopFailures:   []metricsFailure{},
		LastDurations: []metricsDur{},
	}

	if !out.EnoughData {
		return out, nil
	}

	since := now.AddDate(0, 0, -metricsLookbackDays)

	if err := h.fillSuccessAndDuration(&out, projectID, since); err != nil {
		return projectMetrics{}, err
	}
	if err := h.fillTasksPerDay(&out, projectID, since, now); err != nil {
		return projectMetrics{}, err
	}
	if err := h.fillTopFailures(&out, projectID, since); err != nil {
		return projectMetrics{}, err
	}
	if err := h.fillLastDurations(&out, projectID); err != nil {
		return projectMetrics{}, err
	}

	return out, nil
}

// fillSuccessAndDuration computes the 30-day success rate and average duration.
// Success rate uses the tasks table (cancelled/deleted excluded by status filter).
// Average duration uses task_executions.duration_ms, which the runner sets from
// the Claude Code result event.
func (h *ProjectHandler) fillSuccessAndDuration(out *projectMetrics, projectID uuid.UUID, since time.Time) error {
	var sr struct {
		Done   int64
		Failed int64
	}
	err := h.db.Raw(`
		SELECT
			COUNT(*) FILTER (WHERE status = ?) AS done,
			COUNT(*) FILTER (WHERE status = ?) AS failed
		FROM tasks
		WHERE project_id = ?
		  AND completed_at IS NOT NULL
		  AND completed_at >= ?
	`, models.TaskStatusDone, models.TaskStatusFailed, projectID, since).Scan(&sr).Error
	if err != nil {
		return err
	}
	if completed := sr.Done + sr.Failed; completed > 0 {
		rate := float64(sr.Done) / float64(completed)
		out.SuccessRate30d = &rate
	}

	var avg struct {
		Avg *float64
	}
	err = h.db.Raw(`
		SELECT AVG(te.duration_ms) AS avg
		FROM task_executions te
		JOIN tasks t ON t.id = te.task_id
		WHERE t.project_id = ?
		  AND te.duration_ms IS NOT NULL
		  AND te.finished_at IS NOT NULL
		  AND te.finished_at >= ?
	`, projectID, since).Scan(&avg).Error
	if err != nil {
		return err
	}
	out.AvgDurationMs30d = avg.Avg
	return nil
}

// fillTasksPerDay produces a 30-day series of task counts grouped by UTC date
// of creation. Days with zero tasks are included so the bar chart has a stable
// width on the client.
func (h *ProjectHandler) fillTasksPerDay(out *projectMetrics, projectID uuid.UUID, since, now time.Time) error {
	var rows []struct {
		Date  string
		Count int64
	}
	err := h.db.Raw(`
		SELECT to_char(date_trunc('day', created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS date,
		       COUNT(*) AS count
		FROM tasks
		WHERE project_id = ?
		  AND created_at >= ?
		GROUP BY date
		ORDER BY date
	`, projectID, since).Scan(&rows).Error
	if err != nil {
		return err
	}
	byDate := make(map[string]int64, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r.Count
	}

	// Build a contiguous series from `since` (rounded to UTC day) through `now`.
	start := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	series := make([]metricsDay, 0, metricsLookbackDays+1)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		series = append(series, metricsDay{Date: key, Count: byDate[key]})
	}
	out.TasksPerDay = series
	return nil
}

// fillTopFailures returns the 5 most common failure reasons for the project
// in the last 30 days. Reasons are extracted from `failure_summary` (first
// sentence) when available, falling back to `failure_reason`. Tasks without
// either are grouped under "Unknown error".
func (h *ProjectHandler) fillTopFailures(out *projectMetrics, projectID uuid.UUID, since time.Time) error {
	var rows []struct {
		FailureSummary *string
		FailureReason  *string
	}
	err := h.db.Raw(`
		SELECT failure_summary, failure_reason
		FROM tasks
		WHERE project_id = ?
		  AND status = ?
		  AND completed_at IS NOT NULL
		  AND completed_at >= ?
	`, projectID, models.TaskStatusFailed, since).Scan(&rows).Error
	if err != nil {
		return err
	}

	// Group in Go: SQL grouping by a "first sentence" projection is awkward
	// across summary/reason fallback, and the per-project failure volume is
	// small enough that a Go map is cheap.
	bucket := make(map[string]int64)
	order := make([]string, 0)
	for _, r := range rows {
		reason := failureBucketKey(r.FailureSummary, r.FailureReason)
		if _, seen := bucket[reason]; !seen {
			order = append(order, reason)
		}
		bucket[reason]++
	}

	// Sort by count desc, then by first-seen order for stable output.
	type kv struct {
		reason string
		count  int64
		idx    int
	}
	pairs := make([]kv, 0, len(bucket))
	for i, r := range order {
		pairs = append(pairs, kv{reason: r, count: bucket[r], idx: i})
	}
	// Insertion-style sort: stable, simple, fine for ≤ a few hundred rows.
	for i := 1; i < len(pairs); i++ {
		for j := i; j > 0; j-- {
			if pairs[j].count > pairs[j-1].count ||
				(pairs[j].count == pairs[j-1].count && pairs[j].idx < pairs[j-1].idx) {
				pairs[j-1], pairs[j] = pairs[j], pairs[j-1]
				continue
			}
			break
		}
	}

	limit := 5
	if len(pairs) < limit {
		limit = len(pairs)
	}
	failures := make([]metricsFailure, 0, limit)
	for i := 0; i < limit; i++ {
		failures = append(failures, metricsFailure{Reason: pairs[i].reason, Count: pairs[i].count})
	}
	out.TopFailures = failures
	return nil
}

// failureBucketKey selects the human-readable bucket label for a failed task.
// Prefers the first sentence of failure_summary; falls back to failure_reason
// (truncated). Empty inputs map to "Unknown error".
func failureBucketKey(summary, reason *string) string {
	if summary != nil {
		if s := firstSentence(*summary); s != "" {
			return s
		}
	}
	if reason != nil {
		// Reason can be a multi-line stderr blob; keep just the leading line
		// so visually distinct errors don't collapse into one giant string.
		first := strings.TrimSpace(strings.SplitN(*reason, "\n", 2)[0])
		if len(first) > 200 {
			first = first[:200] + "…"
		}
		if first != "" {
			return first
		}
	}
	return "Unknown error"
}

// firstSentence returns the first sentence of s, trimmed and limited to 200
// characters. A "sentence" ends at the first `.`, `!`, `?`, or newline.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	end := len(s)
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' || r == '\n' {
			end = i
			break
		}
	}
	out := strings.TrimSpace(s[:end])
	if len(out) > 200 {
		out = out[:200] + "…"
	}
	return out
}

// fillLastDurations returns the most recent N task durations (any status) so
// the dashboard can plot them as a sparkline to spot regressions.
func (h *ProjectHandler) fillLastDurations(out *projectMetrics, projectID uuid.UUID) error {
	var rows []struct {
		TaskID     uuid.UUID
		DurationMs int64
		FinishedAt time.Time
	}
	err := h.db.Raw(`
		SELECT te.task_id AS task_id,
		       te.duration_ms AS duration_ms,
		       te.finished_at AS finished_at
		FROM task_executions te
		JOIN tasks t ON t.id = te.task_id
		WHERE t.project_id = ?
		  AND te.duration_ms IS NOT NULL
		  AND te.finished_at IS NOT NULL
		ORDER BY te.finished_at DESC
		LIMIT ?
	`, projectID, metricsLastNDurations).Scan(&rows).Error
	if err != nil {
		return err
	}

	// Reverse so the oldest is first (left-to-right time axis on the sparkline).
	durations := make([]metricsDur, len(rows))
	for i, r := range rows {
		durations[len(rows)-1-i] = metricsDur{
			TaskID:      r.TaskID.String(),
			DurationMs:  r.DurationMs,
			CompletedAt: r.FinishedAt,
		}
	}
	out.LastDurations = durations
	return nil
}
