package handlers

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// quotedPathRegex matches absolute paths inside single or double quotes,
// which is the only safe way to encode paths containing spaces. The two
// alternatives use independent capture groups so we can extract whichever
// matched.
var quotedPathRegex = regexp.MustCompile(
	`(?i)"(/[^"\n]*\.(?:png|jpe?g|gif|webp|pdf|html|csv))"` +
		`|'(/[^'\n]*\.(?:png|jpe?g|gif|webp|pdf|html|csv))'`,
)

// unquotedPathRegex matches bare absolute paths ending in a supported
// extension. The leading non-capturing group requires either start-of-
// string or a non-pathname character before the leading slash, so that
// relative paths like "./foo.png" or "bar/baz.pdf" do not match.
// The body excludes characters that are unlikely to appear in a real
// shell-readable path so the regex stops cleanly at end-of-token.
// The trailing \b prevents matching inside larger tokens such as
// "/tmp/foo.pngs" or "/tmp/file.csvdump".
var unquotedPathRegex = regexp.MustCompile(
	`(?i)(?:^|[^a-zA-Z0-9./_-])(/[^\s"'<>()|;,\[\]{}` + "`" + `]+\.(?:png|jpe?g|gif|webp|pdf|html|csv))\b`,
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
// Write/Edit/MultiEdit tool calls expose a `file_path` field directly and
// NotebookEdit a `notebook_path`; Bash commands are scanned with
// extractFilePaths so paths embedded in the command line
// ("chromium --screenshot=/tmp/x.png") are picked up too. Every candidate
// still has to pass selectCapturePaths, so a path the command merely reads
// is dropped by the mtime rule.
func (c *pathCollector) fromToolUse(toolName string, input json.RawMessage) {
	switch toolName {
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		var in struct {
			FilePath     string `json:"file_path"`
			NotebookPath string `json:"notebook_path"`
		}
		if json.Unmarshal(input, &in) == nil {
			c.add(in.FilePath)
			c.add(in.NotebookPath)
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
// file paths ending in a supported extension. This catches tools that
// report "saved to /path/x.png". Listings and reads (ls, find, grep) match
// too, but those files predate the turn and are dropped by the mtime rule
// in selectCapturePaths.
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

// maxCapturedFiles caps how many files a single assistant reply may
// auto-attach. When more qualify, the most recently modified ones win.
const maxCapturedFiles = 8

// captureMtimeSlack tolerates clock and filesystem timestamp granularity
// differences between the turn start and a file written right after it.
const captureMtimeSlack = 2 * time.Second

// excludedCaptureSegments are directory names that never hold deliverables.
// A candidate path containing any of them as a path segment is skipped.
var excludedCaptureSegments = map[string]struct{}{
	"node_modules": {},
	".git":         {},
	"dist":         {},
	"build":        {},
	"vendor":       {},
}

// captureCandidate is a path that passed every filter, along with the
// modification time used to rank it against the cap.
type captureCandidate struct {
	path    string
	modTime time.Time
	order   int
}

// selectCapturePaths filters the paths collected during a reply down to
// the files the agent actually produced. A path qualifies only when it has
// a supported extension, lies outside the excluded directories (the
// segments in excludedCaptureSegments plus every directory in excludeDirs,
// e.g. the uploads and context dirs), is an existing regular file, and was
// modified at or after turnStart (minus captureMtimeSlack). At most
// maxCapturedFiles are returned — the most recently modified — in the
// order they were first observed.
func selectCapturePaths(paths []string, turnStart time.Time, excludeDirs []string) []string {
	cutoff := turnStart.Add(-captureMtimeSlack)

	var candidates []captureCandidate
	for i, p := range paths {
		if _, ok := extToMime[strings.ToLower(filepath.Ext(p))]; !ok {
			continue
		}
		clean := filepath.Clean(p)
		if inExcludedDir(clean, excludeDirs) {
			continue
		}
		info, err := os.Stat(clean)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.ModTime().Before(cutoff) {
			continue
		}
		candidates = append(candidates, captureCandidate{path: p, modTime: info.ModTime(), order: i})
	}

	if len(candidates) > maxCapturedFiles {
		sort.SliceStable(candidates, func(a, b int) bool {
			return candidates[a].modTime.After(candidates[b].modTime)
		})
		log.Printf("[chat] %d files qualified for auto-attach, keeping the %d newest and skipping %d",
			len(candidates), maxCapturedFiles, len(candidates)-maxCapturedFiles)
		candidates = candidates[:maxCapturedFiles]
		sort.SliceStable(candidates, func(a, b int) bool {
			return candidates[a].order < candidates[b].order
		})
	}

	out := make([]string, 0, len(candidates))
	for _, cand := range candidates {
		out = append(out, cand.path)
	}
	return out
}

// inExcludedDir reports whether the cleaned absolute path p lies inside one
// of the excluded directory names or inside any directory in excludeDirs.
// Relative excludeDirs are resolved against the working directory; empty
// entries are ignored.
func inExcludedDir(p string, excludeDirs []string) bool {
	for _, seg := range strings.Split(filepath.Dir(p), string(filepath.Separator)) {
		if _, ok := excludedCaptureSegments[seg]; ok {
			return true
		}
	}
	for _, dir := range excludeDirs {
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if p == abs || strings.HasPrefix(p, abs+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
