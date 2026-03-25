package config

import (
	"testing"
	"time"
)

// --- getEnv tests ---

func TestGetEnv_SetValue(t *testing.T) {
	t.Setenv("TEST_GET_ENV", "custom_value")
	got := getEnv("TEST_GET_ENV", "default")
	if got != "custom_value" {
		t.Errorf("getEnv() = %q, want %q", got, "custom_value")
	}
}

func TestGetEnv_UnsetReturnsFallback(t *testing.T) {
	// TEST_GET_ENV_UNSET is not set by t.Setenv, so it should be empty
	got := getEnv("TEST_GET_ENV_UNSET_"+t.Name(), "fallback_val")
	if got != "fallback_val" {
		t.Errorf("getEnv() = %q, want %q", got, "fallback_val")
	}
}

func TestGetEnv_EmptyReturnsFallback(t *testing.T) {
	t.Setenv("TEST_GET_ENV_EMPTY", "")
	got := getEnv("TEST_GET_ENV_EMPTY", "default")
	if got != "default" {
		t.Errorf("getEnv() = %q, want %q", got, "default")
	}
}

// --- getEnvInt tests ---

func TestGetEnvInt_ValidInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	got, err := getEnvInt("TEST_INT", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Errorf("getEnvInt() = %d, want %d", got, 42)
	}
}

