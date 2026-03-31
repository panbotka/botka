package runner

import (
	"testing"
	"time"
)

func TestNewUsageMonitor(t *testing.T) {
	m := NewUsageMonitor("/usr/bin/test-cmd", 0.90, 0.95)
	if m.cmdPath != "/usr/bin/test-cmd" {
		t.Errorf("expected cmdPath %q, got %q", "/usr/bin/test-cmd", m.cmdPath)
	}
}

func TestIsRateLimited_BelowBothThresholds(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	m.info = UsageInfo{
		FiveHourPct: 0.50,
		SevenDayPct: 0.60,
	}

	limited, reason := m.IsRateLimited()
	if limited {
		t.Errorf("expected not rate limited, got limited with reason: %s", reason)
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestIsRateLimited_FiveHourExceedsThreshold(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	m.info = UsageInfo{
		FiveHourPct: 0.95,
		SevenDayPct: 0.50,
	}

	limited, reason := m.IsRateLimited()
	if !limited {
		t.Error("expected rate limited due to 5-hour threshold")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestIsRateLimited_SevenDayExceedsThreshold(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	m.info = UsageInfo{
		FiveHourPct: 0.50,
		SevenDayPct: 0.98,
	}

	limited, reason := m.IsRateLimited()
	if !limited {
		t.Error("expected rate limited due to 7-day threshold")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestIsRateLimited_StaleDataPastResetTime(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	m.info = UsageInfo{
		FiveHourPct: 0.99,
		SevenDayPct: 0.99,
		ResetsAt:    time.Now().Add(-1 * time.Hour), // reset time is in the past
	}

	limited, reason := m.IsRateLimited()
	if limited {
		t.Errorf("expected not rate limited when past reset time, got limited with reason: %s", reason)
	}
}

func TestCurrentUsage_ReturnsStoredSnapshot(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	now := time.Now()
	resetTime := now.Add(2 * time.Hour)
	m.info = UsageInfo{
		FiveHourPct: 0.42,
		SevenDayPct: 0.73,
		ResetsAt:    resetTime,
		LastChecked: now,
	}

	usage := m.CurrentUsage()
	if usage.FiveHourPct != 0.42 {
		t.Errorf("expected FiveHourPct 0.42, got %f", usage.FiveHourPct)
	}
	if usage.SevenDayPct != 0.73 {
		t.Errorf("expected SevenDayPct 0.73, got %f", usage.SevenDayPct)
	}
	if !usage.ResetsAt.Equal(resetTime) {
		t.Errorf("expected ResetsAt %v, got %v", resetTime, usage.ResetsAt)
	}
	if !usage.LastChecked.Equal(now) {
		t.Errorf("expected LastChecked %v, got %v", now, usage.LastChecked)
	}
}

func TestResetsAt_ReturnsStoredResetTime(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	resetTime := time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC)
	m.info = UsageInfo{
		ResetsAt: resetTime,
	}

	got := m.ResetsAt()
	if !got.Equal(resetTime) {
		t.Errorf("expected ResetsAt %v, got %v", resetTime, got)
	}
}

func TestParseUsageJSON_FullResponse(t *testing.T) {
	input := []byte(`{
		"fetched_at": "2026-03-25T13:00:01Z",
		"fetched_epoch": 1774443601,
		"age_seconds": 101,
		"stale": false,
		"data": {
			"five_hour": {
				"utilization": 35.0,
				"resets_at": "2026-03-25T17:00:00.500386+00:00"
			},
			"seven_day": {
				"utilization": 59.0,
				"resets_at": "2026-03-27T07:00:00.500406+00:00"
			},
			"seven_day_opus": null,
			"seven_day_sonnet": null,
			"extra_usage": {
				"is_enabled": false,
				"monthly_limit": null,
				"used_credits": null,
				"utilization": null
			}
		}
	}`)

	info, err := parseUsageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantFiveHour := 0.35
	if info.FiveHourPct != wantFiveHour {
		t.Errorf("expected FiveHourPct %f, got %f", wantFiveHour, info.FiveHourPct)
	}
	wantSevenDay := 0.59
	if info.SevenDayPct != wantSevenDay {
		t.Errorf("expected SevenDayPct %f, got %f", wantSevenDay, info.SevenDayPct)
	}
	if info.ResetsAt.IsZero() {
		t.Error("expected ResetsAt to be set")
	}
	wantReset := time.Date(2026, 3, 25, 17, 0, 0, 500386000, time.UTC)
	if !info.ResetsAt.Equal(wantReset) {
		t.Errorf("expected ResetsAt %v, got %v", wantReset, info.ResetsAt)
	}
	if info.LastChecked.IsZero() {
		t.Error("expected LastChecked to be set")
	}
	if info.AgeSeconds != 101 {
		t.Errorf("expected AgeSeconds 101, got %d", info.AgeSeconds)
	}
	if info.Stale {
		t.Error("expected Stale to be false")
	}
}

func TestParseUsageJSON_StaleData(t *testing.T) {
	input := []byte(`{
		"fetched_at": "2026-03-25T12:00:00Z",
		"age_seconds": 1200,
		"stale": true,
		"data": {
			"five_hour": {
				"utilization": 50.0,
				"resets_at": "2026-03-25T17:00:00+00:00"
			},
			"seven_day": {
				"utilization": 40.0,
				"resets_at": "2026-03-27T07:00:00+00:00"
			}
		}
	}`)

	info, err := parseUsageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.AgeSeconds != 1200 {
		t.Errorf("expected AgeSeconds 1200, got %d", info.AgeSeconds)
	}
	if !info.Stale {
		t.Error("expected Stale to be true")
	}
}

func TestParseUsageJSON_NullWindows(t *testing.T) {
	input := []byte(`{
		"fetched_at": "2026-03-25T13:00:01Z",
		"data": {
			"five_hour": null,
			"seven_day": null
		}
	}`)

	info, err := parseUsageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.FiveHourPct != 0 {
		t.Errorf("expected FiveHourPct 0, got %f", info.FiveHourPct)
	}
	if info.SevenDayPct != 0 {
		t.Errorf("expected SevenDayPct 0, got %f", info.SevenDayPct)
	}
	if !info.ResetsAt.IsZero() {
		t.Errorf("expected zero ResetsAt, got %v", info.ResetsAt)
	}
}

func TestParseUsageJSON_InvalidJSON(t *testing.T) {
	_, err := parseUsageJSON([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseUsageJSON_ZeroUtilization(t *testing.T) {
	input := []byte(`{
		"data": {
			"five_hour": {
				"utilization": 0.0,
				"resets_at": "2026-03-25T17:00:00+00:00"
			},
			"seven_day": {
				"utilization": 0.0,
				"resets_at": "2026-03-27T07:00:00+00:00"
			}
		}
	}`)

	info, err := parseUsageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.FiveHourPct != 0 {
		t.Errorf("expected FiveHourPct 0, got %f", info.FiveHourPct)
	}
	if info.SevenDayPct != 0 {
		t.Errorf("expected SevenDayPct 0, got %f", info.SevenDayPct)
	}
}

func TestParseUsageJSON_HighUtilization(t *testing.T) {
	input := []byte(`{
		"data": {
			"five_hour": {
				"utilization": 95.5,
				"resets_at": "2026-03-25T17:00:00+00:00"
			},
			"seven_day": {
				"utilization": 100.0,
				"resets_at": "2026-03-27T07:00:00+00:00"
			}
		}
	}`)

	info, err := parseUsageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantFiveHour := 0.955
	if info.FiveHourPct != wantFiveHour {
		t.Errorf("expected FiveHourPct %f, got %f", wantFiveHour, info.FiveHourPct)
	}
	wantSevenDay := 1.0
	if info.SevenDayPct != wantSevenDay {
		t.Errorf("expected SevenDayPct %f, got %f", wantSevenDay, info.SevenDayPct)
	}
}

func TestIsRateLimited_NoData(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	// Zero-value info: no usage data yet
	limited, reason := m.IsRateLimited()
	if limited {
		t.Errorf("should not be rate limited with no data, got reason: %s", reason)
	}
}

func TestIsRateLimited_BothThresholdsExceeded(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	m.info = UsageInfo{
		FiveHourPct: 0.95,
		SevenDayPct: 0.99,
	}

	limited, reason := m.IsRateLimited()
	if !limited {
		t.Error("expected rate limited when both thresholds exceeded")
	}
	// The 5-hour check comes first in the code, so reason should mention 5-hour
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestIsRateLimited_ExactlyAtThreshold(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	// At the threshold but not exceeding (uses > not >=)
	m.info = UsageInfo{
		FiveHourPct: 0.90,
		SevenDayPct: 0.95,
	}

	limited, reason := m.IsRateLimited()
	if limited {
		t.Errorf("should not be rate limited at exact threshold (> not >=), got reason: %s", reason)
	}
}

func TestIsRateLimited_JustAboveThreshold(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	m.info = UsageInfo{
		FiveHourPct: 0.901,
		SevenDayPct: 0.50,
	}

	limited, _ := m.IsRateLimited()
	if !limited {
		t.Error("expected rate limited when just above 5-hour threshold")
	}
}

func TestIsRateLimited_FutureResetTimeNotIgnored(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	m.info = UsageInfo{
		FiveHourPct: 0.95,
		SevenDayPct: 0.50,
		ResetsAt:    time.Now().Add(1 * time.Hour), // reset in the future
	}

	limited, _ := m.IsRateLimited()
	if !limited {
		t.Error("expected rate limited when reset time is in the future")
	}
}

func TestParseUsageJSON_EmptyDataObject(t *testing.T) {
	input := []byte(`{"data": {}}`)

	info, err := parseUsageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.FiveHourPct != 0 {
		t.Errorf("expected FiveHourPct 0, got %f", info.FiveHourPct)
	}
	if info.SevenDayPct != 0 {
		t.Errorf("expected SevenDayPct 0, got %f", info.SevenDayPct)
	}
}

func TestParseUsageJSON_MissingResetsAt(t *testing.T) {
	input := []byte(`{
		"data": {
			"five_hour": {
				"utilization": 42.0
			},
			"seven_day": {
				"utilization": 10.0
			}
		}
	}`)

	info, err := parseUsageJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.FiveHourPct != 0.42 {
		t.Errorf("expected FiveHourPct 0.42, got %f", info.FiveHourPct)
	}
	if !info.ResetsAt.IsZero() {
		t.Errorf("expected zero ResetsAt when resets_at is missing, got %v", info.ResetsAt)
	}
}

func TestParseUsageJSON_EmptyBytes(t *testing.T) {
	_, err := parseUsageJSON([]byte{})
	if err == nil {
		t.Fatal("expected error for empty bytes, got nil")
	}
}

func TestCurrentUsage_ZeroValue(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	usage := m.CurrentUsage()
	if usage.FiveHourPct != 0 || usage.SevenDayPct != 0 {
		t.Errorf("expected zero usage, got five_hour=%f seven_day=%f", usage.FiveHourPct, usage.SevenDayPct)
	}
	if !usage.ResetsAt.IsZero() {
		t.Errorf("expected zero ResetsAt, got %v", usage.ResetsAt)
	}
}

func TestResetsAt_ZeroValue(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95)
	got := m.ResetsAt()
	if !got.IsZero() {
		t.Errorf("expected zero ResetsAt, got %v", got)
	}
}
