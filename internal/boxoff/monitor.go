package boxoff

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"botka/internal/box"
	"botka/internal/models"
)

const (
	// settingKey is the app_settings row holding the on/off switch.
	settingKey = "box_auto_off"
	// recentEvaluations is how many past evaluations the snapshot keeps for
	// the UI. They are in memory only: transient by nature, and refilled
	// within one interval after a restart.
	recentEvaluations = 20
	// firstTickDelay is how long after startup the first evaluation runs, so
	// the UI is not blank for a whole interval.
	firstTickDelay = 30 * time.Second
	// gatherTimeout bounds one full round of readings.
	gatherTimeout = 60 * time.Second
	// shutdownTimeout bounds the shutdown command itself.
	shutdownTimeout = 30 * time.Second
)

// Prober samples Box over SSH.
type Prober interface {
	Probe(ctx context.Context) (BoxReadings, error)
}

// KukatkoSource reads Kukátko's job queue depth.
type KukatkoSource interface {
	Fetch(ctx context.Context) (KukatkoReadings, error)
}

// Shutdowner powers Box off.
type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

// Config wires a Monitor. DB may be nil, in which case events are not
// persisted and the switch cannot be read from the database; everything else
// is required.
type Config struct {
	DB         *gorm.DB
	Prober     Prober
	Kukatko    KukatkoSource
	Activity   ActivitySource
	Shutdowner Shutdowner
	Thresholds Thresholds
}

// Snapshot is the monitor's state as the API exposes it.
type Snapshot struct {
	Enabled            bool         `json:"enabled"`
	Streak             int          `json:"streak"`
	RequiredChecks     int          `json:"required_checks"`
	IntervalSeconds    int          `json:"interval_seconds"`
	NextCheckAt        *time.Time   `json:"next_check_at"`
	EarliestShutdownAt *time.Time   `json:"earliest_shutdown_at"`
	LastEvaluation     *Evaluation  `json:"last_evaluation"`
	Recent             []Evaluation `json:"recent"`
	LastError          string       `json:"last_error,omitempty"`
}

