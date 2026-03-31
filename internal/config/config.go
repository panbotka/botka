// Package config loads application configuration from .env files and environment variables.
package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration values.
type Config struct {
	Port                 string
	DatabaseURL          string
	ProjectsDir          string
	ClaudePath           string
	ClaudeUsageCmd       string
	MaxWorkers           int
	UsageThreshold5h     float64
	UsageThreshold7d     float64
	OpenClawURL          string
	OpenClawToken        string
	OpenClawWorkspace    string
	ClaudeContextDir     string
	ClaudeDefaultWorkDir string
	WhisperEnabled       bool
	UploadDir            string
	AIModel              string
	AvailableModels      []string
	WebAuthnOrigin       string
	WebAuthnRPID         string
	SessionMaxAge        time.Duration
	MCPToken             string
	BoxHost              string
	BoxSSHUser           string
	BoxWOLCommand        string
	KeepaliveEnabled     bool
	KeepaliveInterval    time.Duration
}

// Load reads configuration from the .env file and environment variables.
// Environment variables take precedence over .env file values.
// Returns an error if any numeric or duration value cannot be parsed.
func Load() (*Config, error) {
	loadDotEnv()

	threshold5h, err := getEnvFloat("USAGE_THRESHOLD_5H", 0.90)
	if err != nil {
		return nil, fmt.Errorf("parsing USAGE_THRESHOLD_5H: %w", err)
	}

	threshold7d, err := getEnvFloat("USAGE_THRESHOLD_7D", 0.95)
	if err != nil {
		return nil, fmt.Errorf("parsing USAGE_THRESHOLD_7D: %w", err)
	}

	maxWorkers, err := getEnvInt("MAX_WORKERS", 2)
	if err != nil {
		return nil, fmt.Errorf("parsing MAX_WORKERS: %w", err)
	}

	whisperEnabled, err := getEnvBool("WHISPER_ENABLED", true)
	if err != nil {
		return nil, fmt.Errorf("parsing WHISPER_ENABLED: %w", err)
	}

	keepaliveEnabled, err := getEnvBool("KEEPALIVE_ENABLED", true)
	if err != nil {
		return nil, fmt.Errorf("parsing KEEPALIVE_ENABLED: %w", err)
	}

	keepaliveInterval, err := time.ParseDuration(getEnv("KEEPALIVE_INTERVAL", "60m"))
	if err != nil {
		return nil, fmt.Errorf("parsing KEEPALIVE_INTERVAL: %w", err)
	}

	availableModels := getEnvCSV("AVAILABLE_MODELS", []string{"sonnet", "opus", "haiku"})

	sessionMaxAge, err := time.ParseDuration(getEnv("SESSION_MAX_AGE", "720h"))
	if err != nil {
		return nil, fmt.Errorf("parsing SESSION_MAX_AGE: %w", err)
	}

	// Derive WebAuthn RPID from origin if not explicitly set.
	webAuthnOrigin := getEnv("WEBAUTHN_ORIGIN", "http://localhost:5110")
	webAuthnRPID := getEnv("WEBAUTHN_RPID", "")
	if webAuthnRPID == "" {
		if u, err := url.Parse(webAuthnOrigin); err == nil {
			webAuthnRPID = u.Hostname()
		} else {
			webAuthnRPID = "localhost"
		}
	}

	return &Config{
		Port:                 getEnv("PORT", "5110"),
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://botka:botka@localhost:5432/botka?sslmode=disable"),
		ProjectsDir:          getEnv("PROJECTS_DIR", "/home/pi/projects"),
		ClaudePath:           getEnv("CLAUDE_PATH", "claude"),
		ClaudeUsageCmd:       getEnv("CLAUDE_USAGE_CMD", "/home/pi/bin/claude-usage"),
		MaxWorkers:           maxWorkers,
		UsageThreshold5h:     threshold5h,
		UsageThreshold7d:     threshold7d,
		OpenClawURL:          getEnv("OPENCLAW_URL", "http://localhost:18789"),
		OpenClawToken:        getEnv("OPENCLAW_TOKEN", ""),
		OpenClawWorkspace:    getEnv("OPENCLAW_WORKSPACE", "/home/pi/.openclaw/workspace"),
		ClaudeContextDir:     getEnv("CLAUDE_CONTEXT_DIR", "./data/context"),
		ClaudeDefaultWorkDir: getEnv("CLAUDE_DEFAULT_WORK_DIR", "/home/pi"),
		WhisperEnabled:       whisperEnabled,
		UploadDir:            getEnv("UPLOAD_DIR", "./data/uploads"),
		AIModel:              getEnv("AI_MODEL", "sonnet"),
		AvailableModels:      availableModels,
		WebAuthnOrigin:       webAuthnOrigin,
		WebAuthnRPID:         webAuthnRPID,
		SessionMaxAge:        sessionMaxAge,
		MCPToken:             getEnv("MCP_TOKEN", ""),
		BoxHost:              getEnv("BOX_HOST", "100.127.79.1"),
		BoxSSHUser:           getEnv("BOX_SSH_USER", "box"),
		BoxWOLCommand:        getEnv("BOX_WOL_COMMAND", "/home/pi/bin/boxon"),
		KeepaliveEnabled:     keepaliveEnabled,
		KeepaliveInterval:    keepaliveInterval,
	}, nil
}

// loadDotEnv reads key=value pairs from a .env file in the current directory.
// Lines starting with # are comments. Empty lines are skipped.
// Values may be quoted with single or double quotes (quotes are stripped).
// Only sets an env var if it is not already set, preserving env precedence.
func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = stripQuotes(val)
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}

// stripQuotes removes matching single or double quotes from around a value.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// getEnv returns the value of the environment variable named by key,
// or fallback if the variable is empty or unset.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt returns the environment variable as an int, or fallback if unset.
// Returns an error if the value is set but not a valid integer.
func getEnvInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q for %s: %w", v, key, err)
	}
	return n, nil
}

// getEnvFloat returns the environment variable as a float64, or fallback if unset.
// Returns an error if the value is set but not a valid float.
func getEnvFloat(key string, fallback float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid float %q for %s: %w", v, key, err)
	}
	return f, nil
}

// getEnvBool returns the environment variable as a bool, or fallback if unset.
// Returns an error if the value is set but not a valid bool.
func getEnvBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid bool %q for %s: %w", v, key, err)
	}
	return b, nil
}

// getEnvCSV returns the environment variable as a string slice split by commas,
// or fallback if unset. Whitespace around each element is trimmed.
func getEnvCSV(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
