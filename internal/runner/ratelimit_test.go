package runner

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"botka/internal/models"
)

// fixedNow returns a deterministic "now" used for time-of-day parsing tests.
// We use 2026-05-03T00:30:00Z which falls inside the real incident window.
func fixedNow() time.Time {
	return time.Date(2026, 5, 3, 0, 30, 0, 0, time.UTC)
}

func TestDetectRateLimit_PositivePhrases(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"You've hit your limit", `You've hit your limit · resets 4:50am (Europe/Prague)`},
		{"hit your usage limit", `Sorry, you have hit your usage limit. Try again later.`},
		{"rate_limit_error", `{"type":"error","error":{"type":"rate_limit_error","message":"too many"}}`},
		{"limit · resets", `Daily limit · resets 16:30 (UTC) please try again`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hit, _, _ := DetectRateLimit(tc.text)
			if !hit {
				t.Errorf("expected hit=true for %q", tc.text)
			}
		})
	}
}

func TestDetectRateLimit_Negative(t *testing.T) {
	cases := []string{
		"",
		"task failed: undefined symbol foo",
		"build error: missing import",
		// Lowercase doesn't match the case-sensitive signals.
		"you've hit your limit",
	}
	for _, txt := range cases {
		hit, _, _ := DetectRateLimit(txt)
		if hit {
			t.Errorf("expected hit=false for %q", txt)
		}
	}
}

func TestDetectRateLimit_ParsesPragueTime(t *testing.T) {
	text := `You've hit your limit · resets 4:50am (Europe/Prague)`
	hit, resetAt, ok := DetectRateLimit(text)
	if !hit {
		t.Fatal("expected hit=true")
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	prague, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		t.Fatalf("load Prague: %v", err)
	}
	got := resetAt.In(prague)
	if got.Hour() != 4 || got.Minute() != 50 {
		t.Errorf("expected 4:50 in Prague, got %02d:%02d", got.Hour(), got.Minute())
	}
	if !resetAt.After(time.Now()) {
		t.Errorf("expected resetAt to be in the future, got %s", resetAt)
	}
}

func TestDetectRateLimit_Parses24Hour(t *testing.T) {
	text := `Daily limit · resets 16:30 (UTC)`
	hit, resetAt, ok := DetectRateLimit(text)
	if !hit || !ok {
		t.Fatalf("expected hit and ok, got hit=%v ok=%v", hit, ok)
	}
	got := resetAt.UTC()
	if got.Hour() != 16 || got.Minute() != 30 {
		t.Errorf("expected 16:30 UTC, got %02d:%02d", got.Hour(), got.Minute())
	}
}

func TestDetectRateLimit_ParsesPM(t *testing.T) {
	text := `Daily limit · resets 9:15pm (UTC)`
	hit, resetAt, ok := DetectRateLimit(text)
	if !hit || !ok {
		t.Fatalf("expected hit and ok, got hit=%v ok=%v", hit, ok)
	}
	got := resetAt.UTC()
	if got.Hour() != 21 || got.Minute() != 15 {
		t.Errorf("expected 21:15 UTC, got %02d:%02d", got.Hour(), got.Minute())
	}
}

