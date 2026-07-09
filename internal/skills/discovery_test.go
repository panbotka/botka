package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkill creates dir/SKILL.md with the given content, failing the test on error.
func writeSkill(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("write SKILL.md in %s: %v", dir, err)
	}
}

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantName    string
		wantDesc    string
		wantErr     bool
		description string
	}{
		{
			name:     "simple frontmatter",
			input:    "---\nname: alpha\ndescription: does alpha things\n---\n\n# Alpha\n",
			wantName: "alpha",
			wantDesc: "does alpha things",
		},
		{
			name:     "folded description scalar",
			input:    "---\nname: beta\ndescription: >\n  first line\n  second line\n---\nbody\n",
			wantName: "beta",
			wantDesc: "first line second line",
		},
		{
			name:     "crlf line endings",
			input:    "---\r\nname: gamma\r\ndescription: g\r\n---\r\nbody\r\n",
			wantName: "gamma",
			wantDesc: "g",
		},
		{
			name:     "missing description is empty",
			input:    "---\nname: delta\n---\nbody\n",
			wantName: "delta",
			wantDesc: "",
		},
		{
			name:    "no fence",
			input:   "# Just markdown\n",
			wantErr: true,
		},
		{
			name:    "unterminated fence",
			input:   "---\nname: eps\n",
			wantErr: true,
		},
		{
			name:    "invalid yaml",
			input:   "---\nname: [unclosed\n---\nbody\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm, err := parseFrontmatter([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseFrontmatter(%q) = %+v, want error", tt.input, fm)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFrontmatter(%q) error = %v", tt.input, err)
			}
			if fm.Name != tt.wantName {
				t.Errorf("name = %q, want %q", fm.Name, tt.wantName)
			}
			if got := trimSpaceLike(fm.Description); got != tt.wantDesc {
				t.Errorf("description = %q, want %q", got, tt.wantDesc)
			}
		})
	}
}

// trimSpaceLike normalizes the trailing newline YAML folded scalars produce.
func trimSpaceLike(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}

func TestPluginNameFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "cached plugin layout",
			path: "/home/u/.claude/plugins/cache/official/superpowers/6.1.1/skills/brainstorming/SKILL.md",
			want: "superpowers",
		},
		{
			name: "too short",
			path: "SKILL.md",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := pluginNameFromPath(tt.path); got != tt.want {
				t.Errorf("pluginNameFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestScan_allSources(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	projectsDir := t.TempDir()

	writeSkill(t, filepath.Join(home, ".claude", "skills", "alpha"),
		"---\nname: alpha\ndescription: user skill\n---\n")
	writeSkill(t, filepath.Join(projectsDir, "repo", ".claude", "skills", "beta"),
		"---\nname: beta\ndescription: project skill\n---\n")
	writeSkill(t, filepath.Join(home, ".claude", "plugins", "cache", "market", "superpowers", "6.1.1", "skills", "brainstorming"),
		"---\nname: brainstorming\ndescription: plugin skill\n---\n")

	got, err := Scan(home, projectsDir)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	want := []Discovered{
		{Name: "alpha", Description: "user skill", Source: SourceUser},
		{Name: "beta", Description: "project skill", Source: SourceProject},
		{Name: "superpowers:brainstorming", Description: "plugin skill", Source: "plugin:superpowers"},
	}
	if len(got) != len(want) {
		t.Fatalf("Scan() returned %d skills (%+v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Scan()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestScan_skipsMalformedAndMissingDirs(t *testing.T) {
	t.Parallel()

	home := t.TempDir()

	writeSkill(t, filepath.Join(home, ".claude", "skills", "good"),
		"---\nname: good\ndescription: fine\n---\n")
	writeSkill(t, filepath.Join(home, ".claude", "skills", "nofence"), "# no frontmatter\n")
	writeSkill(t, filepath.Join(home, ".claude", "skills", "noname"), "---\ndescription: nameless\n---\n")

	// projectsDir does not exist — must not fail the scan.
	got, err := Scan(home, filepath.Join(home, "does-not-exist"))
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "good" {
		t.Fatalf("Scan() = %+v, want only the well-formed skill", got)
	}
}

func TestScan_userSkillWinsOverPluginWithSameName(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeSkill(t, filepath.Join(home, ".claude", "skills", "dup"),
		"---\nname: dup\ndescription: from user\n---\n")
	// Two cached versions of the same plugin resolve to the same namespaced name.
	writeSkill(t, filepath.Join(home, ".claude", "plugins", "cache", "m", "p", "1.0.0", "skills", "s"),
		"---\nname: s\ndescription: v1\n---\n")
	writeSkill(t, filepath.Join(home, ".claude", "plugins", "cache", "m", "p", "2.0.0", "skills", "s"),
		"---\nname: s\ndescription: v2\n---\n")

	got, err := Scan(home, "")
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Scan() = %+v, want 2 deduped skills", got)
	}
	if got[0].Name != "dup" || got[0].Source != SourceUser {
		t.Errorf("got[0] = %+v, want the user skill", got[0])
	}
	if got[1].Name != "p:s" || got[1].Description != "v1" {
		t.Errorf("got[1] = %+v, want the first cached plugin version to win", got[1])
	}
}

func TestScan_emptyDirsYieldNothing(t *testing.T) {
	t.Parallel()

	got, err := Scan("", "")
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Scan(\"\", \"\") = %+v, want no skills", got)
	}
}

func TestHomeDir(t *testing.T) {
	if got := HomeDir(); got == "" {
		t.Error("HomeDir() = \"\", want a non-empty path in the test environment")
	}
}

func TestDedupe_sortsByName(t *testing.T) {
	t.Parallel()

	got := dedupe([]Discovered{
		{Name: "zeta"},
		{Name: "alpha"},
		{Name: "zeta", Description: "shadowed"},
	})
	if len(got) != 2 {
		t.Fatalf("dedupe() = %+v, want 2 entries", got)
	}
	if got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Errorf("dedupe() = %+v, want sorted by name", got)
	}
	if got[1].Description != "" {
		t.Errorf("dedupe() kept the shadowed duplicate: %+v", got[1])
	}
}
