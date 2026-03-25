package runner

import (
	"strings"
	"testing"
	"time"

	"botka/internal/models"
)

func TestClassifyOutcome_Timeout(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int
		wantRetry  bool
	}{
		{"retry when RetryCount=0", 0, true},
		{"no retry at maxRetries", 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &spawnOutput{timedOut: true}
			task := &models.Task{RetryCount: tt.retryCount}

			r := classifyOutcome(out, task)

			if r.Status != models.TaskStatusFailed {
				t.Errorf("status = %v, want %v", r.Status, models.TaskStatusFailed)
			}
			if r.ErrorMessage != "execution timed out" {
				t.Errorf("error = %q, want %q", r.ErrorMessage, "execution timed out")
			}
			if r.ShouldRetry != tt.wantRetry {
				t.Errorf("ShouldRetry = %v, want %v", r.ShouldRetry, tt.wantRetry)
			}
		})
	}
}

func TestClassifyOutcome_Success(t *testing.T) {
	out := &spawnOutput{
		exitCode: 0,
		lastResult: &Event{
			Type:       EventResult,
			CostUSD:    0.05,
			DurationMs: 12000,
			IsError:    false,
		},
		lastText: "All changes committed.",
	}
	task := &models.Task{RetryCount: 0}

	r := classifyOutcome(out, task)

	if r.Status != models.TaskStatusDone {
		t.Errorf("status = %v, want %v", r.Status, models.TaskStatusDone)
	}
	if r.CostUSD != 0.05 {
		t.Errorf("CostUSD = %v, want %v", r.CostUSD, 0.05)
	}
	if r.DurationMs != 12000 {
		t.Errorf("DurationMs = %v, want %v", r.DurationMs, 12000)
	}
	if r.Summary != "All changes committed." {
		t.Errorf("Summary = %q, want %q", r.Summary, "All changes committed.")
	}
	if r.ShouldRetry {
		t.Error("ShouldRetry = true, want false")
	}
	if r.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", r.ErrorMessage)
	}
}

func TestClassifyOutcome_APIError(t *testing.T) {
	out := &spawnOutput{
		exitCode: 1,
		stderr:   "Error: 529 overloaded",
		lastResult: &Event{
			Type:       EventResult,
			CostUSD:    0.01,
			DurationMs: 5000,
		},
		lastText: "overloaded response",
	}
	task := &models.Task{RetryCount: 0}

	r := classifyOutcome(out, task)

	if r.Status != models.TaskStatusFailed {
		t.Errorf("status = %v, want %v", r.Status, models.TaskStatusFailed)
	}
	if r.RetryAfter != time.Hour {
		t.Errorf("RetryAfter = %v, want %v", r.RetryAfter, time.Hour)
	}
	if r.ShouldRetry {
		t.Error("ShouldRetry should be false for API errors (uses RetryAfter instead)")
	}
	if r.CostUSD != 0.01 {
		t.Errorf("CostUSD = %v, want %v", r.CostUSD, 0.01)
	}
	if !strings.Contains(r.ErrorMessage, "API error") {
		t.Errorf("ErrorMessage = %q, want it to contain 'API error'", r.ErrorMessage)
	}
}

func TestClassifyOutcome_CrashNoResult(t *testing.T) {
	out := &spawnOutput{
		exitCode: 1,
		stderr:   "segfault",
	}
	task := &models.Task{RetryCount: 0}

	r := classifyOutcome(out, task)

	if r.Status != models.TaskStatusFailed {
		t.Errorf("status = %v, want %v", r.Status, models.TaskStatusFailed)
	}
	if !strings.Contains(r.ErrorMessage, "crashed") {
		t.Errorf("ErrorMessage = %q, want it to contain 'crashed'", r.ErrorMessage)
	}
	if !r.ShouldRetry {
		t.Error("ShouldRetry = false, want true (RetryCount=0 < maxRetries)")
	}
}

func TestClassifyOutcome_CrashWithAPIPattern(t *testing.T) {
	out := &spawnOutput{
		exitCode: 1,
		stderr:   "503 service unavailable",
	}
	task := &models.Task{RetryCount: 0}

	r := classifyOutcome(out, task)

	if r.Status != models.TaskStatusFailed {
		t.Errorf("status = %v, want %v", r.Status, models.TaskStatusFailed)
	}
	if r.RetryAfter != time.Hour {
		t.Errorf("RetryAfter = %v, want %v", r.RetryAfter, time.Hour)
	}
	if !strings.Contains(r.ErrorMessage, "API error") {
		t.Errorf("ErrorMessage = %q, want it to contain 'API error'", r.ErrorMessage)
	}
	if r.ShouldRetry {
		t.Error("ShouldRetry should be false for API crash (uses RetryAfter)")
	}
}

