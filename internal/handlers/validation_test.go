package handlers

import (
	"strings"
	"testing"
)

// TestValidateRequired verifies that validateRequired catches empty and whitespace-only values.
func TestValidateRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		value   string
		wantMsg string
	}{
		{name: "non-empty passes", field: "title", value: "hello", wantMsg: ""},
		{name: "empty string fails", field: "title", value: "", wantMsg: "title is required"},
		{name: "whitespace only fails", field: "name", value: "   ", wantMsg: "name is required"},
		{name: "tab only fails", field: "name", value: "\t", wantMsg: "name is required"},
		{name: "value with spaces passes", field: "title", value: " hello ", wantMsg: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := validateRequired(tt.field, tt.value)
			if got != tt.wantMsg {
				t.Errorf("validateRequired(%q, %q) = %q, want %q", tt.field, tt.value, got, tt.wantMsg)
			}
		})
	}
}

// TestValidateMaxLength verifies that validateMaxLength enforces length limits.
func TestValidateMaxLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		value   string
		max     int
		wantMsg string
	}{
		{name: "under limit passes", field: "title", value: "short", max: 10, wantMsg: ""},
		{name: "at limit passes", field: "title", value: "12345", max: 5, wantMsg: ""},
		{name: "over limit fails", field: "title", value: "123456", max: 5, wantMsg: "title must be at most 5 characters"},
		{name: "empty string passes", field: "title", value: "", max: 5, wantMsg: ""},
		{name: "large field", field: "spec", value: strings.Repeat("x", 1001), max: 1000, wantMsg: "spec must be at most 1000 characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := validateMaxLength(tt.field, tt.value, tt.max)
			if got != tt.wantMsg {
				t.Errorf("validateMaxLength(%q, len=%d, %d) = %q, want %q", tt.field, len(tt.value), tt.max, got, tt.wantMsg)
			}
		})
	}
}

// TestValidateEnum verifies that validateEnum checks against allowed values.
func TestValidateEnum(t *testing.T) {
	t.Parallel()

	allowed := []string{"sonnet", "opus", "haiku"}
	tests := []struct {
		name    string
		field   string
		value   string
		wantMsg string
	}{
		{name: "valid value passes", field: "model", value: "sonnet", wantMsg: ""},
		{name: "another valid value", field: "model", value: "opus", wantMsg: ""},
		{name: "invalid value fails", field: "model", value: "gpt4", wantMsg: "model must be one of: sonnet, opus, haiku"},
		{name: "empty value fails", field: "model", value: "", wantMsg: "model must be one of: sonnet, opus, haiku"},
		{name: "case sensitive", field: "model", value: "Sonnet", wantMsg: "model must be one of: sonnet, opus, haiku"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := validateEnum(tt.field, tt.value, allowed)
			if got != tt.wantMsg {
				t.Errorf("validateEnum(%q, %q) = %q, want %q", tt.field, tt.value, got, tt.wantMsg)
			}
		})
	}
}

// TestFirstError verifies that firstError returns the first non-empty error message.
func TestFirstError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		checks []string
		want   string
	}{
		{name: "all empty returns empty", checks: []string{"", "", ""}, want: ""},
		{name: "first error returned", checks: []string{"", "err1", "err2"}, want: "err1"},
		{name: "single error", checks: []string{"err"}, want: "err"},
		{name: "no checks returns empty", checks: []string{}, want: ""},
		{name: "first of many", checks: []string{"a", "b", "c"}, want: "a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := firstError(tt.checks...)
			if got != tt.want {
				t.Errorf("firstError(%v) = %q, want %q", tt.checks, got, tt.want)
			}
		})
	}
}