func TestDetectRateLimit_NextDayWhenAlreadyPassed(t *testing.T) {
	now := fixedNow() // 2026-05-03 00:30 UTC
	text := `Limit · resets 12:00am (UTC)`
	if _, ok := parseResetTime(text, now); !ok {
		t.Fatal("expected ok=true")
	}
	got, _ := parseResetTime(text, now)
	// 12am = 00:00 UTC. The 00:00 of today (2026-05-03) is already past, so
	// we expect the next occurrence at 2026-05-04 00:00 UTC.
	want := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestDetectRateLimit_PragueDifferentFromUTC(t *testing.T) {
	// 4:50am Prague on 2026-05-03 is 02:50 UTC (CEST = UTC+2 in May).
	now := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	got, ok := parseResetTime(`resets 4:50am (Europe/Prague)`, now)
	if !ok {
		t.Fatal("expected ok=true")
	}
	want := time.Date(2026, 5, 3, 2, 50, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestDetectRateLimit_MalformedReset(t *testing.T) {
	// Signal phrase present but no parsable reset time. ok must be false so
	// the caller falls back to a configured cooldown.
	text := `You've hit your limit, but the reset time is unspecified`
	hit, _, ok := DetectRateLimit(text)
	if !hit {
		t.Fatal("expected hit=true")
	}
	if ok {
		t.Error("expected ok=false (no parsable reset time)")
	}
}

func TestDetectRateLimit_UnknownTimezone(t *testing.T) {
	text := `daily limit · resets 4:50am (America/Los_Angeles)`
	hit, _, ok := DetectRateLimit(text)
	if !hit {
		t.Fatal("expected hit=true (signal still present)")
	}
	if ok {
		t.Error("expected ok=false for unknown timezone")
	}
}

func TestDetectRateLimit_CaseInsensitiveAMPM(t *testing.T) {
	// "AM"/"PM" uppercase variants should still parse.
	text := `daily limit · resets 11:00AM (UTC)`
	hit, resetAt, ok := DetectRateLimit(text)
	if !hit || !ok {
		t.Fatalf("expected hit and ok, got hit=%v ok=%v", hit, ok)
	}
	if resetAt.UTC().Hour() != 11 {
		t.Errorf("expected 11:00 UTC, got %02d:%02d", resetAt.UTC().Hour(), resetAt.UTC().Minute())
	}
}

func TestParseTimeOfDay(t *testing.T) {
	cases := []struct {
		in       string
		wantHour int
		wantMin  int
		wantOK   bool
	}{
		{"4:50am", 4, 50, true},
		{"4:50 am", 4, 50, true},
		{"12:00am", 0, 0, true},
		{"12:00pm", 12, 0, true},
		{"1:00pm", 13, 0, true},
		{"11:59pm", 23, 59, true},
		{"16:30", 16, 30, true},
		{"00:00", 0, 0, true},
		{"23:59", 23, 59, true},
		{"4am", 4, 0, true},
		{"24:00", 0, 0, false},
		{"13:00pm", 0, 0, false},
		{"abc", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			h, m, ok := parseTimeOfDay(tc.in)
			if ok != tc.wantOK {
				t.Errorf("ok: want %v, got %v", tc.wantOK, ok)
			}
			if !tc.wantOK {
				return
			}
			if h != tc.wantHour || m != tc.wantMin {
				t.Errorf("want %02d:%02d, got %02d:%02d", tc.wantHour, tc.wantMin, h, m)
			}
		})
	}
}

func TestRateLimitGate_PauseAndClear(t *testing.T) {
	g := NewRateLimitGate(nil)
	if g.IsActive() {
		t.Fatal("expected new gate to be inactive")
	}
	resetAt := time.Now().Add(2 * time.Hour)
	g.PauseUntil(resetAt, "test", uuid.New())
	if !g.IsActive() {
		t.Fatal("expected gate to be active after PauseUntil")
	}
	g.Clear()
	if g.IsActive() {
		t.Fatal("expected gate to be inactive after Clear")
	}
}

func TestRateLimitGate_ExpiresAutomatically(t *testing.T) {
	// Directly seed a past pausedUntil to test the auto-expire path. Going
	// through PauseUntil would bump it to now+minPauseAdjustment.
	g := NewRateLimitGate(nil)
	g.mu.Lock()
	g.pausedUntil = time.Now().Add(-1 * time.Hour)
	g.reason = "expired"
	g.source = PauseSourceRateLimit
	g.mu.Unlock()

	if g.IsActive() {
		t.Errorf("gate should not be active for past pausedUntil")
	}
	active, _, _, _, _ := g.Snapshot()
	if active {
		t.Errorf("Snapshot should report inactive for past pausedUntil")
	}
}

func TestRateLimitGate_PauseUntilTakesMax(t *testing.T) {
	g := NewRateLimitGate(nil)
	earlier := time.Now().Add(30 * time.Minute)
	later := time.Now().Add(2 * time.Hour)

	g.PauseUntil(later, "later", uuid.Nil)
	g.PauseUntil(earlier, "earlier", uuid.Nil) // must NOT shorten the gate

	got := g.PausedUntil()
	if got.Before(later.Add(-time.Second)) {
		t.Errorf("expected gate to keep the later pausedUntil, got %s", got)
	}
}

func TestRateLimitGate_ClockSkewMinimum(t *testing.T) {
	g := NewRateLimitGate(nil)
	g.PauseUntil(time.Now().Add(-1*time.Hour), "skewed", uuid.Nil)
	got := g.PausedUntil()
	if got.Before(time.Now().Add(minPauseAdjustment - time.Second)) {
		t.Errorf("expected pausedUntil >= now + %s, got %s", minPauseAdjustment, got)
	}
}

func TestRateLimitGate_PersistsAcrossReload(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	// app_settings table is created by setupTestDB indirectly via runner_test
	// but its truncate happens in cleanTables — make sure the table exists.
	db.Exec(`CREATE TABLE IF NOT EXISTS app_settings (
		key VARCHAR(100) PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TIMESTAMPTZ
	)`)
	db.Exec("DELETE FROM app_settings WHERE key = ?", rateLimitSettingKey)

	g1 := NewRateLimitGate(db)
	resetAt := time.Now().Add(90 * time.Minute).UTC().Truncate(time.Second)
	taskID := uuid.New()
	g1.PauseUntil(resetAt, "persisted reason", taskID)

	g2 := NewRateLimitGate(db)
	active, until, reason, source, gotID := g2.Snapshot()
	if !active {
		t.Fatal("expected reloaded gate to be active")
	}
	if !until.Truncate(time.Second).Equal(resetAt) {
		t.Errorf("expected pausedUntil %s, got %s", resetAt, until)
	}
	if reason != "persisted reason" {
		t.Errorf("expected reason 'persisted reason', got %q", reason)
	}
	if source != PauseSourceRateLimit {
		t.Errorf("expected source 'rate_limit', got %q", source)
	}
	if gotID != taskID {
		t.Errorf("expected taskID %s, got %s", taskID, gotID)
	}

	g2.Clear()
	g3 := NewRateLimitGate(db)
	if g3.IsActive() {
		t.Error("expected gate to be inactive after Clear and reload")
	}
}

func TestRunnerTick_SkipsLaunchWhenGateActive(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-gated")
	createTask(t, db, proj.ID, "queued-task", models.TaskStatusQueued)

	// UsageMonitor reports OK so the only gate stopping us is the rate-limit
	// gate.
	usage := NewUsageMonitor("", 0.99, 0.99)
	usage.lastPollOK = true
	usage.info = UsageInfo{FiveHourPct: 0.10, SevenDayPct: 0.20}

	gate := NewRateLimitGate(nil)
	gate.PauseUntil(time.Now().Add(2*time.Hour), "test pause", uuid.New())

	r := &Runner{
		db:             db,
		state:          models.StateRunning,
		maxWorkers:     2,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		retryNotBefore: make(map[uuid.UUID]time.Time),
		usageMon:       usage,
		rateLimitGate:  gate,
		TaskEvents:     NewTaskEventHub(),
	}

	r.tick()

	// Gate is active, so no executor should have been spawned.
	r.mu.RLock()
	count := len(r.executors)
	r.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 executors with gate active, got %d", count)
	}

	// Task must remain queued.
	var reloaded models.Task
	if err := db.First(&reloaded).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("expected task to remain queued, got %s", reloaded.Status)
	}
}

