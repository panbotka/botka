package runner

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewUsageMonitor_DefaultEndpoint(t *testing.T) {
	m := NewUsageMonitor("/tmp/creds.json", 0.90, 0.95, 15*time.Minute, "")
	if m.endpoint != defaultAPIEndpoint {
		t.Errorf("expected default endpoint %q, got %q", defaultAPIEndpoint, m.endpoint)
	}
}

func TestNewUsageMonitor_CustomEndpoint(t *testing.T) {
	custom := "https://custom.example.com/usage"
	m := NewUsageMonitor("/tmp/creds.json", 0.90, 0.95, 15*time.Minute, custom)
	if m.endpoint != custom {
		t.Errorf("expected custom endpoint %q, got %q", custom, m.endpoint)
	}
}

func TestIsRateLimited_BelowBothThresholds(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, "")
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
	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, "")
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
	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, "")
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
	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, "")
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
	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, "")
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
	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, "")
	resetTime := time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC)
	m.info = UsageInfo{
		ResetsAt: resetTime,
	}

	got := m.ResetsAt()
	if !got.Equal(resetTime) {
		t.Errorf("expected ResetsAt %v, got %v", resetTime, got)
	}
}

func TestAdaptiveInterval_LowUsage(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, "")
	m.info = UsageInfo{
		FiveHourPct: 0.20,
		SevenDayPct: 0.30,
	}

	got := m.adaptiveInterval()
	want := 60 * time.Minute
	if got != want {
		t.Errorf("expected %v for low usage (<50%%), got %v", want, got)
	}
}

func TestAdaptiveInterval_MediumUsage(t *testing.T) {
	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, "")
	m.info = UsageInfo{
		FiveHourPct: 0.55,
		SevenDayPct: 0.60,
	}

	got := m.adaptiveInterval()
	want := 30 * time.Minute
	if got != want {
		t.Errorf("expected %v for medium usage (50-70%%), got %v", want, got)
	}
}

func TestAdaptiveInterval_HighUsage(t *testing.T) {
	configured := 10 * time.Minute
	m := NewUsageMonitor("", 0.90, 0.95, configured, "")
	m.info = UsageInfo{
		FiveHourPct: 0.85,
		SevenDayPct: 0.40,
	}

	got := m.adaptiveInterval()
	if got != configured {
		t.Errorf("expected configured interval %v for high usage (>=70%%), got %v", configured, got)
	}
}

func TestDoFetch_SuccessfulResponse(t *testing.T) {
	resetTime := time.Date(2026, 3, 25, 18, 30, 0, 0, time.UTC)
	resp := apiResponse{}
	resp.FiveHour.Utilization = 45.5
	resp.FiveHour.ResetsAt = resetTime.Format(time.RFC3339)
	resp.SevenDay.Utilization = 72.3

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Authorization 'Bearer test-token', got %q", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != "oauth-2025-04-20" {
			t.Errorf("expected anthropic-beta header 'oauth-2025-04-20', got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, srv.URL)
	info, err := m.doFetch("test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Utilization is divided by 100 in doFetch
	wantFiveHour := 0.455
	if info.FiveHourPct != wantFiveHour {
		t.Errorf("expected FiveHourPct %f, got %f", wantFiveHour, info.FiveHourPct)
	}
	wantSevenDay := 0.723
	if info.SevenDayPct != wantSevenDay {
		t.Errorf("expected SevenDayPct %f, got %f", wantSevenDay, info.SevenDayPct)
	}
	if !info.ResetsAt.Equal(resetTime) {
		t.Errorf("expected ResetsAt %v, got %v", resetTime, info.ResetsAt)
	}
	if info.LastChecked.IsZero() {
		t.Error("expected LastChecked to be set")
	}
}

func TestDoFetch_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, srv.URL)
	_, err := m.doFetch("bad-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errUnauthorized) {
		t.Errorf("expected errUnauthorized, got %v", err)
	}
}

func TestDoFetch_TooManyRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, srv.URL)
	_, err := m.doFetch("some-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errTooManyRequests) {
		t.Errorf("expected errTooManyRequests, got %v", err)
	}
}

func TestDoFetch_InternalServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := NewUsageMonitor("", 0.90, 0.95, 15*time.Minute, srv.URL)
	_, err := m.doFetch("some-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, errUnauthorized) || errors.Is(err, errTooManyRequests) {
		t.Errorf("expected generic error, got specific sentinel: %v", err)
	}
}