func TestGetEnvInt_UnsetReturnsFallback(t *testing.T) {
	got, err := getEnvInt("TEST_INT_UNSET_"+t.Name(), 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 99 {
		t.Errorf("getEnvInt() = %d, want %d", got, 99)
	}
}

func TestGetEnvInt_InvalidReturnsError(t *testing.T) {
	t.Setenv("TEST_INT_BAD", "not_a_number")
	_, err := getEnvInt("TEST_INT_BAD", 0)
	if err == nil {
		t.Fatal("expected error for invalid int, got nil")
	}
}

// --- getEnvFloat tests ---

func TestGetEnvFloat_ValidFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT", "0.75")
	got, err := getEnvFloat("TEST_FLOAT", 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0.75 {
		t.Errorf("getEnvFloat() = %f, want %f", got, 0.75)
	}
}

func TestGetEnvFloat_UnsetReturnsFallback(t *testing.T) {
	got, err := getEnvFloat("TEST_FLOAT_UNSET_"+t.Name(), 1.23)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1.23 {
		t.Errorf("getEnvFloat() = %f, want %f", got, 1.23)
	}
}

func TestGetEnvFloat_InvalidReturnsError(t *testing.T) {
	t.Setenv("TEST_FLOAT_BAD", "abc")
	_, err := getEnvFloat("TEST_FLOAT_BAD", 0)
	if err == nil {
		t.Fatal("expected error for invalid float, got nil")
	}
}

// --- getEnvBool tests ---

func TestGetEnvBool_True(t *testing.T) {
	t.Setenv("TEST_BOOL", "true")
	got, err := getEnvBool("TEST_BOOL", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != true {
		t.Errorf("getEnvBool() = %v, want true", got)
	}
}

func TestGetEnvBool_False(t *testing.T) {
	t.Setenv("TEST_BOOL", "false")
	got, err := getEnvBool("TEST_BOOL", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != false {
		t.Errorf("getEnvBool() = %v, want false", got)
	}
}

func TestGetEnvBool_UnsetReturnsFallback(t *testing.T) {
	got, err := getEnvBool("TEST_BOOL_UNSET_"+t.Name(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != true {
		t.Errorf("getEnvBool() = %v, want true", got)
	}
}

func TestGetEnvBool_InvalidReturnsError(t *testing.T) {
	t.Setenv("TEST_BOOL_BAD", "maybe")
	_, err := getEnvBool("TEST_BOOL_BAD", false)
	if err == nil {
		t.Fatal("expected error for invalid bool, got nil")
	}
}

// --- getEnvCSV tests ---

func TestGetEnvCSV_CommaSeparated(t *testing.T) {
	t.Setenv("TEST_CSV", "a,b,c")
	got := getEnvCSV("TEST_CSV", nil)
	want := []string{"a", "b", "c"}
	if !sliceEqual(got, want) {
		t.Errorf("getEnvCSV() = %v, want %v", got, want)
	}
}

func TestGetEnvCSV_WhitespaceTrimmed(t *testing.T) {
	t.Setenv("TEST_CSV", " a , b ")
	got := getEnvCSV("TEST_CSV", nil)
	want := []string{"a", "b"}
	if !sliceEqual(got, want) {
		t.Errorf("getEnvCSV() = %v, want %v", got, want)
	}
}

func TestGetEnvCSV_UnsetReturnsFallback(t *testing.T) {
	fallback := []string{"x", "y"}
	got := getEnvCSV("TEST_CSV_UNSET_"+t.Name(), fallback)
	if !sliceEqual(got, fallback) {
		t.Errorf("getEnvCSV() = %v, want %v", got, fallback)
	}
}

func TestGetEnvCSV_EmptyElementsSkipped(t *testing.T) {
	t.Setenv("TEST_CSV", "a,,b")
	got := getEnvCSV("TEST_CSV", nil)
	want := []string{"a", "b"}
	if !sliceEqual(got, want) {
		t.Errorf("getEnvCSV() = %v, want %v", got, want)
	}
}

// --- stripQuotes tests ---

func TestStripQuotes_DoubleQuotes(t *testing.T) {
	got := stripQuotes(`"hello"`)
	if got != "hello" {
		t.Errorf("stripQuotes() = %q, want %q", got, "hello")
	}
}

func TestStripQuotes_SingleQuotes(t *testing.T) {
	got := stripQuotes(`'hello'`)
	if got != "hello" {
		t.Errorf("stripQuotes() = %q, want %q", got, "hello")
	}
}

func TestStripQuotes_NoQuotes(t *testing.T) {
	got := stripQuotes("hello")
	if got != "hello" {
		t.Errorf("stripQuotes() = %q, want %q", got, "hello")
	}
}

func TestStripQuotes_MismatchedQuotes(t *testing.T) {
	got := stripQuotes(`"hello'`)
	if got != `"hello'` {
		t.Errorf("stripQuotes() = %q, want %q", got, `"hello'`)
	}
}

func TestStripQuotes_SingleChar(t *testing.T) {
	got := stripQuotes(`"`)
	if got != `"` {
		t.Errorf("stripQuotes() = %q, want %q", got, `"`)
	}
}

// --- Load tests ---

func TestLoad_Defaults(t *testing.T) {
	// Clear all config-relevant env vars to ensure defaults are used.
	envVars := []string{
		"PORT", "DATABASE_URL", "PROJECTS_DIR", "CLAUDE_PATH",
		"CLAUDE_CREDENTIALS_PATH", "MAX_WORKERS", "USAGE_POLL_INTERVAL",
		"USAGE_THRESHOLD_5H", "USAGE_THRESHOLD_7D", "OPENCLAW_URL",
		"OPENCLAW_TOKEN", "OPENCLAW_WORKSPACE", "CLAUDE_CONTEXT_DIR",
		"CLAUDE_DEFAULT_WORK_DIR", "WHISPER_ENABLED", "UPLOAD_DIR",
		"AI_MODEL", "AVAILABLE_MODELS",
	}
	for _, key := range envVars {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Port != "5110" {
		t.Errorf("Port = %q, want %q", cfg.Port, "5110")
	}
	if cfg.DatabaseURL != "postgres://botka:botka@localhost:5432/botka?sslmode=disable" {
		t.Errorf("DatabaseURL = %q, want default", cfg.DatabaseURL)
	}
	if cfg.ProjectsDir != "/home/pi/projects" {
		t.Errorf("ProjectsDir = %q, want %q", cfg.ProjectsDir, "/home/pi/projects")
	}
	if cfg.ClaudePath != "claude" {
		t.Errorf("ClaudePath = %q, want %q", cfg.ClaudePath, "claude")
	}
	if cfg.MaxWorkers != 2 {
		t.Errorf("MaxWorkers = %d, want %d", cfg.MaxWorkers, 2)
	}
	if cfg.UsagePollInterval != 15*time.Minute {
		t.Errorf("UsagePollInterval = %v, want %v", cfg.UsagePollInterval, 15*time.Minute)
	}
	if cfg.UsageThreshold5h != 0.90 {
		t.Errorf("UsageThreshold5h = %f, want %f", cfg.UsageThreshold5h, 0.90)
	}
	if cfg.UsageThreshold7d != 0.95 {
		t.Errorf("UsageThreshold7d = %f, want %f", cfg.UsageThreshold7d, 0.95)
	}
	if cfg.WhisperEnabled != true {
		t.Errorf("WhisperEnabled = %v, want true", cfg.WhisperEnabled)
	}
	if cfg.AIModel != "sonnet" {
		t.Errorf("AIModel = %q, want %q", cfg.AIModel, "sonnet")
	}
	want := []string{"sonnet", "opus", "haiku"}
	if !sliceEqual(cfg.AvailableModels, want) {
		t.Errorf("AvailableModels = %v, want %v", cfg.AvailableModels, want)
	}
}

func TestLoad_InvalidPollInterval(t *testing.T) {
	t.Setenv("USAGE_POLL_INTERVAL", "not_a_duration")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid USAGE_POLL_INTERVAL, got nil")
	}
}

func TestLoad_InvalidMaxWorkers(t *testing.T) {
	t.Setenv("MAX_WORKERS", "xyz")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid MAX_WORKERS, got nil")
	}
}

func TestLoad_InvalidThreshold5h(t *testing.T) {
	t.Setenv("USAGE_THRESHOLD_5H", "nope")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid USAGE_THRESHOLD_5H, got nil")
	}
}

func TestLoad_InvalidThreshold7d(t *testing.T) {
	t.Setenv("USAGE_THRESHOLD_7D", "nope")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid USAGE_THRESHOLD_7D, got nil")
	}
}

func TestLoad_InvalidWhisperEnabled(t *testing.T) {
	t.Setenv("WHISPER_ENABLED", "nope")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for invalid WHISPER_ENABLED, got nil")
	}
}

// --- helpers ---

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
