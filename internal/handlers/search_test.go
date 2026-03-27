package handlers

import (
	"strings"
	"testing"
)

func TestBuildSnippet(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		query    string
		wantMark bool   // should contain <mark>...</mark>
		wantSub  string // substring expected in output
	}{
		{
			name:     "simple match",
			content:  "hello world",
			query:    "world",
			wantMark: true,
			wantSub:  "<mark>world</mark>",
		},
		{
			name:     "no match fallback",
			content:  "abc",
			query:    "xyz",
			wantMark: false,
			wantSub:  "abc",
		},
		{
			name:     "long content match in middle",
			content:  strings.Repeat("a", 200) + "TARGET" + strings.Repeat("b", 200),
			query:    "TARGET",
			wantMark: true,
			wantSub:  "<mark>TARGET</mark>",
		},
		{
			name:     "short content no ellipsis",
			content:  "short text here",
			query:    "text",
			wantMark: true,
			wantSub:  "<mark>text</mark>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSnippet(tt.content, tt.query)
			if tt.wantMark && !strings.Contains(got, "<mark>") {
				t.Errorf("expected <mark> tag in snippet, got %q", got)
			}
			if !tt.wantMark && strings.Contains(got, "<mark>") {
				t.Errorf("did not expect <mark> tag in snippet, got %q", got)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("expected snippet to contain %q, got %q", tt.wantSub, got)
			}
		})
	}
}

func TestBuildSnippet_NoMatch(t *testing.T) {
	got := buildSnippet("abc def ghi", "xyz")
	if strings.Contains(got, "<mark>") {
		t.Errorf("expected no mark tag for unmatched query, got %q", got)
	}
	if got != "abc def ghi" {
		t.Errorf("expected full content for short unmatched, got %q", got)
	}
}

func TestBuildSnippet_ShortContent(t *testing.T) {
	got := buildSnippet("hello world", "hello")
	if !strings.Contains(got, "<mark>hello</mark>") {
		t.Errorf("expected <mark>hello</mark> in snippet, got %q", got)
	}
	// Short content should not have leading "..."
	if strings.HasPrefix(got, "...") {
		t.Errorf("short content should not have leading ellipsis, got %q", got)
	}
}

func TestBuildSnippet_LongContent(t *testing.T) {
	content := strings.Repeat("x", 200) + "NEEDLE" + strings.Repeat("y", 200)
	got := buildSnippet(content, "NEEDLE")

	if !strings.Contains(got, "<mark>NEEDLE</mark>") {
		t.Errorf("expected <mark>NEEDLE</mark>, got %q", got)
	}
	// Match is far from start, so we expect leading "..."
	if !strings.HasPrefix(got, "...") {
		t.Errorf("expected leading ellipsis for long content, got %q", got)
	}
	// Match is far from end, so we expect trailing "..."
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected trailing ellipsis for long content, got %q", got)
	}
}

func TestBuildSnippet_DiacriticMatch(t *testing.T) {
	got := buildSnippet("příliš žluťoučký kůň", "prilis")
	if !strings.Contains(got, "<mark>") {
		t.Errorf("expected diacritic match to produce <mark> tag, got %q", got)
	}
	// The original accented text should be in the mark.
	if !strings.Contains(got, "příliš") {
		t.Errorf("expected original accented text preserved, got %q", got)
	}
}

func TestStripDiacritics(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"příliš žluťoučký", "prilis zlutoucky"},
		{"café", "cafe"},
		{"ascii", "ascii"},
		{"", ""},
		{"Ñoño", "Nono"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripDiacritics(tt.input)
			if got != tt.want {
				t.Errorf("stripDiacritics(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
