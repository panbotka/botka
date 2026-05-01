package runner

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"botka/internal/models"
)

// scheduleTickInterval is how often the scheduler scans for due schedules.
// Schedules are minute-granular, so a minute is the natural cadence.
const scheduleTickInterval = time.Minute

// ScheduleScheduler is a background goroutine that wakes every minute,
// finds enabled task_schedules whose next_run_at is due, creates the
// corresponding pending task, and advances next_run_at to the next firing.
type ScheduleScheduler struct {
	db   *gorm.DB
	stop chan struct{}
	wg   sync.WaitGroup
	mu   sync.Mutex
}

// NewScheduleScheduler creates a new ScheduleScheduler bound to db.
func NewScheduleScheduler(db *gorm.DB) *ScheduleScheduler {
	return &ScheduleScheduler{db: db}
}

// Start begins the scheduler loop in a background goroutine. Calling Start on
// an already-running scheduler is a no-op.
func (s *ScheduleScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stop != nil {
		return
	}
	s.stop = make(chan struct{})
	s.wg.Add(1)
	go s.loop(s.stop)
	slog.Info("schedule scheduler started")
}

// Stop terminates the scheduler loop and waits for the goroutine to exit.
func (s *ScheduleScheduler) Stop() {
	s.mu.Lock()
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	s.mu.Unlock()
	s.wg.Wait()
	slog.Info("schedule scheduler stopped")
}

func (s *ScheduleScheduler) loop(stopCh <-chan struct{}) {
	defer s.wg.Done()

	// Run an immediate tick on startup so missed schedules fire promptly.
	if s.db != nil {
		s.tick()
	}

	ticker := time.NewTicker(scheduleTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

// tick scans for enabled schedules due now (or earlier) and fires each one.
func (s *ScheduleScheduler) tick() {
	now := time.Now()
	var schedules []models.TaskSchedule
	err := s.db.
		Where("enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", true, now).
		Find(&schedules).Error
	if err != nil {
		slog.Error("[schedule] failed to query due schedules", "error", err)
		return
	}

	for i := range schedules {
		if _, err := s.fireSchedule(&schedules[i], now); err != nil {
			slog.Error("[schedule] failed to fire schedule",
				"schedule_id", schedules[i].ID,
				"title", schedules[i].Title,
				"error", err)
		}
	}
}

// fireSchedule creates a pending task from the schedule and advances its
// next_run_at. Returns the created task ID. The whole operation runs in a
// transaction so a partial failure cannot leave next_run_at unset.
func (s *ScheduleScheduler) fireSchedule(sched *models.TaskSchedule, now time.Time) (string, error) {
	parsed, err := cron.ParseStandard(sched.CronExpression)
	if err != nil {
		return "", fmt.Errorf("invalid cron expression %q: %w", sched.CronExpression, err)
	}

	var taskID string
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		task := models.Task{
			Title:      sched.Title,
			Spec:       sched.Spec,
			Status:     models.TaskStatusPending,
			Priority:   sched.Priority,
			ProjectID:  sched.ProjectID,
			ScheduleID: &sched.ID,
		}
		if err := tx.Create(&task).Error; err != nil {
			return fmt.Errorf("create task: %w", err)
		}
		taskID = task.ID.String()

		next := parsed.Next(now)
		updates := map[string]interface{}{
			"last_run_at": now,
			"next_run_at": next,
		}
		if err := tx.Model(sched).Updates(updates).Error; err != nil {
			return fmt.Errorf("update schedule: %w", err)
		}
		return nil
	})
	if txErr != nil {
		return "", txErr
	}

	slog.Info("[schedule] fired",
		"schedule_id", sched.ID,
		"title", sched.Title,
		"task_id", taskID)
	return taskID, nil
}

// RunNow immediately creates a pending task from the schedule, regardless of
// next_run_at. last_run_at is updated, but next_run_at is not changed — the
// scheduled cadence continues from its previous trajectory. Returns the
// created task ID.
func (s *ScheduleScheduler) RunNow(scheduleID int64) (string, error) {
	var sched models.TaskSchedule
	if err := s.db.First(&sched, scheduleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("schedule %d not found", scheduleID)
		}
		return "", fmt.Errorf("load schedule: %w", err)
	}

	now := time.Now()
	var taskID string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		task := models.Task{
			Title:      sched.Title,
			Spec:       sched.Spec,
			Status:     models.TaskStatusPending,
			Priority:   sched.Priority,
			ProjectID:  sched.ProjectID,
			ScheduleID: &sched.ID,
		}
		if err := tx.Create(&task).Error; err != nil {
			return fmt.Errorf("create task: %w", err)
		}
		taskID = task.ID.String()
		if err := tx.Model(&sched).Update("last_run_at", now).Error; err != nil {
			return fmt.Errorf("update schedule: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	slog.Info("[schedule] run-now",
		"schedule_id", sched.ID,
		"title", sched.Title,
		"task_id", taskID)
	return taskID, nil
}

// ComputeNextRun returns the next firing time for the given cron expression
// strictly after `from`. Returns an error if the expression is invalid.
func ComputeNextRun(expression string, from time.Time) (time.Time, error) {
	parsed, err := cron.ParseStandard(expression)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.Next(from), nil
}
