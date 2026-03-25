// Package runner implements the core scheduling loop and task execution.
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

// UsageInfo holds the current usage state returned by CurrentUsage.
type UsageInfo struct {
	FiveHourPct float64   `json:"five_hour_pct"`
	SevenDayPct float64   `json:"seven_day_pct"`
	ResetsAt    time.Time `json:"resets_at"`
	LastChecked time.Time `json:"last_checked"`
}

// UsageMonitor polls the claude-usage command to track rate limits.
// It is safe for concurrent use.
type UsageMonitor struct {
	cmdPath     string
	threshold5h float64
	threshold7d float64
	interval    time.Duration

	mu   sync.RWMutex
	info UsageInfo

	cancel context.CancelFunc
	done   chan struct{}
}

// NewUsageMonitor creates a new usage monitor that shells out to cmdPath
// to fetch usage data.
func NewUsageMonitor(
	cmdPath string, threshold5h, threshold7d float64,
	pollInterval time.Duration,
) *UsageMonitor {
	return &UsageMonitor{
		cmdPath:     cmdPath,
		threshold5h: threshold5h,
		threshold7d: threshold7d,
		interval:    pollInterval,
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
// Lower usage means longer intervals since the data changes slowly.
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

// claudeUsageResponse represents the JSON output of the claude-usage command.
type claudeUsageResponse struct {
	Data struct {
		FiveHour *struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay *struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"seven_day"`
	} `json:"data"`
}

// Poll executes the claude-usage command and updates the stored state.
// It is called automatically by the background goroutine.
func (m *UsageMonitor) Poll() {
	info, err := m.fetchUsage()
	if err != nil {
		slog.Error("usage monitor: fetch failed", "error", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.info = *info
}

// fetchUsage runs the claude-usage command and parses its JSON output.
func (m *UsageMonitor) fetchUsage() (*UsageInfo, error) {
	out, err := exec.Command(m.cmdPath).Output() //nolint:gosec // cmdPath is set from config
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", m.cmdPath, err)
	}
	return parseUsageJSON(out)
}

// parseUsageJSON parses the JSON output from the claude-usage command
// into a UsageInfo. Exported for testing.
func parseUsageJSON(data []byte) (*UsageInfo, error) {
	var resp claudeUsageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse usage JSON: %w", err)
	}

	info := &UsageInfo{
		LastChecked: time.Now(),
	}

	if resp.Data.FiveHour != nil {
		info.FiveHourPct = resp.Data.FiveHour.Utilization / 100.0
		if resp.Data.FiveHour.ResetsAt != "" {
			t, err := time.Parse(time.RFC3339Nano, resp.Data.FiveHour.ResetsAt)
			if err != nil {
				slog.Warn("usage monitor: cannot parse five_hour resets_at", "value", resp.Data.FiveHour.ResetsAt, "error", err)
			} else {
				info.ResetsAt = t
			}
		}
	}

	if resp.Data.SevenDay != nil {
		info.SevenDayPct = resp.Data.SevenDay.Utilization / 100.0
	}

	return info, nil
}
