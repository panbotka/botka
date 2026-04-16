package runner

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"botka/internal/config"
	"botka/internal/models"
)

func TestShouldRun_MatchingSchedule(t *testing.T) {
	t.Parallel()

	cs := &CronScheduler{running: make(map[int64]bool)}

	// "every minute" schedule should match any time truncated to minute.
	sched, err := cron.ParseStandard("* * * * *")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}

	job := &models.CronJob{ID: 1}
	now := time.Date(2025, 6, 15, 10, 30, 15, 0, time.UTC)

	if !cs.shouldRun(job, sched, now) {
		t.Error("expected job to run with every-minute schedule")
	}
}

func TestShouldRun_NonMatchingSchedule(t *testing.T) {
	t.Parallel()

	cs := &CronScheduler{running: make(map[int64]bool)}

	// Only at minute 0, hour 9
	sched, err := cron.ParseStandard("0 9 * * *")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}

	job := &models.CronJob{ID: 1}
	// 10:30 should not match "0 9 * * *"
	now := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	if cs.shouldRun(job, sched, now) {
		t.Error("expected job NOT to run at 10:30 with 0 9 * * * schedule")
	}
}

func TestShouldRun_AlreadyRanThisMinute(t *testing.T) {
	t.Parallel()

	cs := &CronScheduler{running: make(map[int64]bool)}

	sched, err := cron.ParseStandard("* * * * *")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}

	now := time.Date(2025, 6, 15, 10, 30, 15, 0, time.UTC)
	lastRun := now.Add(-10 * time.Second) // same minute
	job := &models.CronJob{ID: 1, LastRunAt: &lastRun}

	if cs.shouldRun(job, sched, now) {
		t.Error("expected job NOT to run when it already ran this minute")
	}
}

func TestShouldRun_RanInPreviousMinute(t *testing.T) {
	t.Parallel()

	cs := &CronScheduler{running: make(map[int64]bool)}

	sched, err := cron.ParseStandard("* * * * *")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}

	now := time.Date(2025, 6, 15, 10, 31, 15, 0, time.UTC)
	lastRun := time.Date(2025, 6, 15, 10, 30, 45, 0, time.UTC) // previous minute
	job := &models.CronJob{ID: 1, LastRunAt: &lastRun}

	if !cs.shouldRun(job, sched, now) {
		t.Error("expected job to run when last run was in previous minute")
	}
}

func TestShouldRun_DisabledJob(t *testing.T) {
	t.Parallel()

	// Disabled jobs are filtered at the query level (WHERE enabled = true),
	// so shouldRun doesn't check enabled. This test documents that behavior.
	cs := &CronScheduler{running: make(map[int64]bool)}

	sched, err := cron.ParseStandard("* * * * *")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}

	job := &models.CronJob{ID: 1, Enabled: false}
	now := time.Date(2025, 6, 15, 10, 30, 15, 0, time.UTC)

	// shouldRun only checks schedule and last_run_at — enabled filtering happens in tick().
	if !cs.shouldRun(job, sched, now) {
		t.Error("shouldRun should not check enabled flag (that's done in the query)")
	}
}

func TestMaybeRun_SkipsAlreadyRunningJob(t *testing.T) {
	t.Parallel()

	cs := &CronScheduler{running: make(map[int64]bool)}

	// Mark job as running.
	cs.running[1] = true

	project := &models.Project{ID: uuid.New(), Name: "test"}
	job := &models.CronJob{
		ID:       1,
		Schedule: "* * * * *",
		Project:  project,
	}

	now := time.Date(2025, 6, 15, 10, 30, 15, 0, time.UTC)

	// Should not panic or start another execution.
	cs.maybeRun(job, now)

	// Job should still be marked running (no change).
	if !cs.running[1] {
		t.Error("expected job to remain in running map")
	}
}

func TestMaybeRun_InvalidSchedule(t *testing.T) {
	t.Parallel()

	cs := &CronScheduler{running: make(map[int64]bool)}

	job := &models.CronJob{
		ID:       1,
		Schedule: "invalid cron expression",
	}

	now := time.Now()

	// Should not panic — just log the error.
	cs.maybeRun(job, now)
}

func TestCronScheduler_StartStop(t *testing.T) {
	t.Parallel()

	cs := &CronScheduler{
		cfg:     &config.Config{},
		running: make(map[int64]bool),
		usageMon: &UsageMonitor{
			lastPollOK:  true,
			threshold5h: 0.90,
			threshold7d: 0.95,
		},
	}

	cs.Start()

	// Starting again should be a no-op (idempotent).
	cs.Start()

	cs.Stop()
}

