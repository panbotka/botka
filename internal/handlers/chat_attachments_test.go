package handlers

import (
	"encoding/json"
	"reflect"
	"testing"
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
			in:   "saved /tmp/a.png and /tmp/b.csv plus /opt/c.json",
			want: []string{"/tmp/a.png", "/tmp/b.csv", "/opt/c.json"},
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
			in:   "see /tmp/foo.json, also /tmp/bar.html.",
			want: []string{"/tmp/foo.json", "/tmp/bar.html"},
		},
		{
			name: "skip relative paths",
			in:   "wrote ./foo.png and bar/baz.pdf",
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
	c.fromToolUse("Edit", json.RawMessage(`{"file_path":"/tmp/b.json"}`))
	c.fromToolUse("Bash", json.RawMessage(`{"command":"convert /tmp/in.jpg /tmp/out.pdf"}`))
	c.fromToolUse("Read", json.RawMessage(`{"file_path":"/tmp/ignored.png"}`))
	// Duplicate Write should be deduplicated.
	c.fromToolUse("Write", json.RawMessage(`{"file_path":"/tmp/a.png"}`))

	got := c.paths()
	want := []string{"/tmp/a.png", "/tmp/b.json", "/tmp/in.jpg", "/tmp/out.pdf"}
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