// Monitor evaluates Box's idleness on a ticker and shuts it down once the
// conditions have held for Thresholds.IdleChecks consecutive evaluations.
type Monitor struct {
	cfg Config

	mu          sync.RWMutex
	enabled     bool
	streak      int
	nextCheckAt time.Time
	last        *Evaluation
	recent      []Evaluation
	lastErr     string

	// beforeShutdown is a test seam fired after the decision to shut down and
	// before the pre-shutdown re-check, so a test can make work appear in
	// exactly the window the re-check exists to close.
	beforeShutdown func()

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewMonitor returns a monitor for the given wiring. Call Start to run it.
func NewMonitor(cfg Config) *Monitor {
	return &Monitor{cfg: cfg, stopCh: make(chan struct{})}
}

// Start reads the setting and launches the evaluation loop.
func (m *Monitor) Start() {
	m.ReloadSetting()
	m.wg.Add(1)
	go m.loop()
}

// Stop halts the loop and waits for it to finish.
func (m *Monitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// ReloadSetting re-reads box_auto_off from the database, so flipping the
// switch takes effect without waiting for the next tick.
func (m *Monitor) ReloadSetting() {
	if m.cfg.DB == nil {
		return
	}
	var value string
	err := m.cfg.DB.Table("app_settings").Where("key = ?", settingKey).Pluck("value", &value).Error
	if err != nil {
		slog.Warn("box auto-off: reading the setting failed", "error", err)
		return
	}
	m.setEnabled(value == "true")
}

// setEnabled sets the switch without touching the database. Switching off also
// clears the streak, so re-arming the feature always starts a fresh count
// rather than inheriting quiet checks from before it was turned off.
func (m *Monitor) setEnabled(on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !on {
		m.streak = 0
	}
	m.enabled = on
}

// loop runs an evaluation firstTickDelay after start and every Interval after
// that.
func (m *Monitor) loop() {
	defer m.wg.Done()

	interval := m.cfg.Thresholds.Interval
	slog.Info("box auto-off monitor started",
		"interval", interval,
		"idle_checks", m.cfg.Thresholds.IdleChecks,
		"min_uptime", m.cfg.Thresholds.MinUptime,
	)

	timer := time.NewTimer(firstTickDelay)
	defer timer.Stop()
	m.setNextCheckAt(time.Now().Add(firstTickDelay))

	for {
		select {
		case <-m.stopCh:
			slog.Info("box auto-off monitor stopped")
			return
		case <-timer.C:
			ctx, cancel := context.WithTimeout(context.Background(), gatherTimeout)
			m.RunOnce(ctx)
			cancel()
			timer.Reset(interval)
			m.setNextCheckAt(time.Now().Add(interval))
		}
	}
}

// RunOnce performs one full evaluation and, when the streak is complete, shuts
// Box down. It returns the evaluation it acted on.
func (m *Monitor) RunOnce(ctx context.Context) Evaluation {
	in := m.gather(ctx)
	ev := Evaluate(time.Now(), in, m.cfg.Thresholds)

	m.mu.Lock()
	if ev.Idle {
		m.streak++
	} else {
		m.streak = 0
	}
	streak := m.streak
	m.last = &ev
	m.recent = append(m.recent, ev)
	if len(m.recent) > recentEvaluations {
		m.recent = m.recent[len(m.recent)-recentEvaluations:]
	}
	m.mu.Unlock()

	if !ev.Idle || streak < m.cfg.Thresholds.IdleChecks {
		return ev
	}

	m.fireShutdown(ctx, ev)
	return ev
}

// fireShutdown re-checks the cheap local conditions and, if they still hold,
// powers Box off. The streak is reset either way: after a shutdown there is
// nothing left to count, and after an abort the count must start over.
func (m *Monitor) fireShutdown(ctx context.Context, ev Evaluation) {
	if m.beforeShutdown != nil {
		m.beforeShutdown()
	}

	defer func() {
		m.mu.Lock()
		m.streak = 0
		m.mu.Unlock()
	}()

	// Gathering the readings took seconds, during which the runner may have
	// claimed a task or a chat may have started on Box. Those two checks are
	// local and cheap, so there is no reason to act on stale ones.
	if blocker, ok := m.recheckLocalWork(ctx); !ok {
		slog.Info("box auto-off: shutdown aborted", "reason", blocker.Detail)
		m.setLastError("")
		m.recordEvent(models.BoxAutoOffOutcomeAborted, ev, blocker.Detail)
		return
	}

	// Deliberately not derived from ctx: the caller's context bounds the
	// gathering round, and a shutdown decided at the very end of it must not
	// inherit whatever is left of that budget.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := m.cfg.Shutdowner.Shutdown(shutdownCtx); err != nil {
		slog.Error("box auto-off: shutdown failed", "error", err)
		m.setLastError(err.Error())
		m.recordEvent(models.BoxAutoOffOutcomeFailed, ev, err.Error())
		return
	}

	slog.Info("box auto-off: box shut down",
		"uptime_seconds", ev.Box.UptimeSeconds,
		"load1", ev.Box.Load1,
		"gpu_max_percent", ev.Box.GPUMaxPercent,
	)
	m.setLastError("")
	m.recordEvent(models.BoxAutoOffOutcomeShutdown, ev, "")
}

// recheckLocalWork re-runs the two conditions that can change between the
// readings and the shutdown command. A false return carries the blocker.
func (m *Monitor) recheckLocalWork(ctx context.Context) (Blocker, bool) {
	if tasks := m.cfg.Activity.RunningTasks(); len(tasks) > 0 {
		return Blocker{Name: BlockerBotkaTasks, Detail: "a task started during the evaluation"}, false
	}
	chats, err := m.cfg.Activity.BoxChatThreads(ctx)
	if err != nil {
		return Blocker{Name: BlockerBotkaBoxChats, Detail: "could not be determined: " + err.Error()}, false
	}
	if len(chats) > 0 {
		return Blocker{Name: BlockerBotkaBoxChats, Detail: "a chat started on box during the evaluation"}, false
	}
	return Blocker{}, true
}

// gather collects every reading the evaluation needs. Disabled and offline are
// terminal, so neither the scrape nor the activity lookup runs in those cases.
func (m *Monitor) gather(ctx context.Context) Inputs {
	in := Inputs{Enabled: m.isEnabled()}
	if !in.Enabled {
		return in
	}

	readings, err := m.cfg.Prober.Probe(ctx)
	if err != nil {
		in.ProbeError = err.Error()
		return in
	}
	in.BoxOnline = true
	in.Box = readings

	kukatko, err := m.cfg.Kukatko.Fetch(ctx)
	in.Kukatko = kukatko
	in.KukatkoOK = err == nil

	in.RunningTasks = m.cfg.Activity.RunningTasks()
	chats, err := m.cfg.Activity.BoxChatThreads(ctx)
	if err != nil {
		in.ActivityError = err.Error()
	} else {
		in.BoxChats = chats
	}

	return in
}

// Snapshot returns the monitor's current state for the API.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snap := Snapshot{
		Enabled:         m.enabled,
		Streak:          m.streak,
		RequiredChecks:  m.cfg.Thresholds.IdleChecks,
		IntervalSeconds: int(m.cfg.Thresholds.Interval.Seconds()),
		LastEvaluation:  m.last,
		Recent:          reversed(m.recent),
		LastError:       m.lastErr,
	}
	if !m.nextCheckAt.IsZero() {
		next := m.nextCheckAt
		snap.NextCheckAt = &next
		if m.last != nil {
			snap.EarliestShutdownAt = EarliestShutdownAt(time.Now(), *m.last, m.streak, next, m.cfg.Thresholds)
		}
	}
	return snap
}

