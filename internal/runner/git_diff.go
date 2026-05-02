package runner

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"botka/internal/box"
	"botka/internal/models"
)

// MaxDiffBytes caps the size of a returned diff. When the raw `git diff`
// output exceeds this, DiffResult.Diff is truncated and DiffResult.Truncated
// is set to true so the UI can show a "diff too large" banner.
const MaxDiffBytes = 5 * 1024 * 1024

// DiffStats summarizes the per-file insertions/deletions in a diff range.
type DiffStats struct {
	FilesChanged int `json:"files_changed"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
}

// DiffResult is the payload returned by GitDiff and the /tasks/:id/diff
// endpoint. Diff contains the verbatim unified diff (possibly truncated),
// Stats sums the per-file numstat numbers, and Truncated is true when the
// raw diff was larger than MaxDiffBytes.
type DiffResult struct {
	Diff      string    `json:"diff"`
	Stats     DiffStats `json:"stats"`
	Truncated bool      `json:"truncated"`
}

// CommitMissingError indicates that one of the requested SHAs is not present
// in the project's git repository (`git cat-file -e` reported an error). The
// HTTP layer maps this to 404.
type CommitMissingError struct {
	SHA string
}

func (e *CommitMissingError) Error() string {
	return "commit not found in repository: " + e.SHA
}

const gitDiffTimeout = 60 * time.Second

// GitDiff returns the unified diff between two commits in a project's working
// directory along with summary stats. The caller is expected to have already
// validated that base and head are non-empty and not equal (an empty diff is
// returned without error in either case, but the API surface treats those as
// "no diff yet" rather than a real range).
//
// Both SHAs are first verified with `git cat-file -e`; a missing commit is
// reported as a *CommitMissingError. The raw diff is then capped at
// MaxDiffBytes and any per-file numstat lines are summed into DiffStats.
func GitDiff(
	project *models.Project, waker *box.Waker, sshTarget, base, head string,
) (*DiffResult, error) {
	pr := newProjectRunner(project, waker, sshTarget, "")
	ctx, cancel := context.WithTimeout(context.Background(), gitDiffTimeout)
	defer cancel()

	for _, sha := range []string{base, head} {
		if _, err := pr.runGit(ctx, "cat-file", "-e", sha); err != nil {
			return nil, &CommitMissingError{SHA: sha}
		}
	}

	rng := base + ".." + head
	rawDiff, err := pr.runGit(ctx, "diff", rng)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w: %s", err, string(rawDiff))
	}
	numstat, err := pr.runGit(ctx, "diff", "--numstat", rng)
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat: %w: %s", err, string(numstat))
	}

	diff, truncated := capDiff(string(rawDiff), MaxDiffBytes)
	return &DiffResult{
		Diff:      diff,
		Stats:     summarizeNumstat(string(numstat)),
		Truncated: truncated,
	}, nil
}

// capDiff truncates s to maxBytes bytes, returning the truncated string and a
// flag reporting whether truncation occurred.
func capDiff(s string, maxBytes int) (string, bool) {
	if len(s) <= maxBytes {
		return s, false
	}
	return s[:maxBytes], true
}

// summarizeNumstat parses `git diff --numstat` output and aggregates the
// totals. Each line is "<additions>\t<deletions>\t<path>"; binary files are
// reported as "-\t-\t<path>" and contribute to FilesChanged but not to the
// line counts.
func summarizeNumstat(s string) DiffStats {
	var stats DiffStats
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		stats.FilesChanged++
		stats.Insertions += parseStatCount(parts[0])
		stats.Deletions += parseStatCount(parts[1])
	}
	return stats
}

// parseStatCount parses a numstat add/del column, treating "-" (binary) as 0.
func parseStatCount(s string) int {
	if s == "-" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}
