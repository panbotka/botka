package middleware

import "testing"

func TestIsAllowedExternalPath(t *testing.T) {
	tests := []struct {
		path    string
		method  string
		allowed bool
	}{
		// Thread list — allowed
		{"/api/v1/threads", "GET", true},
		// Thread list — POST not allowed (no thread creation)
		{"/api/v1/threads", "POST", false},
		// Thread detail — GET allowed
		{"/api/v1/threads/42", "GET", true},
		// Thread detail — DELETE not allowed
		{"/api/v1/threads/42", "DELETE", false},
		// Thread detail — PUT not allowed (rename)
		{"/api/v1/threads/42", "PUT", false},
		// Messages — POST and GET allowed
		{"/api/v1/threads/42/messages", "POST", true},
		{"/api/v1/threads/42/messages", "GET", true},
		{"/api/v1/threads/42/messages", "DELETE", false},
		// Stream subscribe — allowed
		{"/api/v1/threads/42/stream/subscribe", "GET", true},
		// Regenerate — allowed
		{"/api/v1/threads/42/regenerate", "POST", true},
		// Branch — allowed
		{"/api/v1/threads/42/branch", "POST", true},
		// Session health — allowed
		{"/api/v1/threads/42/session-health", "GET", true},
		// Interrupt — allowed
		{"/api/v1/threads/42/interrupt", "POST", true},
		// Edit message — allowed
		{"/api/v1/threads/42/messages/123/edit", "POST", true},
		// Thread operations not allowed for external
		{"/api/v1/threads/42/pin", "PUT", false},
		{"/api/v1/threads/42/archive", "PUT", false},
		{"/api/v1/threads/42/model", "PUT", false},
		{"/api/v1/threads/42/project", "PUT", false},
		{"/api/v1/threads/42/tags", "PUT", false},
		{"/api/v1/threads/42/usage", "GET", false},
		// Auth endpoints — allowed
		{"/api/v1/auth/login", "POST", true},
		{"/api/v1/auth/me", "GET", true},
		{"/api/v1/auth/logout", "POST", true},
		// Transcribe — allowed
		{"/api/v1/transcribe", "POST", true},
		{"/api/v1/transcribe/status", "GET", true},
		// Status — allowed
		{"/api/v1/status", "GET", true},
		// Admin-only endpoints — forbidden
		{"/api/v1/tasks", "GET", false},
		{"/api/v1/projects", "GET", false},
		{"/api/v1/settings", "GET", false},
		{"/api/v1/personas", "GET", false},
		{"/api/v1/tags", "GET", false},
		{"/api/v1/memories", "GET", false},
		{"/api/v1/analytics/cost", "GET", false},
		{"/api/v1/users", "GET", false},
		{"/api/v1/runner/status", "GET", false},
		{"/api/v1/search", "GET", false},
		// Non-numeric thread ID
		{"/api/v1/threads/abc", "GET", false},
	}

	for _, tt := range tests {
		got := isAllowedExternalPath(tt.path, tt.method)
		if got != tt.allowed {
			t.Errorf("isAllowedExternalPath(%q, %q) = %v, want %v", tt.path, tt.method, got, tt.allowed)
		}
	}
}

func TestExtractThreadID(t *testing.T) {
	tests := []struct {
		path string
		want int64
	}{
		{"/api/v1/threads/42", 42},
		{"/api/v1/threads/42/messages", 42},
		{"/api/v1/threads/100/stream/subscribe", 100},
		{"/api/v1/threads/abc", 0},
		{"/api/v1/tasks", 0},
		{"/api/v1/threads", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := extractThreadID(tt.path)
		if got != tt.want {
			t.Errorf("extractThreadID(%q) = %d, want %d", tt.path, got, tt.want)
		}
	}
}
