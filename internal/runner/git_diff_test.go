package runner

import (
	"strings"
	"testing"
)

func TestSummarizeNumstat(t *testing.T) {
	in := "3\t1\tfoo.go\n0\t10\tbar.go\n-\t-\timg.png\n"
	got := summarizeNumstat(in)
	if got.FilesChanged != 3 {
		t.Errorf("files_changed: got %d, want 3", got.FilesChanged)
	}
	if got.Insertions != 3 {
		t.Errorf("insertions: got %d, want 3", got.Insertions)
	}
	if got.Deletions != 11 {
		t.Errorf("deletions: got %d, want 11", got.Deletions)
	}
}

func TestSummarizeNumstatEmpty(t *testing.T) {
	if got := summarizeNumstat(""); got != (DiffStats{}) {
		t.Errorf("expected zero stats, got %+v", got)
	}
	if got := summarizeNumstat("\n"); got != (DiffStats{}) {
		t.Errorf("expected zero stats from blank line, got %+v", got)
	}
}

func TestSummarizeNumstatSkipsMalformed(t *testing.T) {
	in := "not-a-line\n5\t2\tfoo.go\n"
	got := summarizeNumstat(in)
	if got.FilesChanged != 1 || got.Insertions != 5 || got.Deletions != 2 {
		t.Errorf("expected single file, got %+v", got)
	}
}

func TestCapDiff(t *testing.T) {
	short := "small diff"
	out, truncated := capDiff(short, 100)
	if truncated || out != short {
		t.Errorf("short string should pass through: out=%q truncated=%v", out, truncated)
	}

	big := strings.Repeat("a", 1000)
	out, truncated = capDiff(big, 256)
	if !truncated {
		t.Errorf("expected truncated=true")
	}
	if len(out) != 256 {
		t.Errorf("expected len 256, got %d", len(out))
	}
}

func TestParseStatCountBinary(t *testing.T) {
	if got := parseStatCount("-"); got != 0 {
		t.Errorf("binary marker should be 0, got %d", got)
	}
	if got := parseStatCount("42"); got != 42 {
		t.Errorf("numeric: got %d, want 42", got)
	}
	if got := parseStatCount("not-a-number"); got != 0 {
		t.Errorf("garbage: got %d, want 0", got)
	}
}
