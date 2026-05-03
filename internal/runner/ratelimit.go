package runner

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// rateLimitSettingKey is the app_settings key under which the persisted gate
// state is stored as JSON.
const rateLimitSettingKey = "runner_rate_limit_pause"

// minPauseAdjustment is the minimum future window applied when a parsed reset
// time is already in the past (clock skew). Avoids immediate retry storms.
const minPauseAdjustment = 5 * time.Minute

// rateLimitSignals are the substrings that, when present in Claude's failure
// text, indicate Anthropic itself rejected the request because a quota was
// exhausted. Matched case-sensitively because Claude emits them verbatim.
var rateLimitSignals = []string{
	"You've hit your limit",
	"hit your usage limit",
	"rate_limit_error",
	"limit · resets",
}

// resetTimeRegex captures the time-of-day and timezone from phrases like
// "resets 4:50am (Europe/Prague)" or "resets 16:30 (UTC)". The timezone
// name appears verbatim in the second capture group.
var resetTimeRegex = regexp.MustCompile(
	`(?i)resets?\s+(\d{1,2}(?::\d{2})?\s*(?:am|pm)?)\s*\(([^)]+)\)`,
)

// DetectRateLimit reports whether failureText contains a Claude rate-limit
// signal, and parses the reset time when present. The returned resetAt is in
// UTC and represents the next future occurrence of the wall-clock time named
// in the message. ok=false means the message matched a signal but no reset
// time could be parsed; the caller should fall back to a configured cooldown.
//
// The text match is substring-based and case-sensitive — Claude emits these
// phrases verbatim and we intentionally avoid lowercasing to keep the false
// positive rate near zero.
func DetectRateLimit(failureText string) (hit bool, resetAt time.Time, ok bool) {
	if failureText == "" {
		return false, time.Time{}, false
	}
	matched := false
	for _, sig := range rateLimitSignals {
		if strings.Contains(failureText, sig) {
			matched = true
			break
		}
	}
	if !matched {
		return false, time.Time{}, false
	}

	resetAt, ok = parseResetTime(failureText, time.Now())
	return true, resetAt, ok
}

// parseResetTime extracts a reset time from failureText relative to now.
// Returns ok=false when no parsable time-and-timezone pair is present.
func parseResetTime(failureText string, now time.Time) (time.Time, bool) {
	m := resetTimeRegex.FindStringSubmatch(failureText)
	if m == nil {
		return time.Time{}, false
	}
	tz := strings.TrimSpace(m[2])
	loc, err := loadKnownLocation(tz)
	if err != nil {
		return time.Time{}, false
	}
	hour, minute, ok := parseTimeOfDay(m[1])
	if !ok {
		return time.Time{}, false
	}

	nowInTZ := now.In(loc)
	candidate := time.Date(
		nowInTZ.Year(), nowInTZ.Month(), nowInTZ.Day(),
		hour, minute, 0, 0, loc,
	)
	if !candidate.After(nowInTZ) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate.UTC(), true
}

// loadKnownLocation returns the time.Location for one of the timezones we
// recognize. Anything else returns an error so the caller falls back to the
// configured cooldown rather than guessing.
func loadKnownLocation(tz string) (*time.Location, error) {
	switch tz {
	case "UTC":
		return time.UTC, nil
	case "Europe/Prague":
		return time.LoadLocation("Europe/Prague")
	default:
		return nil, fmt.Errorf("unknown timezone %q", tz)
	}
}

