package models

import "testing"

func TestUserIsAdmin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role UserRole
		want bool
	}{
		{RoleAdmin, true},
		{RoleExternal, false},
		{"", false},
	}

	for _, tt := range tests {
		u := &User{Role: tt.role}
		if got := u.IsAdmin(); got != tt.want {
			t.Errorf("User{Role: %q}.IsAdmin() = %v, want %v", tt.role, got, tt.want)
		}
	}
}

func TestUserRoleConstants(t *testing.T) {
	t.Parallel()

	if RoleAdmin != "admin" {
		t.Errorf("RoleAdmin = %q, want %q", RoleAdmin, "admin")
	}
	if RoleExternal != "external" {
		t.Errorf("RoleExternal = %q, want %q", RoleExternal, "external")
	}
}
