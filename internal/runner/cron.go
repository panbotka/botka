package runner

import (
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"botka/internal/config"
	"botka/internal/models"
)

const cronTickInterval = 30 * time.Second

// CronScheduler checks cron job schedules on a 30-second tick and triggers
// executions when they are due. It prevents overlapping runs of the same job
// and respects API rate limits via the shared UsageMonitor.
type CronScheduler struct {
	db       *gorm.DB
	cfg      *config.Config
	usageMon *UsageMonitor
	stop     chan struct{}
	wg       sync.WaitGroup
	running  map[int64]bool // track running jobs to prevent overlap
	mu       sync.Mutex
}

// NewCronScheduler creates a new CronScheduler.
func NewCronScheduler(db *gorm.DB, cfg *config.Config, usageMon *UsageMonitor) *CronScheduler {
	return &CronScheduler{
		db:       db,
		cfg:      cfg,
		usageMon: usageMon,
		running:  make(map[int64]bool),
	}
}

// Start begins the cron scheduler loop in a background goroutine.
func (cs *CronScheduler) Start() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.stop != nil {
		return // already running
	}

	cs.stop = make(chan struct{})
	cs.wg.Add(1)
	go cs.loop(cs.stop)
	slog.Info("cron scheduler started")
}

// Stop terminates the scheduler loop and waits for all running jobs to finish.
func (cs *CronScheduler) Stop() {
	cs.mu.Lock()
	if cs.stop != nil {
		close(cs.stop)
		cs.stop = nil
	}
	cs.mu.Unlock()

	cs.wg.Wait()
	slog.Info("cron scheduler stopped")
}

func (cs *CronScheduler) loop(stopCh <-chan struct{}) {
	defer cs.wg.Done()

	ticker := time.NewTicker(cronTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			cs.tick()
		}
	}
}

func (cs *CronScheduler) tick() {
	if limited, reason := cs.usageMon.IsRateLimited(); limited {
		slog.Debug("[cron] rate limited, skipping tick", "reason", reason)
		return
	}

	var jobs []models.CronJob
	if err := cs.db.Preload("Project").Where("enabled = ?", true).Find(&jobs).Error; err != nil {
		slog.Error("[cron] failed to query jobs", "error", err)
		return
	}

	now := time.Now()
	for i := range jobs {
		cs.maybeRun(&jobs[i], now)
	}
}

// maybeRun checks whether a job should fire at the given time and spawns
// its execution if so.
func (cs *CronScheduler) maybeRun(job *models.CronJob, now time.Time) {
	sched, err := cron.ParseStandard(job.Schedule)
	if err != nil {
		slog.Error("[cron] invalid schedule", "job_id", job.ID, "schedule", job.Schedule, "error", err)
		return
	}

	if !cs.shouldRun(job, sched, now) {
		return
	}

	cs.mu.Lock()
	if cs.running[job.ID] {
		cs.mu.Unlock()
		slog.Debug("[cron] job already running, skipping", "job_id", job.ID)
		return
	}
	cs.running[job.ID] = true
	cs.mu.Unlock()

	projectName := ""
	if job.Project != nil {
		projectName = job.Project.Name
	}
	slog.Info("[cron] triggering job",
		"job_id", job.ID, "name", job.Name, "project", projectName)

	cs.wg.Add(1)
	go cs.executeCronJob(job)
}

// shouldRun determines if a job's schedule matches the current time and it
// hasn't already run in this minute.
func (cs *CronScheduler) shouldRun(job *models.CronJob, sched cron.Schedule, now time.Time) bool {
	// Truncate to minute for comparison — cron expressions are minute-granularity.
	nowMinute := now.Truncate(time.Minute)

	// Check if the job already ran in this minute.
	if job.LastRunAt != nil && !job.LastRunAt.Truncate(time.Minute).Before(nowMinute) {
		return false
	}

	// The schedule's Next() returns the first matching time after the given time.
	// If Next(nowMinute - 1s) == nowMinute, the job should fire now.
	next := sched.Next(nowMinute.Add(-time.Second))
	return next.Equal(nowMinute)
}

// executeCronJob is a placeholder that creates a CronExecution record with
// status "success" and a dummy output. The actual Claude execution will be
// implemented in a follow-up task.
func (cs *CronScheduler) executeCronJob(job *models.CronJob) {
	defer cs.wg.Done()
	defer func() {
		cs.mu.Lock()
		delete(cs.running, job.ID)
		cs.mu.Unlock()
	}()

	now := time.Now()
	output := "placeholder: execution not yet implemented"
	status := "success"

	exec := models.CronExecution{
		CronJobID:  job.ID,
		Status:     status,
		Output:     &output,
		StartedAt:  now,
		FinishedAt: &now,
	}

	if err := cs.db.Create(&exec).Error; err != nil {
		slog.Error("[cron] failed to create execution record", "job_id", job.ID, "error", err)
		return
	}

	// Update denormalized fields on the job.
	cs.db.Model(job).Updates(map[string]interface{}{
		"last_run_at": now,
		"last_status": status,
	})

	slog.Info("[cron] job completed", "job_id", job.ID, "name", job.Name)
}

// PurgeCronExecutions deletes cron execution records older than the given
// retention period. Returns the number of deleted rows.
func PurgeCronExecutions(db *gorm.DB, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	result := db.Where("started_at < ?", cutoff).Delete(&models.CronExecution{})
	return result.RowsAffected, result.Error
}
