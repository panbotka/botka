package claude

import "testing"

func TestSkillDenySpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		names []string
		want  string
	}{
		{name: "nil slice yields empty spec", names: nil, want: ""},
		{name: "empty slice yields empty spec", names: []string{}, want: ""},
		{name: "single skill", names: []string{"brainstorming"}, want: "Skill(brainstorming)"},
		{
			name:  "multiple skills are space separated",
			names: []string{"brainstorming", "golang-developer"},
			want:  "Skill(brainstorming) Skill(golang-developer)",
		},
		{
			name:  "namespaced plugin skill is kept intact",
			names: []string{"superpowers:brainstorming"},
			want:  "Skill(superpowers:brainstorming)",
		},
		{name: "empty name is skipped", names: []string{"", "a"}, want: "Skill(a)"},
		{name: "name with space is skipped", names: []string{"bad name", "a"}, want: "Skill(a)"},
		{name: "name with parens is skipped", names: []string{"bad(name)", "a"}, want: "Skill(a)"},
		{name: "all names invalid yields empty spec", names: []string{"", " "}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SkillDenySpec(tt.names); got != tt.want {
				t.Errorf("SkillDenySpec(%q) = %q, want %q", tt.names, got, tt.want)
			}
		})
	}
}

func TestIsValidSkillName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "plain name", input: "golang-developer", want: true},
		{name: "namespaced name", input: "superpowers:brainstorming", want: true},
		{name: "empty", input: "", want: false},
		{name: "space", input: "a b", want: false},
		{name: "tab", input: "a\tb", want: false},
		{name: "newline", input: "a\nb", want: false},
		{name: "open paren", input: "a(b", want: false},
		{name: "close paren", input: "a)b", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isValidSkillName(tt.input); got != tt.want {
				t.Errorf("isValidSkillName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestBuildArgs_disallowedSkills checks that both spawn paths emit the deny
// flag, since a divergence there would silently leave one path unenforced.
func TestBuildArgs_disallowedSkills(t *testing.T) {
	t.Parallel()

	builders := map[string]func(RunConfig) []string{
		"buildRunArgs":    buildRunArgs,
		"buildStreamArgs": buildStreamArgs,
	}

	for name, build := range builders {
		t.Run(name+"/with disabled skills", func(t *testing.T) {
			t.Parallel()
			args := build(RunConfig{DisabledSkills: []string{"a", "b"}})
			idx := indexOf(args, "--disallowedTools")
			if idx < 0 {
				t.Fatalf("%s: expected --disallowedTools in %v", name, args)
			}
			if got := args[idx+1]; got != "Skill(a) Skill(b)" {
				t.Errorf("%s: deny spec = %q, want %q", name, got, "Skill(a) Skill(b)")
			}
		})

		t.Run(name+"/without disabled skills", func(t *testing.T) {
			t.Parallel()
			args := build(RunConfig{})
			if idx := indexOf(args, "--disallowedTools"); idx >= 0 {
				t.Errorf("%s: unexpected --disallowedTools in %v", name, args)
			}
		})
	}
}

// indexOf returns the position of want in args, or -1 when absent.
func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}
