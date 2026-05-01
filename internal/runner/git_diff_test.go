package runner

import (
	"testing"
)

func TestParseNameStatus(t *testing.T) {
	in := "M\tfoo.go\nA\tnew/bar.go\nD\tbaz.go\nR100\told.go\tnew.go\n"
	got := parseNameStatus(in)
	if len(got) != 4 {
		t.Fatalf("want 4 entries, got %d: %+v", len(got), got)
	}
	if got[0].Path != "foo.go" || got[0].Status != "modified" {
		t.Errorf("entry 0: %+v", got[0])
	}
	if got[1].Path != "new/bar.go" || got[1].Status != "added" {
		t.Errorf("entry 1: %+v", got[1])
	}
	if got[2].Path != "baz.go" || got[2].Status != "deleted" {
		t.Errorf("entry 2: %+v", got[2])
	}
	if got[3].Path != "new.go" || got[3].OldPath != "old.go" || got[3].Status != "renamed" {
		t.Errorf("entry 3: %+v", got[3])
	}
}

func TestParseNameStatusEmpty(t *testing.T) {
	if got := parseNameStatus(""); len(got) != 0 {
		t.Errorf("want empty, got %v", got)
	}
	if got := parseNameStatus("\n"); len(got) != 0 {
		t.Errorf("want empty, got %v", got)
	}
}

func TestParseNumstat(t *testing.T) {
	in := "3\t1\tfoo.go\n0\t10\tbar.go\n-\t-\timg.png\n"
	got := parseNumstat(in)
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d: %+v", len(got), got)
	}
	if got[0].Additions != 3 || got[0].Deletions != 1 {
		t.Errorf("entry 0: %+v", got[0])
	}
	if got[1].Additions != 0 || got[1].Deletions != 10 {
		t.Errorf("entry 1: %+v", got[1])
	}
	if got[2].Additions != 0 || got[2].Deletions != 0 {
		t.Errorf("entry 2 (binary): %+v", got[2])
	}
}

func TestMergeDiffFiles(t *testing.T) {
	ns := []nameStatusEntry{
		{Status: "modified", Path: "foo.go"},
		{Status: "added", Path: "bar.go"},
	}
	num := []numstatEntry{
		{Additions: 5, Deletions: 2},
		{Additions: 10, Deletions: 0},
	}
	got := mergeDiffFiles(ns, num)
	if len(got) != 2 {
		t.Fatalf("want 2 files, got %d", len(got))
	}
	if got[0].Path != "foo.go" || got[0].Additions != 5 || got[0].Deletions != 2 || got[0].Status != "modified" {
		t.Errorf("entry 0: %+v", got[0])
	}
	if got[1].Path != "bar.go" || got[1].Additions != 10 || got[1].Status != "added" {
		t.Errorf("entry 1: %+v", got[1])
	}
}

func TestMergeDiffFilesLengthMismatch(t *testing.T) {
	ns := []nameStatusEntry{
		{Status: "modified", Path: "foo.go"},
		{Status: "added", Path: "bar.go"},
	}
	num := []numstatEntry{{Additions: 5, Deletions: 2}}
	got := mergeDiffFiles(ns, num)
	if len(got) != 2 {
		t.Fatalf("want 2 files, got %d", len(got))
	}
	if got[1].Additions != 0 || got[1].Deletions != 0 {
		t.Errorf("entry 1 should default to zero: %+v", got[1])
	}
}

func TestMapStatusCode(t *testing.T) {
	cases := map[string]string{
		"A":    "added",
		"D":    "deleted",
		"M":    "modified",
		"R100": "renamed",
		"C75":  "copied",
		"T":    "typechange",
		"":     "modified",
		"X":    "modified",
	}
	for in, want := range cases {
		if got := mapStatusCode(in); got != want {
			t.Errorf("mapStatusCode(%q) = %q, want %q", in, got, want)
		}
	}
}
