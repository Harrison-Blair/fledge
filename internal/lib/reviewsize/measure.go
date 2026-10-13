package reviewsize

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// FileChange is one changed file from git diff --numstat. A binary file has
// zero added and deleted lines.
type FileChange struct {
	Path    string
	Added   int
	Deleted int
}

// Diff is the size of a change for review.
type Diff struct {
	SourceLines int      // added plus deleted hand-written source lines
	Files       int      // guard-eligible changed files
	CodeDomains []string // sorted domains of changed source and test files
	DocAreas    []string // sorted reference/<area> documentation areas, not counted
	TotalLines  int      // added plus deleted lines of every changed file
}

// Measure sizes the diff from the merge base of base and head to head in the
// Git repository at dir.
func Measure(ctx context.Context, dir, base, head string) (Diff, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--numstat", "--no-renames", "-z", base+"..."+head, "--").Output()
	if err != nil {
		return Diff{}, fmt.Errorf("git diff %s...%s: %w", base, head, err)
	}
	changes, err := parseNumstat(out)
	if err != nil {
		return Diff{}, err
	}
	return Summarize(changes), nil
}

// parseNumstat reads git diff --numstat -z output without renames: each
// record is "added\tdeleted\tpath\0", with "-" counts for a binary file.
func parseNumstat(b []byte) ([]FileChange, error) {
	var changes []FileChange
	for _, rec := range bytes.Split(b, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		f := strings.SplitN(string(rec), "\t", 3)
		if len(f) != 3 {
			return nil, fmt.Errorf("numstat record %q", rec)
		}
		c := FileChange{Path: f[2]}
		if f[0] != "-" || f[1] != "-" {
			var errA, errD error
			c.Added, errA = strconv.Atoi(f[0])
			c.Deleted, errD = strconv.Atoi(f[1])
			if errA != nil || errD != nil {
				return nil, fmt.Errorf("numstat record %q", rec)
			}
		}
		changes = append(changes, c)
	}
	return changes, nil
}

// Summarize sizes a list of changed files.
func Summarize(changes []FileChange) Diff {
	var d Diff
	code, docs := map[string]bool{}, map[string]bool{}
	for _, ch := range changes {
		c := classify(ch.Path)
		lines := ch.Added + ch.Deleted
		d.TotalLines += lines
		if c.source {
			d.SourceLines += lines
		}
		if c.guarded {
			d.Files++
		}
		if c.domain != "" {
			code[c.domain] = true
		}
		if c.docArea != "" {
			docs[c.docArea] = true
		}
	}
	d.CodeDomains, d.DocAreas = sorted(code), sorted(docs)
	return d
}

func sorted(set map[string]bool) []string {
	s := make([]string, 0, len(set))
	for k := range set {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}
