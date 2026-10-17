package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestExtractFilePaths(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "empty",
			in:   "",
			want: nil,
		},
		{
			name: "no paths",
			in:   "hello world, nothing to see here",
			want: nil,
		},
		{
			name: "single absolute png",
			in:   "wrote /tmp/foo.png to disk",
			want: []string{"/tmp/foo.png"},
		},
		{
			name: "multiple distinct types",
			in:   "saved /tmp/a.png and /tmp/b.csv plus /opt/c.pdf",
			want: []string{"/tmp/a.png", "/tmp/b.csv", "/opt/c.pdf"},
		},
		{
			name: "double quoted with spaces",
			in:   `created "/tmp/path with spaces.pdf" successfully`,
			want: []string{"/tmp/path with spaces.pdf"},
		},
		{
			name: "single quoted simple",
			in:   `wrote '/tmp/report.html' to disk`,
			want: []string{"/tmp/report.html"},
		},
		{
			name: "case insensitive extension",
			in:   "wrote /tmp/IMG.PNG and /tmp/REPORT.PDF",
			want: []string{"/tmp/IMG.PNG", "/tmp/REPORT.PDF"},
		},
		{
			name: "deduplicate same path",
			in:   "wrote /tmp/foo.png. Then read /tmp/foo.png again",
			want: []string{"/tmp/foo.png"},
		},
		{
			name: "trailing punctuation stripped",
			in:   "see /tmp/foo.csv, also /tmp/bar.html.",
			want: []string{"/tmp/foo.csv", "/tmp/bar.html"},
		},
		{
			name: "skip relative paths",
			in:   "wrote ./foo.png and bar/baz.pdf",
			want: nil,
		},
		{
			name: "skip json",
			in:   "read /home/pi/app/package.json and /tmp/fixture.json",
			want: nil,
		},
		{
			name: "skip unsupported extension",
			in:   "wrote /tmp/script.sh and /tmp/data.txt",
			want: nil,
		},
		{
			name: "in parentheses",
			in:   "see file (/tmp/foo.png) for details",
			want: []string{"/tmp/foo.png"},
		},
		{
			name: "in markdown link",
			in:   "see [report](/tmp/report.pdf) for details",
			want: []string{"/tmp/report.pdf"},
		},
		{
			name: "deeply nested path",
			in:   "wrote /home/pi/projects/botka/data/uploads/abc-123.png",
			want: []string{"/home/pi/projects/botka/data/uploads/abc-123.png"},
		},
		{
			name: "do not match extensions inside other tokens",
			in:   "the file foo.pngfile is not a real png",
			want: nil,
		},
		{
			name: "bash command with output redirect",
			in:   "convert input.jpg /tmp/out.png && cat /tmp/out.png",
			want: []string{"/tmp/out.png"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractFilePaths(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("extractFilePaths(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPathCollector_FromToolUse(t *testing.T) {
	c := newPathCollector()

	c.fromToolUse("Write", json.RawMessage(`{"file_path":"/tmp/a.png","content":"x"}`))
	c.fromToolUse("Edit", json.RawMessage(`{"file_path":"/tmp/b.html"}`))
	c.fromToolUse("MultiEdit", json.RawMessage(`{"file_path":"/tmp/c.csv","edits":[]}`))
	c.fromToolUse("NotebookEdit", json.RawMessage(`{"notebook_path":"/tmp/d.ipynb"}`))
	c.fromToolUse("Bash", json.RawMessage(`{"command":"convert /tmp/in.jpg /tmp/out.pdf"}`))
	c.fromToolUse("Read", json.RawMessage(`{"file_path":"/tmp/ignored.png"}`))
	// Duplicate Write should be deduplicated.
	c.fromToolUse("Write", json.RawMessage(`{"file_path":"/tmp/a.png"}`))

	got := c.paths()
	want := []string{"/tmp/a.png", "/tmp/b.html", "/tmp/c.csv", "/tmp/d.ipynb", "/tmp/in.jpg", "/tmp/out.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %v, want %v", got, want)
	}
}

func TestPathCollector_FromToolResult(t *testing.T) {
	c := newPathCollector()
	c.fromToolResult("Created /tmp/report.html and /tmp/data.csv")
	c.fromToolResult("Re-listed /tmp/report.html") // duplicate

	got := c.paths()
	want := []string{"/tmp/report.html", "/tmp/data.csv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %v, want %v", got, want)
	}
}

func TestPathCollector_MalformedInputIsSafe(t *testing.T) {
	c := newPathCollector()
	c.fromToolUse("Write", json.RawMessage(`not json`))
	c.fromToolUse("Bash", json.RawMessage(`{"command": 42}`))
	c.fromToolUse("Bash", json.RawMessage(`{}`))

	if got := c.paths(); len(got) != 0 {
		t.Errorf("expected no paths, got %v", got)
	}
}

// writeFileWithMtime creates path (and its parents) and sets its mtime.
func writeFileWithMtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func TestSelectCapturePaths_SkipsListedOldFile(t *testing.T) {
	dir := t.TempDir()
	turnStart := time.Now()
	old := filepath.Join(dir, "old.png")
	writeFileWithMtime(t, old, turnStart.Add(-time.Hour))

	c := newPathCollector()
	c.fromToolResult(old + "\n")

	if got := selectCapturePaths(c.paths(), turnStart, nil); len(got) != 0 {
		t.Errorf("listed old file must be skipped, got %v", got)
	}
}

func TestSelectCapturePaths_CapturesFileWrittenDuringTurn(t *testing.T) {
	dir := t.TempDir()
	turnStart := time.Now()
	written := filepath.Join(dir, "chart.png")
	writeFileWithMtime(t, written, turnStart.Add(time.Second))
	// Within the clock slack still counts as written during the turn.
	slack := filepath.Join(dir, "slack.pdf")
	writeFileWithMtime(t, slack, turnStart.Add(-time.Second))

	c := newPathCollector()
	c.fromToolUse("Write", json.RawMessage(fmt.Sprintf(`{"file_path":%q}`, written)))
	c.fromToolUse("Bash", json.RawMessage(fmt.Sprintf(`{"command":"chromium --print-to-pdf=%s"}`, slack)))

	got := selectCapturePaths(c.paths(), turnStart, nil)
	want := []string{written, slack}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSelectCapturePaths_SkipsJSON(t *testing.T) {
	dir := t.TempDir()
	turnStart := time.Now()
	jsonPath := filepath.Join(dir, "config.json")
	writeFileWithMtime(t, jsonPath, turnStart.Add(time.Second))

	if got := selectCapturePaths([]string{jsonPath}, turnStart, nil); len(got) != 0 {
		t.Errorf("json must be skipped, got %v", got)
	}
}

func TestSelectCapturePaths_SkipsMissingAndDirectories(t *testing.T) {
	dir := t.TempDir()
	turnStart := time.Now()
	asDir := filepath.Join(dir, "folder.png")
	if err := os.Mkdir(asDir, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.png")

	if got := selectCapturePaths([]string{asDir, missing}, turnStart, nil); len(got) != 0 {
		t.Errorf("expected nothing, got %v", got)
	}
}

func TestSelectCapturePaths_CapsToNewest(t *testing.T) {
	dir := t.TempDir()
	turnStart := time.Now()
	var paths []string
	for i := range 11 {
		p := filepath.Join(dir, fmt.Sprintf("shot-%02d.png", i))
		writeFileWithMtime(t, p, turnStart.Add(time.Duration(i+1)*time.Second))
		paths = append(paths, p)
	}

	got := selectCapturePaths(paths, turnStart, nil)
	// The three oldest (0..2) are dropped; the rest keep observation order.
	want := paths[3:]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSelectCapturePaths_SkipsExcludedDirs(t *testing.T) {
	dir := t.TempDir()
	turnStart := time.Now()
	fresh := turnStart.Add(time.Second)
	uploads := filepath.Join(dir, "data", "uploads")
	contextDir := filepath.Join(dir, "data", "context")

	var excluded []string
	for _, rel := range []string{
		"node_modules/pkg/logo.png",
		".git/x.png",
		"dist/index.html",
		"build/report.pdf",
		"vendor/lib/doc.html",
		"data/uploads/abc.png",
		"data/context/thread-1.html",
	} {
		p := filepath.Join(dir, rel)
		writeFileWithMtime(t, p, fresh)
		excluded = append(excluded, p)
	}
	// A sibling whose name merely starts with an excluded dir must survive.
	kept := filepath.Join(dir, "data", "uploads-report.png")
	writeFileWithMtime(t, kept, fresh)
	keptNested := filepath.Join(dir, "builder", "out.png")
	writeFileWithMtime(t, keptNested, fresh)

	got := selectCapturePaths(append(excluded, kept, keptNested), turnStart, []string{uploads, contextDir, ""})
	want := []string{kept, keptNested}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
