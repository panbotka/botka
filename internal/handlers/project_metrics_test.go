package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/models"
)

// createTestExecution inserts a task_execution row directly. Used so tests
// can populate duration_ms and finished_at without going through the runner.
func createTestExecution(t *testing.T, db *gorm.DB, taskID uuid.UUID, durationMs int64, finishedAt time.Time) {
	t.Helper()
	exec := models.TaskExecution{
		TaskID:     taskID,
		Attempt:    1,
		StartedAt:  finishedAt.Add(-time.Duration(durationMs) * time.Millisecond),
		FinishedAt: &finishedAt,
		DurationMs: &durationMs,
	}
	if err := db.Create(&exec).Error; err != nil {
		t.Fatalf("create test execution: %v", err)
	}
}

// updateTaskCompletion sets status, completed_at, failure_summary, and
// failure_reason on a task without going through the handler API.
func updateTaskCompletion(t *testing.T, db *gorm.DB, taskID uuid.UUID, status models.TaskStatus, completedAt time.Time, summary, reason *string) {
	t.Helper()
	updates := map[string]interface{}{
		"status":          status,
		"completed_at":    completedAt,
		"failure_summary": summary,
		"failure_reason":  reason,
	}
	if err := db.Model(&models.Task{}).Where("id = ?", taskID).Updates(updates).Error; err != nil {
		t.Fatalf("update task completion: %v", err)
	}
}

func TestProject_Metrics_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/metrics", uuid.New()), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestProject_Metrics_NotEnoughData(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	p := createTestProject(t, db)

	// 4 tasks — below the 5-task threshold.
	for i := 0; i < 4; i++ {
		createTestTask(t, db, p.ID, models.TaskStatusDone)
	}

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/metrics", p.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["enough_data"].(bool) != false {
		t.Errorf("expected enough_data=false, got %v", data["enough_data"])
	}
	if int(data["total"].(float64)) != 4 {
		t.Errorf("expected total=4, got %v", data["total"])
	}
	// Series should be empty when not enough data, not nil.
	if days := data["tasks_per_day"].([]interface{}); len(days) != 0 {
		t.Errorf("expected empty tasks_per_day, got %v", days)
	}
	if dur := data["last_durations"].([]interface{}); len(dur) != 0 {
		t.Errorf("expected empty last_durations, got %v", dur)
	}
}

func TestProject_Metrics_FullPayload(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	p := createTestProject(t, db)

	now := time.Now().UTC()

	// 3 done, 2 failed, 1 cancelled = 6 tasks total (>=5 → enough_data).
	for i := 0; i < 3; i++ {
		task := createTestTask(t, db, p.ID, models.TaskStatusPending)
		updateTaskCompletion(t, db, task.ID, models.TaskStatusDone, now.Add(-time.Duration(i)*time.Hour), nil, nil)
		createTestExecution(t, db, task.ID, int64(60_000+i*1000), now.Add(-time.Duration(i)*time.Hour))
	}

	failOne := createTestTask(t, db, p.ID, models.TaskStatusPending)
	summary1 := "Tests failed in CI. The build broke on Go 1.25."
	updateTaskCompletion(t, db, failOne.ID, models.TaskStatusFailed, now.Add(-1*time.Hour), &summary1, nil)
	createTestExecution(t, db, failOne.ID, 90_000, now.Add(-1*time.Hour))

	failTwo := createTestTask(t, db, p.ID, models.TaskStatusPending)
	// Same first sentence as failOne — should be grouped.
	summary2 := "Tests failed in CI. Different second sentence."
	updateTaskCompletion(t, db, failTwo.ID, models.TaskStatusFailed, now.Add(-2*time.Hour), &summary2, nil)
	createTestExecution(t, db, failTwo.ID, 80_000, now.Add(-2*time.Hour))

	createTestTask(t, db, p.ID, models.TaskStatusCancelled)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/metrics", p.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data := resp["data"].(map[string]interface{})

	if data["enough_data"].(bool) != true {
		t.Errorf("expected enough_data=true, got %v", data["enough_data"])
	}
	if int(data["total"].(float64)) != 6 {
		t.Errorf("expected total=6, got %v", data["total"])
	}

	// Success rate over the 30-day window: 3 done, 2 failed → 0.6.
	rate := data["success_rate_30d"].(float64)
	if rate < 0.59 || rate > 0.61 {
		t.Errorf("expected success_rate_30d ~0.6, got %v", rate)
	}

	// Average duration over executions: (60000+61000+62000+90000+80000)/5 = 70_600.
	avg := data["avg_duration_ms_30d"].(float64)
	if avg < 70_500 || avg > 70_700 {
		t.Errorf("expected avg_duration_ms_30d ~70600, got %v", avg)
	}

	// tasks_per_day: 31 entries (since..now inclusive on UTC day boundaries).
	days := data["tasks_per_day"].([]interface{})
	if len(days) < 30 || len(days) > 32 {
		t.Errorf("expected ~31 days, got %d", len(days))
	}
	// Sum should equal total tasks created (6).
	var sum int64
	for _, d := range days {
		sum += int64(d.(map[string]interface{})["count"].(float64))
	}
	if sum != 6 {
		t.Errorf("expected day-counts to sum to 6, got %d", sum)
	}

	// Top failures: both failed tasks share the first sentence "Tests failed in CI" → 1 entry, count 2.
	failures := data["top_failures"].([]interface{})
	if len(failures) != 1 {
		t.Errorf("expected 1 failure bucket, got %d", len(failures))
	}
	if len(failures) > 0 {
		f := failures[0].(map[string]interface{})
		if f["reason"].(string) != "Tests failed in CI" {
			t.Errorf("expected reason 'Tests failed in CI', got %v", f["reason"])
		}
		if int(f["count"].(float64)) != 2 {
			t.Errorf("expected count=2, got %v", f["count"])
		}
	}

	// Last durations: 5 executions, oldest first.
	dur := data["last_durations"].([]interface{})
	if len(dur) != 5 {
		t.Errorf("expected 5 durations, got %d", len(dur))
	}
	// Oldest first → ascending CompletedAt timestamps.
	for i := 1; i < len(dur); i++ {
		prev := dur[i-1].(map[string]interface{})["completed_at"].(string)
		curr := dur[i].(map[string]interface{})["completed_at"].(string)
		if prev > curr {
			t.Errorf("expected durations sorted oldest-first, got %s before %s", prev, curr)
		}
	}
}

