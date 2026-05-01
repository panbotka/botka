package runner

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"botka/internal/models"
)

func TestComputeNextRun_EveryMinute(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 5, 1, 10, 30, 15, 0, time.UTC)
	next, err := ComputeNextRun("* * * * *", from)
	if err != nil {
		t.Fatalf("ComputeNextRun: %v", err)
	}
	want := time.Date(2026, 5, 1, 10, 31, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestComputeNextRun_DailyNineAM(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)
	next, err := ComputeNextRun("0 9 * * *", from)
	if err != nil {
		t.Fatalf("ComputeNextRun: %v", err)
	}
	// Next 09:00 is the next day.
	want := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestComputeNextRun_StepEvery15Minutes(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 5, 1, 10, 0, 30, 0, time.UTC)
	next, err := ComputeNextRun("*/15 * * * *", from)
	if err != nil {
		t.Fatalf("ComputeNextRun: %v", err)
	}
	want := time.Date(2026, 5, 1, 10, 15, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestComputeNextRun_CommaList(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)
	next, err := ComputeNextRun("0 9,17 * * *", from)
	if err != nil {
		t.Fatalf("ComputeNextRun: %v", err)
	}
	want := time.Date(2026, 5, 1, 17, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestComputeNextRun_Range(t *testing.T) {
	t.Parallel()

	// Mon-Fri at 09:00. May 1 2026 is a Friday; next at "10:30 Fri" → Mon May 4.
	from := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)
	next, err := ComputeNextRun("0 9 * * 1-5", from)
	if err != nil {
		t.Fatalf("ComputeNextRun: %v", err)
	}
	want := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("got %v, want %v", next, want)
	}
}

func TestComputeNextRun_InvalidExpression(t *testing.T) {
	t.Parallel()

	_, err := ComputeNextRun("not a cron", time.Now())
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestScheduleScheduler_StartStop(t *testing.T) {
	t.Parallel()

	s := &ScheduleScheduler{}
	s.Start()
	// idempotent
	s.Start()
	s.Stop()
}

func TestScheduleScheduler_FireSchedule(t *testing.T) {
	db := setupTestDB(t)
	cleanScheduleTables(t, db)

	proj := createProject(t, db, "schedule-fire")
	now := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)
	past := now.Add(-time.Minute)

	sched := models.TaskSchedule{
		ProjectID:      proj.ID,
		Title:          "Daily digest",
		Spec:           "Summarize today's PRs",
		CronExpression: "*/5 * * * *",
		Priority:       3,
		Enabled:        true,
		NextRunAt:      &past,
	}
	if err := db.Create(&sched).Error; err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	s := NewScheduleScheduler(db)
	taskID, err := s.fireSchedule(&sched, now)
	if err != nil {
		t.Fatalf("fireSchedule: %v", err)
	}
	if taskID == "" {
		t.Fatal("expected a task ID")
	}

	// Task created in pending state with schedule_id set.
	var task models.Task
	if err := db.First(&task, "id = ?", taskID).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != models.TaskStatusPending {
		t.Errorf("status = %s, want pending", task.Status)
	}
	if task.Title != "Daily digest" {
		t.Errorf("title = %q, want Daily digest", task.Title)
	}
	if task.Spec != "Summarize today's PRs" {
		t.Errorf("spec = %q", task.Spec)
	}
	if task.Priority != 3 {
		t.Errorf("priority = %d, want 3", task.Priority)
	}
	if task.ScheduleID == nil || *task.ScheduleID != sched.ID {
		t.Errorf("schedule_id mismatch")
	}

	// Schedule advanced.
	var reloaded models.TaskSchedule
	if err := db.First(&reloaded, sched.ID).Error; err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if reloaded.LastRunAt == nil {
		t.Error("last_run_at not set")
	}
	if reloaded.NextRunAt == nil {
		t.Error("next_run_at not set")
	} else if !reloaded.NextRunAt.After(now) {
		t.Errorf("next_run_at %v should be after now %v", *reloaded.NextRunAt, now)
	}
}

func TestScheduleScheduler_RunNow(t *testing.T) {
	db := setupTestDB(t)
	cleanScheduleTables(t, db)

	proj := createProject(t, db, "schedule-runnow")
	future := time.Now().Add(2 * time.Hour)
	sched := models.TaskSchedule{
		ProjectID:      proj.ID,
		Title:          "Manual fire",
		CronExpression: "0 * * * *",
		Enabled:        false, // Disabled — RunNow should bypass.
		NextRunAt:      &future,
	}
	if err := db.Create(&sched).Error; err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	s := NewScheduleScheduler(db)
	taskID, err := s.RunNow(sched.ID)
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	var task models.Task
	if err := db.First(&task, "id = ?", taskID).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.ScheduleID == nil || *task.ScheduleID != sched.ID {
		t.Error("schedule_id not set on RunNow task")
	}

	// next_run_at should NOT have been advanced.
	var reloaded models.TaskSchedule
	if err := db.First(&reloaded, sched.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.NextRunAt == nil || !reloaded.NextRunAt.Equal(future.Truncate(time.Microsecond)) {
		// PostgreSQL truncates to microsecond precision; allow that rounding.
		if reloaded.NextRunAt == nil || reloaded.NextRunAt.Sub(future).Abs() > time.Second {
			t.Errorf("next_run_at = %v, want %v (unchanged)", reloaded.NextRunAt, future)
		}
	}
	if reloaded.LastRunAt == nil {
		t.Error("last_run_at not set after RunNow")
	}
}

func TestScheduleScheduler_RunNow_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanScheduleTables(t, db)

	s := NewScheduleScheduler(db)
	if _, err := s.RunNow(999999); err == nil {
		t.Error("expected error for missing schedule")
	}
}

func TestScheduleScheduler_Tick_FiresOnlyDueAndEnabled(t *testing.T) {
	db := setupTestDB(t)
	cleanScheduleTables(t, db)

	proj := createProject(t, db, "schedule-tick")
	now := time.Now()
	past := now.Add(-2 * time.Minute)
	future := now.Add(2 * time.Hour)

	due := models.TaskSchedule{
		ProjectID: proj.ID, Title: "due",
		CronExpression: "* * * * *", Enabled: true, NextRunAt: &past,
	}
	notDue := models.TaskSchedule{
		ProjectID: proj.ID, Title: "not-due",
		CronExpression: "* * * * *", Enabled: true, NextRunAt: &future,
	}
	disabled := models.TaskSchedule{
		ProjectID: proj.ID, Title: "disabled",
		CronExpression: "* * * * *", Enabled: false, NextRunAt: &past,
	}
	for _, s := range []*models.TaskSchedule{&due, &notDue, &disabled} {
		if err := db.Create(s).Error; err != nil {
			t.Fatalf("create schedule: %v", err)
		}
	}
	// GORM skips zero-value bools when the column has default:true, so persist
	// the disabled flag with an explicit update.
	db.Model(&disabled).Update("enabled", false)

	NewScheduleScheduler(db).tick()

	// Only the due+enabled schedule should produce a task.
	var tasks []models.Task
	if err := db.Find(&tasks).Error; err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "due" {
		t.Errorf("wrong task fired: %q", tasks[0].Title)
	}
}

// cleanScheduleTables truncates schedule-related tables along with the usual
// runner test fixtures.
func cleanScheduleTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	// AutoMigrate the task_schedules table once; the runner test bootstrap
	// drops only the tables it knows about, so we register this one here.
	if err := db.AutoMigrate(&models.TaskSchedule{}); err != nil {
		t.Fatalf("automigrate task_schedules: %v", err)
	}
	db.Exec("ALTER TABLE tasks ADD COLUMN IF NOT EXISTS schedule_id BIGINT REFERENCES task_schedules(id) ON DELETE SET NULL")
	db.Exec("TRUNCATE TABLE task_executions, tasks, task_schedules, projects, runner_state CASCADE")
}
