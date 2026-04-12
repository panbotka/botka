package handlers

import (
	"encoding/json"
	"regexp"
	"strings"
)

// quotedPathRegex matches absolute paths inside single or double quotes,
// which is the only safe way to encode paths containing spaces. The two
// alternatives use independent capture groups so we can extract whichever
// matched.
var quotedPathRegex = regexp.MustCompile(
	`(?i)"(/[^"\n]*\.(?:png|jpe?g|gif|webp|pdf|html|json|csv))"` +
		`|'(/[^'\n]*\.(?:png|jpe?g|gif|webp|pdf|html|json|csv))'`,
)

// unquotedPathRegex matches bare absolute paths ending in a supported
// extension. The leading non-capturing group requires either start-of-
// string or a non-pathname character before the leading slash, so that
// relative paths like "./foo.png" or "bar/baz.pdf" do not match.
// The body excludes characters that are unlikely to appear in a real
// shell-readable path so the regex stops cleanly at end-of-token.
// The trailing \b prevents matching inside larger tokens such as
// "/tmp/foo.pngs" or "/tmp/file.jsondump".
var unquotedPathRegex = regexp.MustCompile(
	`(?i)(?:^|[^a-zA-Z0-9./_-])(/[^\s"'<>()|;,\[\]{}` + "`" + `]+\.(?:png|jpe?g|gif|webp|pdf|html|json|csv))\b`,
)

// extractFilePaths returns absolute file paths in s that end in a
// supported extension. Both quoted (single/double, allowing spaces) and
// unquoted forms are recognized. Results are deduplicated in order of
// first appearance and any trailing punctuation that crept past the
// regex (commas, periods, etc. that follow the extension) is stripped.
func extractFilePaths(s string) []string {
	if s == "" {
		return nil
	}

	seen := make(map[string]struct{})
	var paths []string

	add := func(p string) {
		p = strings.TrimRight(p, ".,;:!?")
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}

	for _, m := range quotedPathRegex.FindAllStringSubmatch(s, -1) {
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				add(m[i])
				break
			}
		}
	}

	// Strip already-matched quoted paths so the unquoted pass cannot
	// re-match the inner path and produce duplicates.
	stripped := quotedPathRegex.ReplaceAllString(s, " ")
	for _, m := range unquotedPathRegex.FindAllStringSubmatch(stripped, -1) {
		if len(m) > 1 && m[1] != "" {
			add(m[1])
		}
	}

	return paths
}

// pathCollector accumulates absolute file paths discovered in stream
// events for a single assistant turn. Paths are deduplicated in order
// of first appearance so the post-streaming attachment loop touches
// each file at most once.
type pathCollector struct {
	seen  map[string]struct{}
	order []string
}

func newPathCollector() *pathCollector {
	return &pathCollector{seen: make(map[string]struct{})}
}

func (c *pathCollector) add(p string) {
	if p == "" {
		return
	}
	if _, ok := c.seen[p]; ok {
		return
	}
	c.seen[p] = struct{}{}
	c.order = append(c.order, p)
}

// fromToolUse extracts candidate paths from a tool_use input payload.
// Write/Edit tool calls expose a `file_path` field directly; Bash
// commands are scanned with extractFilePaths so paths embedded in the
// command line ("convert /tmp/in.png /tmp/out.pdf") are picked up too.
func (c *pathCollector) fromToolUse(toolName string, input json.RawMessage) {
	switch toolName {
	case "Write", "Edit":
		var in struct {
			FilePath string `json:"file_path"`
		}
		if json.Unmarshal(input, &in) == nil {
			c.add(in.FilePath)
		}
	case "Bash":
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &in) == nil {
			for _, p := range extractFilePaths(in.Command) {
				c.add(p)
			}
		}
	}
}

// fromToolResult scans a tool_result content payload for any absolute
// file paths ending in a supported extension. This catches files
// referenced in Bash output, ls/find listings, and similar tool output.
func (c *pathCollector) fromToolResult(content string) {
	for _, p := range extractFilePaths(content) {
		c.add(p)
	}
}

// paths returns the deduplicated paths in the order they were first
// observed.
func (c *pathCollector) paths() []string {
	return c.order
}