// parseTimeOfDay accepts "4:50am", "4:50 am", "4am", "16:30", or "16" and
// returns (hour, minute) in 24-hour form. Whitespace around am/pm is tolerated.
func parseTimeOfDay(s string) (hour, minute int, ok bool) {
	clean := strings.ToLower(strings.TrimSpace(s))
	pm := false
	twelveHour := false
	switch {
	case strings.HasSuffix(clean, "am"):
		twelveHour = true
		clean = strings.TrimSpace(strings.TrimSuffix(clean, "am"))
	case strings.HasSuffix(clean, "pm"):
		twelveHour = true
		pm = true
		clean = strings.TrimSpace(strings.TrimSuffix(clean, "pm"))
	}

	parts := strings.Split(clean, ":")
	if len(parts) == 0 || len(parts) > 2 {
		return 0, 0, false
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	m := 0
	if len(parts) == 2 {
		m, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
	}

	if twelveHour {
		if h < 1 || h > 12 {
			return 0, 0, false
		}
		// 12am = 00, 12pm = 12, 1pm-11pm = 13-23.
		switch {
		case h == 12 && !pm:
			h = 0
		case h != 12 && pm:
			h += 12
		}
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// PauseSource identifies what caused the runner to be paused. Right now only
// "rate_limit" is emitted; the field is exposed so the API surface can grow
// without a follow-up migration.
type PauseSource string

const (
	// PauseSourceRateLimit indicates the gate was tripped by Claude returning a
	// rate-limit error.
	PauseSourceRateLimit PauseSource = "rate_limit"
)

// RateLimitGate enforces a global pause on task launches when Claude itself
// reports a rate-limit error. The gate is independent from UsageMonitor — both
// gates run, and the runner is paused when either says so. State is persisted
// to the app_settings table so a restart does not reopen the gate.
//
// Mutex ordering: the gate's own mutex is leaf-level and never held while
// calling out to the runner.
type RateLimitGate struct {
	db *gorm.DB

	mu          sync.RWMutex
	pausedUntil time.Time
	reason      string
	taskID      uuid.UUID
	source      PauseSource
}

// NewRateLimitGate constructs a gate backed by db. Pass nil for an in-memory
// gate (used in tests that don't need persistence).
func NewRateLimitGate(db *gorm.DB) *RateLimitGate {
	g := &RateLimitGate{db: db}
	g.load()
	return g
}

// PausedUntil returns the wall-clock time at which the gate clears. A zero
// value means the gate is not active.
func (g *RateLimitGate) PausedUntil() time.Time {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.pausedUntil
}

// Snapshot returns a copy of the current gate state. The boolean is true when
// the gate is currently active (pausedUntil is in the future).
func (g *RateLimitGate) Snapshot() (active bool, pausedUntil time.Time, reason string, source PauseSource, taskID uuid.UUID) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.pausedUntil.IsZero() || time.Now().After(g.pausedUntil) {
		return false, g.pausedUntil, g.reason, g.source, g.taskID
	}
	return true, g.pausedUntil, g.reason, g.source, g.taskID
}

// IsActive reports whether the gate is currently blocking task launches.
func (g *RateLimitGate) IsActive() bool {
	active, _, _, _, _ := g.Snapshot()
	return active
}

// PauseUntil sets the gate to clear at resetAt. The new pausedUntil is the
// MAX of the existing and proposed times so a later pause from another task
// is never shortened. reason is a human-readable string surfaced via the API
// and logs. taskID is optional and identifies the task that tripped the gate.
//
// If resetAt is already in the past or within minPauseAdjustment from now,
// it is bumped to now+minPauseAdjustment to avoid an immediate retry storm
// from clock skew or borderline parses.
func (g *RateLimitGate) PauseUntil(resetAt time.Time, reason string, taskID uuid.UUID) {
	if resetAt.IsZero() {
		return
	}
	now := time.Now()
	minimum := now.Add(minPauseAdjustment)
	if resetAt.Before(minimum) {
		resetAt = minimum
	}

	g.mu.Lock()
	if !g.pausedUntil.IsZero() && resetAt.Before(g.pausedUntil) {
		// Existing pause is longer; keep it but refresh reason if newer is
		// non-empty. We don't shorten the gate.
		g.mu.Unlock()
		return
	}
	g.pausedUntil = resetAt.UTC()
	g.reason = reason
	g.taskID = taskID
	g.source = PauseSourceRateLimit
	g.mu.Unlock()

	g.persist()
}

// PauseFor pauses the gate for the given duration. Convenience wrapper around
// PauseUntil for callers that only have a cooldown (no parsed reset time).
func (g *RateLimitGate) PauseFor(d time.Duration, reason string, taskID uuid.UUID) {
	g.PauseUntil(time.Now().Add(d), reason, taskID)
}

// Clear immediately resets the gate. Intended for the manual-override
// endpoint.
func (g *RateLimitGate) Clear() {
	g.mu.Lock()
	g.pausedUntil = time.Time{}
	g.reason = ""
	g.taskID = uuid.Nil
	g.source = ""
	g.mu.Unlock()
	g.persist()
}

// persistedGate is the JSON shape stored in app_settings.
type persistedGate struct {
	PausedUntil time.Time `json:"paused_until"`
	Reason      string    `json:"reason"`
	TaskID      string    `json:"task_id,omitempty"`
	Source      string    `json:"source,omitempty"`
}

func (g *RateLimitGate) persist() {
	if g.db == nil {
		return
	}
	g.mu.RLock()
	state := persistedGate{
		PausedUntil: g.pausedUntil,
		Reason:      g.reason,
		Source:      string(g.source),
	}
	if g.taskID != uuid.Nil {
		state.TaskID = g.taskID.String()
	}
	g.mu.RUnlock()

	value, err := json.Marshal(state)
	if err != nil {
		slog.Error("rate-limit gate: failed to marshal state", "error", err)
		return
	}
	// UPSERT into app_settings.
	err = g.db.Exec(
		`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		rateLimitSettingKey, value,
	).Error
	if err != nil {
		slog.Error("rate-limit gate: failed to persist state", "error", err)
	}
}

func (g *RateLimitGate) load() {
	if g.db == nil {
		return
	}
	var value string
	err := g.db.Table("app_settings").
		Where("key = ?", rateLimitSettingKey).
		Pluck("value", &value).Error
	if err != nil || value == "" {
		return
	}
	var state persistedGate
	if err := json.Unmarshal([]byte(value), &state); err != nil {
		slog.Warn("rate-limit gate: failed to parse stored state", "error", err)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pausedUntil = state.PausedUntil
	g.reason = state.Reason
	g.source = PauseSource(state.Source)
	if state.TaskID != "" {
		if id, err := uuid.Parse(state.TaskID); err == nil {
			g.taskID = id
		}
	}
}
