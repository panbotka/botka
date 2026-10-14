package claude

import "strings"

// Botka pins model and effort on every Claude Code spawn (chat, session pool,
// tasks, cron) so nothing is ever inherited from ~/.claude/settings.json. That
// file belongs to interactive Claude Code; leaving an expensive model there
// once let a single Botka task burn the whole usage limit.
const (
	PinnedModel  = "opus"
	PinnedEffort = "high"
)

// ModelArg returns the value to pass as --model: the requested model when it
// is a Claude Code model name, otherwise PinnedModel. An empty value and
// OpenClaw-format names (e.g. "anthropic/claude-sonnet-4-20250514") both fall
// back, so --model is never omitted.
func ModelArg(requested string) string {
	if requested == "" || strings.Contains(requested, "/") {
		return PinnedModel
	}
	return requested
}
