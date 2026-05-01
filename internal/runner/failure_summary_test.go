package runner

import (
	"strings"
	"testing"
)

func TestTailLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"empty input", "", 5, ""},
		{"zero n", "a\nb\n", 0, ""},
		{"fewer lines than n", "a\nb\n", 5, "a\nb\n"},
		{"exact n", "a\nb\nc\n", 3, "a\nb\nc\n"},
		{"more lines than n", "a\nb\nc\nd\ne\n", 2, "d\ne\n"},
		{"no trailing newline", "a\nb\nc", 2, "b\nc"},
		{"single line", "single line", 5, "single line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tailLines(tt.s, tt.n)
			if got != tt.want {
				t.Errorf("tailLines(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

func TestBuildFailureSummaryPrompt_IncludesSpecAndTail(t *testing.T) {
	t.Parallel()
	spec := "Implement feature X"
	output := "line1\nline2\nERROR: panic\n"
	prompt := buildFailureSummaryPrompt(spec, output)

	if !strings.Contains(prompt, spec) {
		t.Errorf("prompt missing spec: %s", prompt)
	}
	if !strings.Contains(prompt, "ERROR: panic") {
		t.Errorf("prompt missing log tail: %s", prompt)
	}
	if !strings.Contains(prompt, "v češtině") {
		t.Errorf("prompt missing Czech instruction: %s", prompt)
	}
}

func TestBuildFailureSummaryPrompt_TruncatesLargeSpec(t *testing.T) {
	t.Parallel()
	spec := strings.Repeat("a", failureSummarySpecCap+100)
	prompt := buildFailureSummaryPrompt(spec, "")
	if !strings.Contains(prompt, "spec truncated") {
		t.Errorf("expected truncation marker for oversized spec")
	}
}