func TestProject_Metrics_FailureFallbackToReason(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	p := createTestProject(t, db)

	// 5 tasks total to clear the threshold.
	for i := 0; i < 4; i++ {
		createTestTask(t, db, p.ID, models.TaskStatusDone)
	}
	failed := createTestTask(t, db, p.ID, models.TaskStatusPending)

	reason := "exit status 1\nstderr: build failed"
	updateTaskCompletion(t, db, failed.ID, models.TaskStatusFailed, time.Now().UTC(), nil, &reason)

	r := projectRouter(db, noopScan, noopSync)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/metrics", p.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	failures := data["top_failures"].([]interface{})
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	got := failures[0].(map[string]interface{})["reason"].(string)
	if got != "exit status 1" {
		t.Errorf("expected reason 'exit status 1' (first line of failure_reason), got %q", got)
	}
}

func TestProject_Metrics_Caching(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	p := createTestProject(t, db)

	// Need >=5 to actually run the queries.
	for i := 0; i < 5; i++ {
		createTestTask(t, db, p.ID, models.TaskStatusDone)
	}

	r := projectRouter(db, noopScan, noopSync)
	url := fmt.Sprintf("/api/v1/projects/%s/metrics", p.ID)

	w1 := doRequest(r, http.MethodGet, url, "")
	if w1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w1.Code)
	}

	// Insert another task; cached payload should still be served.
	createTestTask(t, db, p.ID, models.TaskStatusDone)

	w2 := doRequest(r, http.MethodGet, url, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d", w2.Code)
	}

	var r1, r2 map[string]interface{}
	json.Unmarshal(w1.Body.Bytes(), &r1)
	json.Unmarshal(w2.Body.Bytes(), &r2)
	t1 := int(r1["data"].(map[string]interface{})["total"].(float64))
	t2 := int(r2["data"].(map[string]interface{})["total"].(float64))
	if t1 != t2 {
		t.Errorf("expected cached response to match (got %d vs %d) — cache miss", t1, t2)
	}
	if t1 != 5 {
		t.Errorf("expected first response total=5, got %d", t1)
	}
}

func TestFirstSentence(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"Hello world", "Hello world"},
		{"Hello. World.", "Hello"},
		{"Hello! World", "Hello"},
		{"Hello? Anyone there?", "Hello"},
		{"Line one\nLine two", "Line one"},
		{"  spaced.  next  ", "spaced"},
	}
	for _, c := range cases {
		if got := firstSentence(c.in); got != c.want {
			t.Errorf("firstSentence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFailureBucketKey(t *testing.T) {
	str := func(s string) *string { return &s }

	if got := failureBucketKey(str("Build failed. More details."), nil); got != "Build failed" {
		t.Errorf("summary first sentence: got %q, want 'Build failed'", got)
	}
	if got := failureBucketKey(nil, str("exit code 1\nstack trace")); got != "exit code 1" {
		t.Errorf("reason fallback: got %q, want 'exit code 1'", got)
	}
	if got := failureBucketKey(nil, nil); got != "Unknown error" {
		t.Errorf("nil fallback: got %q, want 'Unknown error'", got)
	}
	// Empty summary should fall through to reason.
	empty := ""
	if got := failureBucketKey(&empty, str("real reason")); got != "real reason" {
		t.Errorf("empty summary fallthrough: got %q, want 'real reason'", got)
	}
}