func TestClassifyOutcome_NonZeroExitWithResult(t *testing.T) {
	out := &spawnOutput{
		exitCode: 1,
		stderr:   "some error occurred",
		lastResult: &Event{
			Type:       EventResult,
			CostUSD:    0.03,
			DurationMs: 8000,
			IsError:    false,
		},
		lastText: "partial output",
	}
	task := &models.Task{RetryCount: 0}

	r := classifyOutcome(out, task)

	if r.Status != models.TaskStatusFailed {
		t.Errorf("status = %v, want %v", r.Status, models.TaskStatusFailed)
	}
	if r.ErrorMessage != "some error occurred" {
		t.Errorf("ErrorMessage = %q, want %q", r.ErrorMessage, "some error occurred")
	}
	if !r.ShouldRetry {
		t.Error("ShouldRetry = false, want true")
	}
}

func TestClassifyOutcome_ResultIsError(t *testing.T) {
	out := &spawnOutput{
		exitCode: 0,
		stderr:   "tool failed",
		lastResult: &Event{
			Type:       EventResult,
			CostUSD:    0.02,
			DurationMs: 3000,
			IsError:    true,
		},
		lastText: "error output",
	}
	task := &models.Task{RetryCount: 1}

	r := classifyOutcome(out, task)

	if r.Status != models.TaskStatusFailed {
		t.Errorf("status = %v, want %v", r.Status, models.TaskStatusFailed)
	}
	if r.ShouldRetry {
		t.Error("ShouldRetry = true, want false (RetryCount=1 == maxRetries)")
	}
}

func TestBuildFailureResult(t *testing.T) {
	tests := []struct {
		name       string
		stderr     string
		lastText   string
		retryCount int
		wantErr    string
		wantRetry  bool
	}{
		{
			name:       "with stderr",
			stderr:     "compilation failed",
			lastText:   "some output",
			retryCount: 0,
			wantErr:    "compilation failed",
			wantRetry:  true,
		},
		{
			name:       "empty stderr uses default message",
			stderr:     "",
			lastText:   "output before crash",
			retryCount: 0,
			wantErr:    "claude process exited with error",
			wantRetry:  true,
		},
		{
			name:       "retry when RetryCount < maxRetries",
			stderr:     "error",
			lastText:   "",
			retryCount: 0,
			wantErr:    "error",
			wantRetry:  true,
		},
		{
			name:       "no retry at maxRetries",
			stderr:     "error",
			lastText:   "",
			retryCount: 1,
			wantErr:    "error",
			wantRetry:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &spawnOutput{
				stderr:   tt.stderr,
				lastText: tt.lastText,
				lastResult: &Event{
					Type:       EventResult,
					CostUSD:    0.04,
					DurationMs: 7000,
				},
			}
			task := &models.Task{RetryCount: tt.retryCount}

			r := buildFailureResult(out, task)

			if r.Status != models.TaskStatusFailed {
				t.Errorf("status = %v, want %v", r.Status, models.TaskStatusFailed)
			}
			if r.ErrorMessage != tt.wantErr {
				t.Errorf("ErrorMessage = %q, want %q", r.ErrorMessage, tt.wantErr)
			}
			if r.ShouldRetry != tt.wantRetry {
				t.Errorf("ShouldRetry = %v, want %v", r.ShouldRetry, tt.wantRetry)
			}
			if r.Summary != tt.lastText {
				t.Errorf("Summary = %q, want %q", r.Summary, tt.lastText)
			}
			if r.CostUSD != 0.04 {
				t.Errorf("CostUSD = %v, want %v", r.CostUSD, 0.04)
			}
			if r.DurationMs != 7000 {
				t.Errorf("DurationMs = %v, want %v", r.DurationMs, 7000)
			}
		})
	}
}

func TestIsAPIError(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"HTTP 500 internal server error", true},
		{"bad gateway 502", true},
		{"503 service unavailable", true},
		{"error code 529", true},
		{"server is overloaded", true},
		{"rate_limit exceeded", true},
		{"at capacity", true},
		{"Overloaded server response", true}, // case insensitive
		{"everything is fine", false},
		{"error code 404", false},
		{"", false},
		{"normal output with exit code 1", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isAPIError(tt.input)
			if got != tt.want {
				t.Errorf("isAPIError(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsSuccessful(t *testing.T) {
	tests := []struct {
		status models.TaskStatus
		want   bool
	}{
		{models.TaskStatusDone, true},
		{models.TaskStatusNeedsReview, true},
		{models.TaskStatusFailed, false},
		{models.TaskStatusPending, false},
		{models.TaskStatusQueued, false},
		{models.TaskStatusRunning, false},
		{models.TaskStatusCancelled, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := isSuccessful(tt.status)
			if got != tt.want {
				t.Errorf("isSuccessful(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string unchanged",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length unchanged",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "long string truncated with ellipsis",
			input:  "hello world",
			maxLen: 5,
			want:   "hello...",
		},
		{
			name:   "empty string unchanged",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