// reversed returns the evaluations newest-first without aliasing the ring.
func reversed(in []Evaluation) []Evaluation {
	out := make([]Evaluation, 0, len(in))
	for i := len(in) - 1; i >= 0; i-- {
		out = append(out, in[i])
	}
	return out
}

// isEnabled reports the switch state.
func (m *Monitor) isEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// setNextCheckAt records when the next evaluation is due.
func (m *Monitor) setNextCheckAt(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextCheckAt = t
}

// setLastError records the most recent shutdown error, or clears it.
func (m *Monitor) setLastError(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastErr = msg
}

// recordEvent persists one shutdown attempt. It is best-effort: a database
// error is logged, never propagated, because losing a history row must not
// change what the monitor does.
func (m *Monitor) recordEvent(outcome string, ev Evaluation, note string) {
	if m.cfg.DB == nil {
		return
	}

	detail, err := json.Marshal(struct {
		Evaluation Evaluation `json:"evaluation"`
		Note       string     `json:"note,omitempty"`
	}{Evaluation: ev, Note: note})
	if err != nil {
		slog.Warn("box auto-off: encoding the event failed", "error", err)
		return
	}

	row := models.BoxAutoOffEvent{OccurredAt: time.Now(), Outcome: outcome, Detail: detail}
	if err := m.cfg.DB.Create(&row).Error; err != nil {
		slog.Warn("box auto-off: recording the event failed", "error", err, "outcome", outcome)
	}
}

// SSHShutdowner powers Box off over SSH, sharing the teardown-detection logic
// with the manual shutdown button.
type SSHShutdowner struct {
	run    box.RunFunc
	target string
}

// NewSSHShutdowner returns a Shutdowner running commands with run against
// sshTarget ("user@host").
func NewSSHShutdowner(run box.RunFunc, sshTarget string) *SSHShutdowner {
	return &SSHShutdowner{run: run, target: sshTarget}
}

// Shutdown powers Box off.
func (s *SSHShutdowner) Shutdown(ctx context.Context) error {
	return box.Shutdown(ctx, s.run, s.target)
}
