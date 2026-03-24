// Package runner implements the core scheduling loop and task execution.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

const defaultAPIEndpoint = "https://api.anthropic.com/api/oauth/usage"

// UsageInfo holds the current usage state returned by CurrentUsage.
type UsageInfo struct {
	FiveHourPct float64   `json:"five_hour_pct"`
	SevenDayPct float64   `json:"seven_day_pct"`
	ResetsAt    time.Time `json:"resets_at"`
	LastChecked time.Time `json:"last_checked"`
}

// UsageMonitor polls the Anthropic OAuth usage API to track rate limits.
// It is safe for concurrent use.
type UsageMonitor struct {
	credPath    string
	threshold5h float64
	threshold7d float64
	interval    time.Duration
	endpoint    string

	mu   sync.RWMutex
	info UsageInfo

	cancel context.CancelFunc
	done   chan struct{}
}

// NewUsageMonitor creates a new usage monitor. If apiEndpoint is empty,
// the default Anthropic API endpoint is used.
func NewUsageMonitor(
	credPath string, threshold5h, threshold7d float64,
	pollInterval time.Duration, apiEndpoint string,
) *UsageMonitor {
	if apiEndpoint == "" {
		apiEndpoint = defaultAPIEndpoint
	}
	return &UsageMonitor{
		credPath:    credPath,
		threshold5h: threshold5h,
		threshold7d: threshold7d,
		interval:    pollInterval,
		endpoint:    apiEndpoint,
	}
}

// Start begins polling in a background goroutine. It performs an immediate
// poll, then polls at an adaptive interval based on current usage levels.
// Cancelling ctx or calling Stop will terminate the goroutine.
func (m *UsageMonitor) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	m.done = make(chan struct{})

	go func() {
		defer close(m.done)

		m.Poll()

		for {
			delay := m.adaptiveInterval()
			slog.Debug("usage monitor: next poll", "delay", delay)
			timer := time.NewTimer(delay)

			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				m.Poll()
			}
		}
	}()
}

// adaptiveInterval returns the next poll delay based on current usage levels.
// Lower usage means longer cache duration to minimize API calls.
func (m *UsageMonitor) adaptiveInterval() time.Duration {
	m.mu.RLock()
	maxPct := max(m.info.FiveHourPct, m.info.SevenDayPct)
	m.mu.RUnlock()

	switch {
	case maxPct < 0.50:
		return 60 * time.Minute
	case maxPct < 0.70:
		return 30 * time.Minute
	default:
		return m.interval
	}
}

// Stop stops the polling goroutine and waits for it to finish.
func (m *UsageMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.done != nil {
		<-m.done
	}
}

// IsRateLimited returns whether usage exceeds either threshold, and a reason string.
// The reason is empty when not rate limited. If the cached data is stale
// (past the known reset time), it is treated as expired and tasks are allowed.
func (m *UsageMonitor) IsRateLimited() (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// If the 5-hour window has reset since our last successful poll,
	// assume usage has dropped — don't block on stale data.
	if !m.info.ResetsAt.IsZero() && time.Now().After(m.info.ResetsAt) {
		return false, ""
	}

	if m.info.FiveHourPct > m.threshold5h {
		return true, fmt.Sprintf("5-hour utilization %.0f%% > %.0f%% threshold", m.info.FiveHourPct*100, m.threshold5h*100)
	}
	if m.info.SevenDayPct > m.threshold7d {
		return true, fmt.Sprintf("7-day utilization %.0f%% > %.0f%% threshold", m.info.SevenDayPct*100, m.threshold7d*100)
	}
	return false, ""
}

// CurrentUsage returns a snapshot of the current usage state.
func (m *UsageMonitor) CurrentUsage() UsageInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.info
}

// ResetsAt returns when the current 5-hour rate limit window resets.
func (m *UsageMonitor) ResetsAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.info.ResetsAt
}

// Poll fetches usage data from the API and updates the stored state.
// It is called automatically by the background goroutine.
func (m *UsageMonitor) Poll() {
	token := m.readToken()
	if token == "" {
		slog.Warn("usage monitor: no access token found, skipping poll")
		return
	}

	resp, err := m.fetchUsage(token)
	if err != nil {
		if errors.Is(err, errTooManyRequests) {
			slog.Warn("usage monitor: rate limited by API, using cached data")
		} else {
			slog.Error("usage monitor: fetch failed", "error", err)
		}
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.info = *resp
}

// credentialsFile represents the structure of Claude's credentials JSON.
type credentialsFile struct {
	ClaudeAiOauth struct {
		Token string `json:"accessToken"` //nolint:gosec // JSON field name, not a hardcoded credential
	} `json:"claudeAiOauth"`
}

// readToken reads the access token from the credentials file.
func (m *UsageMonitor) readToken() string {
	data, err := os.ReadFile(m.credPath)
	if err != nil {
		slog.Warn("usage monitor: cannot read credentials", "path", m.credPath, "error", err)
		return ""
	}
	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		slog.Warn("usage monitor: cannot parse credentials", "error", err)
		return ""
	}
	return creds.ClaudeAiOauth.Token
}

// apiResponse represents the Anthropic usage API response.
type apiResponse struct {
	FiveHour struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization float64 `json:"utilization"`
	} `json:"seven_day"`
}

// fetchUsage calls the Anthropic usage API. On a 401 response, it re-reads
// the credentials file and retries once (Claude CLI may have refreshed the token).
func (m *UsageMonitor) fetchUsage(token string) (*UsageInfo, error) {
	info, err := m.doFetch(token)
	if errors.Is(err, errUnauthorized) {
		// Re-read credentials and retry once
		newToken := m.readToken()
		if newToken == "" || newToken == token {
			return nil, fmt.Errorf("unauthorized and no new token available")
		}
		slog.Info("usage monitor: retrying with refreshed token")
		return m.doFetch(newToken)
	}
	return info, err
}

var (
	errUnauthorized    = errors.New("401 unauthorized")
	errTooManyRequests = errors.New("429 too many requests")
)

// doFetch performs a single API request.
func (m *UsageMonitor) doFetch(token string) (*UsageInfo, error) {
	req, err := http.NewRequest(http.MethodGet, m.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req) //nolint:gosec // endpoint is set from config, not user input
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errUnauthorized
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errTooManyRequests
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var resetsAt time.Time
	if apiResp.FiveHour.ResetsAt != "" {
		resetsAt, err = time.Parse(time.RFC3339, apiResp.FiveHour.ResetsAt)
		if err != nil {
			slog.Warn("usage monitor: cannot parse resets_at", "value", apiResp.FiveHour.ResetsAt, "error", err)
		}
	}

	return &UsageInfo{
		FiveHourPct: apiResp.FiveHour.Utilization / 100.0,
		SevenDayPct: apiResp.SevenDay.Utilization / 100.0,
		ResetsAt:    resetsAt,
		LastChecked: time.Now(),
	}, nil
}
