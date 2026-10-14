package claude

import "testing"

func TestModelArg(t *testing.T) {
	cases := map[string]string{
		"":                                   PinnedModel,
		"anthropic/claude-sonnet-4-20250514": PinnedModel,
		"opus":                               "opus",
		"haiku":                              "haiku",
	}
	for in, want := range cases {
		if got := ModelArg(in); got != want {
			t.Errorf("ModelArg(%q) = %q, want %q", in, got, want)
		}
	}
}

// argAfter returns the value following flag in args, or "" when flag is absent.
func argAfter(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// Both chat spawn paths must always pin model and effort, even when the thread
// has no model, so ~/.claude/settings.json never decides.
func TestChatSpawnArgsPinModelAndEffort(t *testing.T) {
	for _, model := range []string{"", "anthropic/claude-sonnet-4-20250514", "haiku"} {
		cfg := RunConfig{Model: model}
		want := ModelArg(model)
		for name, args := range map[string][]string{
			"buildRunArgs":    buildRunArgs(cfg),
			"buildStreamArgs": buildStreamArgs(cfg),
		} {
			if got := argAfter(args, "--model"); got != want {
				t.Errorf("%s(model=%q): --model = %q, want %q", name, model, got, want)
			}
			if got := argAfter(args, "--effort"); got != PinnedEffort {
				t.Errorf("%s(model=%q): --effort = %q, want %q", name, model, got, PinnedEffort)
			}
		}
	}
}
