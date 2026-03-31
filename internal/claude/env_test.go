package claude

import (
	"os"
	"strings"
	"testing"
)

func TestSanitizedEnv_ExcludesPORT(t *testing.T) {
	t.Setenv("PORT", "5110")

	env := SanitizedEnv()
	for _, e := range env {
		if strings.HasPrefix(e, "PORT=") {
			t.Fatalf("SanitizedEnv() should exclude PORT, but found %q", e)
		}
	}
}

func TestSanitizedEnv_ExcludesBotkaVars(t *testing.T) {
	vars := map[string]string{
		"DATABASE_URL":    "postgres://botka:botka@localhost/botka",
		"MCP_TOKEN":       "secret-token",
		"SESSION_MAX_AGE": "720h",
		"WEBAUTHN_ORIGIN": "http://localhost:5110",
		"WEBAUTHN_RPID":   "localhost",
	}
	for k, v := range vars {
		t.Setenv(k, v)
	}

	env := SanitizedEnv()
	for _, e := range env {
		for k := range vars {
			if strings.HasPrefix(e, k+"=") {
				t.Errorf("SanitizedEnv() should exclude %s, but found %q", k, e)
			}
		}
	}
}

func TestSanitizedEnv_PreservesOtherVars(t *testing.T) {
	t.Setenv("PORT", "5110")
	t.Setenv("HOME", os.Getenv("HOME"))
	t.Setenv("PATH", os.Getenv("PATH"))

	env := SanitizedEnv()

	foundHome := false
	foundPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "HOME=") {
			foundHome = true
		}
		if strings.HasPrefix(e, "PATH=") {
			foundPath = true
		}
	}
	if !foundHome {
		t.Error("SanitizedEnv() should preserve HOME")
	}
	if !foundPath {
		t.Error("SanitizedEnv() should preserve PATH")
	}
}

func TestIsSanitized(t *testing.T) {
	tests := []struct {
		envVar string
		want   bool
	}{
		{"PORT=5110", true},
		{"DATABASE_URL=postgres://...", true},
		{"MCP_TOKEN=abc", true},
		{"SESSION_MAX_AGE=720h", true},
		{"WEBAUTHN_ORIGIN=http://localhost", true},
		{"WEBAUTHN_RPID=localhost", true},
		{"HOME=/home/pi", false},
		{"PATH=/usr/bin", false},
		{"PORTABLE_APP=true", false}, // starts with PORT but not PORT=
	}
	for _, tt := range tests {
		got := isSanitized(tt.envVar)
		if got != tt.want {
			t.Errorf("isSanitized(%q) = %v, want %v", tt.envVar, got, tt.want)
		}
	}
}