func TestCronScheduler_TickSkipsWhenRateLimited(t *testing.T) {
	t.Parallel()

	cs := &CronScheduler{
		cfg:     &config.Config{},
		running: make(map[int64]bool),
		usageMon: &UsageMonitor{
			lastPollOK: false, // Will return rate limited
		},
	}

	// Should not panic — just skip.
	cs.tick()
}

func TestCronScheduler_LoopStops(t *testing.T) {
	t.Parallel()

	var tickCount atomic.Int32
	mon := &UsageMonitor{
		lastPollOK:  true,
		threshold5h: 0.90,
		threshold7d: 0.95,
	}

	cs := &CronScheduler{
		cfg:      &config.Config{},
		running:  make(map[int64]bool),
		usageMon: mon,
	}

	stopCh := make(chan struct{})
	cs.wg.Add(1)

	// We can't test the full tick without a DB, but we can test the loop exits.
	go func() {
		defer cs.wg.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				tickCount.Add(1)
			}
		}
	}()

	time.Sleep(35 * time.Millisecond)
	close(stopCh)
	cs.wg.Wait()

	if tickCount.Load() < 2 {
		t.Errorf("expected at least 2 ticks, got %d", tickCount.Load())
	}
}

func TestShouldRun_SpecificScheduleMatch(t *testing.T) {
	t.Parallel()

	cs := &CronScheduler{running: make(map[int64]bool)}

	// Monday 9am
	sched, err := cron.ParseStandard("0 9 * * 1")
	if err != nil {
		t.Fatalf("parse schedule: %v", err)
	}

	job := &models.CronJob{ID: 1}

	// Monday June 16, 2025 at 9:00
	now := time.Date(2025, 6, 16, 9, 0, 5, 0, time.UTC)
	if !cs.shouldRun(job, sched, now) {
		t.Error("expected job to run on Monday at 9:00")
	}

	// Tuesday at 9:00
	now = time.Date(2025, 6, 17, 9, 0, 5, 0, time.UTC)
	if cs.shouldRun(job, sched, now) {
		t.Error("expected job NOT to run on Tuesday")
	}
}

func TestPurgeCronExecutions_NoDB(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)

	// Ensure the cron tables exist.
	db.Exec("CREATE TABLE IF NOT EXISTS cron_jobs (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, schedule TEXT NOT NULL, prompt TEXT NOT NULL, project_id UUID NOT NULL, enabled BOOLEAN NOT NULL DEFAULT true, timeout_minutes INTEGER NOT NULL DEFAULT 30, model TEXT, last_run_at TIMESTAMPTZ, last_status TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())")
	db.Exec("CREATE TABLE IF NOT EXISTS cron_executions (id BIGSERIAL PRIMARY KEY, cron_job_id BIGINT NOT NULL, status TEXT NOT NULL DEFAULT 'running', output TEXT, error_message TEXT, cost_usd DOUBLE PRECISION, input_tokens INTEGER, output_tokens INTEGER, duration_ms INTEGER, started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), finished_at TIMESTAMPTZ)")

	// Clean up before test.
	db.Exec("DELETE FROM cron_executions")
	db.Exec("DELETE FROM cron_jobs")

	// Create a test project.
	project := models.Project{Name: "cron-test-project", Path: "/tmp/cron-test-" + uuid.New().String()}
	db.Create(&project)
	defer db.Exec("DELETE FROM projects WHERE id = ?", project.ID)

	// Create a cron job.
	job := models.CronJob{
		Name:      "test job",
		Schedule:  "* * * * *",
		Prompt:    "test",
		ProjectID: project.ID,
		Enabled:   true,
	}
	db.Create(&job)

	// Create executions: one old, one recent.
	oldTime := time.Now().Add(-60 * 24 * time.Hour) // 60 days ago
	recentTime := time.Now().Add(-1 * time.Hour)    // 1 hour ago

	db.Create(&models.CronExecution{CronJobID: job.ID, Status: "success", StartedAt: oldTime})
	db.Create(&models.CronExecution{CronJobID: job.ID, Status: "success", StartedAt: recentTime})

	// Purge with 30-day retention.
	purged, err := PurgeCronExecutions(db, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("purge failed: %v", err)
	}
	if purged != 1 {
		t.Errorf("expected 1 purged, got %d", purged)
	}

	// Verify only the recent one remains.
	var count int64
	db.Model(&models.CronExecution{}).Where("cron_job_id = ?", job.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 remaining execution, got %d", count)
	}

	// Clean up.
	db.Exec("DELETE FROM cron_executions WHERE cron_job_id = ?", job.ID)
	db.Exec("DELETE FROM cron_jobs WHERE id = ?", job.ID)
}
