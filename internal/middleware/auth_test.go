package middleware

import "testing"

func TestIsPublicPath(t *testing.T) {
	tests := []struct {
		path   string
		public bool
	}{
		{"/api/v1/auth/login", true},
		{"/api/v1/auth/me", true},
		{"/api/v1/auth/passkey/login/begin", true},
		{"/api/v1/auth/passkey/login/finish", true},
		{"/api/v1/threads", false},
		{"/api/v1/tasks", false},
		{"/api/v1/auth/logout", false},
		{"/api/v1/auth/password", false},
		{"/api/v1/auth/passkeys", false},
		{"/mcp/sse", true},
		{"/", true},
		{"/login", true},
		{"/settings", true},
	}

	for _, tt := range tests {
		got := isPublicPath(tt.path)
		if got != tt.public {
			t.Errorf("isPublicPath(%q) = %v, want %v", tt.path, got, tt.public)
		}
	}
}