func TestRunnerStatus_ExposesGateFields(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	usage := NewUsageMonitor("", 0.99, 0.99)
	usage.lastPollOK = true

	gate := NewRateLimitGate(nil)
	resetAt := time.Now().Add(45 * time.Minute)
	gate.PauseUntil(resetAt, "Claude rate limit (task abc)", uuid.Nil)

	r := &Runner{
		db:            db,
		state:         models.StatePaused,
		maxWorkers:    2,
		executors:     make(map[uuid.UUID]*activeTask),
		usageMon:      usage,
		rateLimitGate: gate,
	}

	status := r.GetStatus()
	if status.PausedUntil == nil {
		t.Fatal("expected PausedUntil to be non-nil when gate active")
	}
	if status.PauseReason == nil || *status.PauseReason == "" {
		t.Error("expected PauseReason to be populated")
	}
	if status.PauseSource == nil || *status.PauseSource != string(PauseSourceRateLimit) {
		t.Errorf("expected PauseSource %q, got %v", PauseSourceRateLimit, status.PauseSource)
	}
}

func TestRunnerStatus_NilWhenNotPaused(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	usage := NewUsageMonitor("", 0.99, 0.99)
	usage.lastPollOK = true

	r := &Runner{
		db:            db,
		state:         models.StatePaused,
		maxWorkers:    2,
		executors:     make(map[uuid.UUID]*activeTask),
		usageMon:      usage,
		rateLimitGate: NewRateLimitGate(nil),
	}

	status := r.GetStatus()
	if status.PausedUntil != nil {
		t.Errorf("expected PausedUntil nil when no pause active, got %v", status.PausedUntil)
	}
	if status.PauseReason != nil {
		t.Errorf("expected PauseReason nil, got %v", status.PauseReason)
	}
	if status.PauseSource != nil {
		t.Errorf("expected PauseSource nil, got %v", status.PauseSource)
	}
}

func TestDetectRateLimit_SubstringInLongerText(t *testing.T) {
	// The detector should match when the signal is embedded inside a longer
	// failure summary, not require equality.
	long := strings.Repeat("noise ", 100) + `You've hit your limit · resets 4:50am (Europe/Prague)` + strings.Repeat(" more noise", 100)
	hit, _, ok := DetectRateLimit(long)
	if !hit || !ok {
		t.Errorf("expected hit and ok in long text, got hit=%v ok=%v", hit, ok)
	}
}
