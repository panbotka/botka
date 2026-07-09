package claude

import "strings"

// SkillDenySpec renders a set of skill names into the value of Claude Code's
// --disallowedTools flag, e.g. `Skill(brainstorming) Skill(golang-developer)`.
//
// Claude Code offers no positive allowlist for skills, so a chat's skill
// selection is enforced subtractively: every skill that is OFF gets a deny
// entry. Names that would break the specifier grammar — empty, or containing
// whitespace or parentheses — are skipped, because a malformed entry makes the
// CLI reject the whole flag and would silently drop every other deny.
//
// It returns an empty string when no name survives, in which case callers must
// omit the flag entirely.
func SkillDenySpec(names []string) string {
	specs := make([]string, 0, len(names))
	for _, name := range names {
		if !isValidSkillName(name) {
			continue
		}
		specs = append(specs, "Skill("+name+")")
	}
	return strings.Join(specs, " ")
}

// isValidSkillName reports whether name can be embedded in a Skill(<name>)
// permission specifier without ambiguity.
func isValidSkillName(name string) bool {
	if name == "" {
		return false
	}
	return !strings.ContainsAny(name, " \t\n()")
}
