// Package skills discovers Claude Code skills on disk and syncs them into the
// Botka skill registry.
//
// Claude Code loads skills from three places: the user's global
// ~/.claude/skills, a project's .claude/skills, and the skills bundled with
// installed plugins under ~/.claude/plugins. Each skill is a directory holding
// a SKILL.md whose YAML frontmatter carries its name and description.
//
// Botka mirrors what it finds into the `skills` table so a chat can be told
// which skills it may use. The registry is keyed by the skill's *invocable*
// name — plugin skills are namespaced as "plugin:skill", matching how Claude
// Code addresses them in a Skill(<name>) permission specifier.
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// Source values stored in the registry's `source` column.
const (
	SourceUser    = "user"
	SourceProject = "project"
	// SourcePluginPrefix is followed by the plugin name, e.g. "plugin:superpowers".
	SourcePluginPrefix = "plugin:"
)

// Discovered is a skill found on disk.
type Discovered struct {
	Name        string
	Description string
	Source      string
}

// frontmatter is the subset of a SKILL.md YAML header that Botka cares about.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// HomeDir returns the current user's home directory, falling back to the HOME
// environment variable when the user database is unavailable (e.g. in
// containers with no passwd entry). It returns an empty string if neither
// source yields a path.
func HomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return os.Getenv("HOME")
}

// Scan discovers every skill reachable from homeDir (user skills and plugin
// skills) and from the git repositories directly under projectsDir (project
// skills). Missing directories are not an error — they simply contribute no
// skills. Names are deduplicated with user skills winning over project skills,
// which win over plugin skills; the result is sorted by name.
//
// A SKILL.md that cannot be read or whose frontmatter lacks a name is skipped
// rather than failing the whole scan, so one malformed skill never blocks the
// registry sync at startup.
func Scan(homeDir, projectsDir string) ([]Discovered, error) {
	var found []Discovered

	if homeDir != "" {
		user, err := scanUserSkills(homeDir)
		if err != nil {
			return nil, err
		}
		found = append(found, user...)
	}

	if projectsDir != "" {
		project, err := scanProjectSkills(projectsDir)
		if err != nil {
			return nil, err
		}
		found = append(found, project...)
	}

	if homeDir != "" {
		plugin, err := scanPluginSkills(homeDir)
		if err != nil {
			return nil, err
		}
		found = append(found, plugin...)
	}

	return dedupe(found), nil
}

// scanUserSkills reads ~/.claude/skills/<name>/SKILL.md.
func scanUserSkills(homeDir string) ([]Discovered, error) {
	pattern := filepath.Join(homeDir, ".claude", "skills", "*", "SKILL.md")
	return collect(pattern, func(string) (string, string) { return "", SourceUser })
}

// scanProjectSkills reads <projectsDir>/*/.claude/skills/<name>/SKILL.md.
func scanProjectSkills(projectsDir string) ([]Discovered, error) {
	pattern := filepath.Join(projectsDir, "*", ".claude", "skills", "*", "SKILL.md")
	return collect(pattern, func(string) (string, string) { return "", SourceProject })
}

// scanPluginSkills reads the cached plugin tree,
// ~/.claude/plugins/cache/<marketplace>/<plugin>/<version>/skills/<name>/SKILL.md,
// and namespaces each skill as "<plugin>:<name>".
func scanPluginSkills(homeDir string) ([]Discovered, error) {
	pattern := filepath.Join(homeDir, ".claude", "plugins", "cache", "*", "*", "*", "skills", "*", "SKILL.md")
	return collect(pattern, func(path string) (string, string) {
		plugin := pluginNameFromPath(path)
		if plugin == "" {
			return "", SourcePluginPrefix + "unknown"
		}
		return plugin + ":", SourcePluginPrefix + plugin
	})
}

// pluginNameFromPath extracts the plugin name from a cached plugin skill path.
// The layout is .../cache/<marketplace>/<plugin>/<version>/skills/<skill>/SKILL.md,
// so the plugin sits five elements above SKILL.md. It returns "" when the path
// is shorter than that.
func pluginNameFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 5 {
		return ""
	}
	return parts[len(parts)-5]
}

// collect globs pattern and parses every matched SKILL.md. The meta callback
// receives the file path and returns the name prefix to apply and the source
// to record. Unreadable or nameless skills are skipped.
func collect(pattern string, meta func(path string) (namePrefix, source string)) ([]Discovered, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", pattern, err)
	}

	result := make([]Discovered, 0, len(matches))
	for _, path := range matches {
		data, readErr := os.ReadFile(path) //nolint:gosec // path comes from a fixed glob
		if readErr != nil {
			continue
		}
		fm, parseErr := parseFrontmatter(data)
		if parseErr != nil || fm.Name == "" {
			continue
		}
		prefix, source := meta(path)
		result = append(result, Discovered{
			Name:        prefix + fm.Name,
			Description: strings.TrimSpace(fm.Description),
			Source:      source,
		})
	}
	return result, nil
}

// parseFrontmatter extracts the leading YAML frontmatter block of a SKILL.md.
// It returns an error when the document does not start with a "---" fence or
// the fence is never closed, and propagates YAML syntax errors.
func parseFrontmatter(data []byte) (frontmatter, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return frontmatter{}, fmt.Errorf("skill file has no frontmatter fence")
	}

	body := text[len("---\n"):]
	end := strings.Index(body, "\n---")
	if end < 0 {
		return frontmatter{}, fmt.Errorf("skill file has an unterminated frontmatter fence")
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(body[:end+1]), &fm); err != nil {
		return frontmatter{}, fmt.Errorf("parse frontmatter: %w", err)
	}
	fm.Name = strings.TrimSpace(fm.Name)
	return fm, nil
}

// dedupe keeps the first occurrence of every skill name (callers pass sources
// in priority order) and returns the survivors sorted by name.
func dedupe(found []Discovered) []Discovered {
	seen := make(map[string]bool, len(found))
	result := make([]Discovered, 0, len(found))
	for _, d := range found {
		if seen[d.Name] {
			continue
		}
		seen[d.Name] = true
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
