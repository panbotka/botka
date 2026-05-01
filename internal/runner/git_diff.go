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

// DiffFile describes a single file changed between two commits.
type DiffFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// DiffResult is the combined output of `git diff` for a task: per-file
// metadata plus the raw unified diff.
type DiffResult struct {
	BaseCommitSHA string     `json:"base_commit_sha"`
	HeadCommitSHA string     `json:"head_commit_sha"`
	Files         []DiffFile `json:"files"`
	Diff          string     `json:"diff"`
}

const gitDiffTimeout = 60 * time.Second

// GitDiff computes the unified diff between two commits in the project's
// working directory. It runs three git commands (--name-status, --numstat,
// and the raw diff) and merges the per-file metadata. For remote projects
// the commands are dispatched via SSH.
func GitDiff(
	project *models.Project, waker *box.Waker, sshTarget, base, head string,
) (*DiffResult, error) {
	pr := newProjectRunner(project, waker, sshTarget, "")
	ctx, cancel := context.WithTimeout(context.Background(), gitDiffTimeout)
	defer cancel()

	rng := base + ".." + head
	rawDiff, err := pr.runGit(ctx, "diff", "-M", rng)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w: %s", err, string(rawDiff))
	}
	nameStatus, err := pr.runGit(ctx, "diff", "--name-status", "-M", rng)
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status: %w: %s", err, string(nameStatus))
	}
	numstat, err := pr.runGit(ctx, "diff", "--numstat", "-M", rng)
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat: %w: %s", err, string(numstat))
	}

	files := mergeDiffFiles(parseNameStatus(string(nameStatus)), parseNumstat(string(numstat)))
	return &DiffResult{
		BaseCommitSHA: base,
		HeadCommitSHA: head,
		Files:         files,
		Diff:          string(rawDiff),
	}, nil
}

type nameStatusEntry struct {
	Status  string
	Path    string
	OldPath string
}

type numstatEntry struct {
	Additions int
	Deletions int
}

// parseNameStatus parses `git diff --name-status -M` line-by-line. Each line
// is "<status>\t<path>" or "R<N>\t<old>\t<new>" / "C<N>\t<old>\t<new>".
func parseNameStatus(s string) []nameStatusEntry {
	var entries []nameStatusEntry
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		code := parts[0]
		e := nameStatusEntry{Status: mapStatusCode(code)}
		if (strings.HasPrefix(code, "R") || strings.HasPrefix(code, "C")) && len(parts) >= 3 {
			e.OldPath = parts[1]
			e.Path = parts[2]
		} else {
			e.Path = parts[1]
		}
		entries = append(entries, e)
	}
	return entries
}

// parseNumstat parses `git diff --numstat -M`. Each line is
// "<additions>\t<deletions>\t<path>". Binary files are reported as "-\t-\tpath".
// Order matches `--name-status`, so the caller merges by index.
func parseNumstat(s string) []numstatEntry {
	var entries []numstatEntry
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		entries = append(entries, numstatEntry{
			Additions: parseStatCount(parts[0]),
			Deletions: parseStatCount(parts[1]),
		})
	}
	return entries
}

func parseStatCount(s string) int {
	if s == "-" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

// mergeDiffFiles combines name-status and numstat output. Both commands emit
// files in the same order, so they merge by index. If the lengths disagree
// (e.g. binary files in some git versions), missing numstat entries default
// to zero counts.
func mergeDiffFiles(ns []nameStatusEntry, num []numstatEntry) []DiffFile {
	files := make([]DiffFile, 0, len(ns))
	for i, e := range ns {
		f := DiffFile{
			Path:    e.Path,
			OldPath: e.OldPath,
			Status:  e.Status,
		}
		if i < len(num) {
			f.Additions = num[i].Additions
			f.Deletions = num[i].Deletions
		}
		files = append(files, f)
	}
	return files
}

// mapStatusCode translates git's single-character status code (with optional
// similarity score for renames/copies) into a descriptive string.
func mapStatusCode(code string) string {
	if code == "" {
		return "modified"
	}
	switch code[0] {
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'M':
		return "modified"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'T':
		return "typechange"
	default:
		return "modified"
	}
}
