// Package scan lists repository files (tracked + untracked, non-gitignored),
// filters them through .fledge/scan-ignore, and groups them into modules by
// top-level directory. Root-level files group under "<root>".
package scan

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Module is one top-level grouping of files.
type Module struct {
	Name  string   `json:"name"`
	Files []string `json:"files"`
	Count int      `json:"count"`
	Bytes int64    `json:"bytes"`
}

// Result of a scan.
type Result struct {
	Commit      string   `json:"commit"`       // full HEAD sha, "" when no commits
	ShortCommit string   `json:"short_commit"` // short sha, "none" when no commits
	Modules     []Module `json:"modules"`
}

// Run scans the repository rooted at root.
func Run(root string) (*Result, error) {
	files, err := listFiles(root)
	if err != nil {
		return nil, err
	}
	files, err = filterIgnored(root, files)
	if err != nil {
		return nil, err
	}

	res := &Result{ShortCommit: "none"}
	if out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); err == nil {
		res.Commit = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD").Output(); err == nil {
		res.ShortCommit = strings.TrimSpace(string(out))
	}

	byModule := map[string][]string{}
	var names []string
	for _, p := range files {
		m := "<root>"
		if i := strings.IndexByte(p, '/'); i >= 0 {
			m = p[:i]
		}
		if _, seen := byModule[m]; !seen {
			names = append(names, m)
		}
		byModule[m] = append(byModule[m], p)
	}
	sort.Strings(names)
	for _, name := range names {
		mf := byModule[name]
		var size int64
		for _, p := range mf {
			// A vanished/staged-deleted file must not abort the scan.
			if info, err := os.Lstat(filepath.Join(root, p)); err == nil {
				size += info.Size()
			}
		}
		res.Modules = append(res.Modules, Module{Name: name, Files: mf, Count: len(mf), Bytes: size})
	}
	return res, nil
}

// listFiles returns tracked + untracked (non-gitignored) paths, byte-sorted.
func listFiles(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "-c", "core.quotePath=false",
		"ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) > 0 {
			files = append(files, string(p))
		}
	}
	sort.Strings(files)
	return files, nil
}

// filterIgnored drops paths matched by .fledge/scan-ignore, if present.
func filterIgnored(root string, files []string) ([]string, error) {
	ignorePath := filepath.Join(root, ".fledge", "scan-ignore")
	if _, err := os.Stat(ignorePath); err != nil || len(files) == 0 {
		return files, nil
	}
	cmd := exec.Command("git", "-C", root, "-c", "core.excludesFile="+ignorePath,
		"check-ignore", "--no-index", "--stdin", "-z")
	cmd.Stdin = strings.NewReader(strings.Join(files, "\x00") + "\x00")
	out, err := cmd.Output()
	if err != nil {
		// exit 1 = no paths ignored; anything else is a real failure
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return files, nil
		}
		return nil, err
	}
	ignored := map[string]bool{}
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) > 0 {
			ignored[string(p)] = true
		}
	}
	var kept []string
	for _, p := range files {
		if !ignored[p] {
			kept = append(kept, p)
		}
	}
	return kept, nil
}
