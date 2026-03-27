package runner

import (
	"strings"
	"testing"
	"time"

	"botka/internal/models"

	"github.com/google/uuid"
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

func TestTruncate_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"zero max length truncates everything", "hello", 0, "..."},
		{"single char max", "hello", 1, "h..."},
		{"one over max", "abcdef", 5, "abcde..."},
		{"max length of 500 (maxErrLen)", strings.Repeat("x", 600), 500, strings.Repeat("x", 500) + "..."},
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

func TestClassifyOutcome_SuccessNoLastText(t *testing.T) {
	out := &spawnOutput{
		exitCode: 0,
		lastResult: &Event{
			Type:       EventResult,
			CostUSD:    0.10,
			DurationMs: 20000,
		},
		lastText: "",
	}
	task := &models.Task{RetryCount: 0}

	r := classifyOutcome(out, task)

	if r.Status != models.TaskStatusDone {
		t.Errorf("status = %v, want %v", r.Status, models.TaskStatusDone)
	}
	if r.Summary != "" {
		t.Errorf("Summary = %q, want empty", r.Summary)
	}
}

func TestClassifyOutcome_CrashMaxRetries(t *testing.T) {
	out := &spawnOutput{
		exitCode: 1,
		stderr:   "unknown crash",
	}
	task := &models.Task{RetryCount: 1}

	r := classifyOutcome(out, task)

	if r.Status != models.TaskStatusFailed {
		t.Errorf("status = %v, want %v", r.Status, models.TaskStatusFailed)
	}
	if r.ShouldRetry {
		t.Error("ShouldRetry = true, want false (RetryCount=1 == maxRetries)")
	}
	if !strings.Contains(r.ErrorMessage, "crashed") {
		t.Errorf("ErrorMessage = %q, want it to contain 'crashed'", r.ErrorMessage)
	}
}

func TestClassifyOutcome_TimeoutExhaustsRetries(t *testing.T) {
	out := &spawnOutput{timedOut: true}
	task := &models.Task{RetryCount: 1}

	r := classifyOutcome(out, task)

	if r.ShouldRetry {
		t.Error("ShouldRetry = true, want false (RetryCount == maxRetries)")
	}
	if r.ErrorMessage != "execution timed out" {
		t.Errorf("ErrorMessage = %q, want %q", r.ErrorMessage, "execution timed out")
	}
}

func TestBuildFailureResult_NilResultFieldsZero(t *testing.T) {
	out := &spawnOutput{
		stderr:   "something went wrong",
		lastText: "partial output",
		lastResult: &Event{
			Type: EventResult,
			// CostUSD and DurationMs default to zero
		},
	}
	task := &models.Task{RetryCount: 0}

	r := buildFailureResult(out, task)

	if r.CostUSD != 0 {
		t.Errorf("CostUSD = %v, want 0", r.CostUSD)
	}
	if r.DurationMs != 0 {
		t.Errorf("DurationMs = %v, want 0", r.DurationMs)
	}
}

func TestBuildFailureResult_LongStderrTruncated(t *testing.T) {
	longStderr := strings.Repeat("error ", 200) // ~1200 chars
	out := &spawnOutput{
		stderr:     longStderr,
		lastText:   "output",
		lastResult: &Event{Type: EventResult},
	}
	task := &models.Task{RetryCount: 0}

	r := buildFailureResult(out, task)

	if len(r.ErrorMessage) > maxErrLen+10 {
		t.Errorf("ErrorMessage length = %d, expected <= %d (maxErrLen + ellipsis)", len(r.ErrorMessage), maxErrLen+10)
	}
	if !strings.HasSuffix(r.ErrorMessage, "...") {
		t.Errorf("ErrorMessage should end with '...', got %q", r.ErrorMessage[len(r.ErrorMessage)-10:])
	}
}

func TestIsAPIError_AllPatterns(t *testing.T) {
	// Ensure each documented pattern is individually matched.
	patterns := []string{"500", "502", "503", "529", "overloaded", "rate_limit", "capacity"}
	for _, p := range patterns {
		if !isAPIError(p) {
			t.Errorf("isAPIError(%q) = false, want true", p)
		}
	}
}

func TestIsAPIError_CaseInsensitive(t *testing.T) {
	tests := []string{"OVERLOADED", "Overloaded", "RATE_LIMIT", "Rate_Limit", "CAPACITY", "Capacity"}
	for _, input := range tests {
		if !isAPIError(input) {
			t.Errorf("isAPIError(%q) = false, want true (case insensitive)", input)
		}
	}
}

func TestBuildPrompt_Basic(t *testing.T) {
	e := &Executor{claudePath: "/usr/bin/claude"}
	task := &models.Task{RetryCount: 0}
	task.ID = parseUUID("11111111-1111-1111-1111-111111111111")
	task.Title = "Fix login bug"

	prompt := e.buildPrompt(task)

	if !strings.Contains(prompt, "Fix login bug") {
		t.Error("prompt should contain task title")
	}
	if !strings.Contains(prompt, "task-11111111-1111-1111-1111-111111111111.md") {
		t.Error("prompt should reference the spec file path")
	}
	if !strings.Contains(prompt, "NEVER run deploy") {
		t.Error("prompt should contain safety warning")
	}
}

func TestBuildPrompt_WithRetryInfo(t *testing.T) {
	e := &Executor{claudePath: "/usr/bin/claude"}
	failReason := "tests failed: 2 errors"
	task := &models.Task{
		RetryCount:    1,
		FailureReason: &failReason,
	}
	task.ID = parseUUID("22222222-2222-2222-2222-222222222222")
	task.Title = "Add feature"

	prompt := e.buildPrompt(task)

	if !strings.Contains(prompt, "Previous attempt failed") {
		t.Error("prompt should mention previous failure")
	}
	if !strings.Contains(prompt, "tests failed: 2 errors") {
		t.Error("prompt should contain the failure reason")
	}
}

func TestBuildPrompt_RetryCountZeroNoFailureInfo(t *testing.T) {
	e := &Executor{claudePath: "/usr/bin/claude"}
	task := &models.Task{RetryCount: 0}
	task.ID = parseUUID("33333333-3333-3333-3333-333333333333")
	task.Title = "First attempt"

	prompt := e.buildPrompt(task)

	if strings.Contains(prompt, "Previous attempt") {
		t.Error("prompt should not mention previous failure on first attempt")
	}
}

// parseUUID is a test helper to create a uuid.UUID from a string.
func parseUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}
