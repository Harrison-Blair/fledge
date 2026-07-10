package bootstrap

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DriftStatus classifies one scaffold file's on-disk state.
type DriftStatus string

const (
	// StatusUpToDate: disk bytes (or symlink target) match the expected entry.
	StatusUpToDate DriftStatus = "up-to-date"
	// StatusStale: disk matches the stamp hash/target but the embedded binary
	// has since changed — provably unedited, refresh-safe.
	StatusStale DriftStatus = "stale"
	// StatusModified: disk differs from both the stamp and the expected entry —
	// user has edited the file.
	StatusModified DriftStatus = "modified"
	// StatusMissing: file (or a required append line) is absent from disk.
	StatusMissing DriftStatus = "missing"
	// StatusObsolete: entry is in the stamp but no longer in the expected set —
	// the binary no longer ships this file.
	StatusObsolete DriftStatus = "obsolete"
)

// Drift is one classified scaffold entry.
type Drift struct {
	Path   string      `json:"path"`
	Status DriftStatus `json:"status"`
	Policy string      `json:"policy"`
}

// DriftReport compares the on-disk state under root to the stamp and expected
// trees. stamp may be nil (the no-stamp path is handled at the call site).
// expected is the output of ExpectedFiles for the adapters in the repo.
//
// For each path in the union of stamp.Files and expected:
//   - obsolete: in stamp only (expected no longer ships it)
//   - up-to-date: disk matches expected bytes/target/lines
//   - stale: disk matches stamp hash/target but expected has moved (refresh-safe)
//   - modified: disk differs from both
//   - missing: file absent or required append line absent
func DriftReport(root string, stamp *Stamp, expected map[string]StampEntry) []Drift {
	// Build the union of all known paths.
	paths := make(map[string]bool, len(expected))
	for p := range expected {
		paths[p] = true
	}
	if stamp != nil {
		for p := range stamp.Files {
			paths[p] = true
		}
	}

	out := make([]Drift, 0, len(paths))
	for p := range paths {
		exp, inExpected := expected[p]

		var stEntry StampEntry
		inStamp := false
		if stamp != nil {
			stEntry, inStamp = stamp.Files[p]
		}

		if !inExpected {
			// Stamp-only: this path is no longer shipped.
			out = append(out, Drift{Path: p, Status: StatusObsolete, Policy: stEntry.Policy})
			continue
		}

		disk := filepath.Join(root, filepath.FromSlash(p))

		var d Drift
		switch {
		case exp.Target != "":
			d = classifySymlink(p, disk, exp, stEntry, inStamp)
		case exp.Lines != nil:
			d = classifyAppend(p, disk, exp)
		default:
			d = classifyContent(p, disk, exp, stEntry, inStamp)
		}
		out = append(out, d)
	}
	return out
}

// classifyContent classifies a content-bearing (sha256) entry.
func classifyContent(path, disk string, exp, stamp StampEntry, hasStamp bool) Drift {
	data, err := os.ReadFile(disk)
	if err != nil {
		return Drift{Path: path, Status: StatusMissing, Policy: exp.Policy}
	}
	h := sha256.Sum256(data)
	diskHash := fmt.Sprintf("%x", h)
	if diskHash == exp.Sha256 {
		return Drift{Path: path, Status: StatusUpToDate, Policy: exp.Policy}
	}
	if hasStamp && stamp.Sha256 != "" && diskHash == stamp.Sha256 {
		return Drift{Path: path, Status: StatusStale, Policy: exp.Policy}
	}
	return Drift{Path: path, Status: StatusModified, Policy: exp.Policy}
}

// classifySymlink classifies a symlink entry.
// A non-symlink where one is expected reports modified (Windows degradation
// path — never errors).
func classifySymlink(path, disk string, exp, stamp StampEntry, hasStamp bool) Drift {
	cur, err := os.Readlink(disk)
	if err != nil {
		if os.IsNotExist(err) {
			return Drift{Path: path, Status: StatusMissing, Policy: exp.Policy}
		}
		// Regular file or directory where symlink expected → modified.
		return Drift{Path: path, Status: StatusModified, Policy: exp.Policy}
	}
	curSlash := filepath.ToSlash(cur)
	if curSlash == exp.Target {
		return Drift{Path: path, Status: StatusUpToDate, Policy: exp.Policy}
	}
	if hasStamp && stamp.Target != "" && curSlash == stamp.Target {
		return Drift{Path: path, Status: StatusStale, Policy: exp.Policy}
	}
	return Drift{Path: path, Status: StatusModified, Policy: exp.Policy}
}

// classifyAppend classifies an append_if_missing entry by checking whether all
// required lines are present in the file. Any missing line → StatusMissing.
func classifyAppend(path, disk string, exp StampEntry) Drift {
	data, err := os.ReadFile(disk)
	if err != nil {
		return Drift{Path: path, Status: StatusMissing, Policy: exp.Policy}
	}
	content := string(data)
	for _, line := range exp.Lines {
		if !appendLinePresent(content, line) {
			return Drift{Path: path, Status: StatusMissing, Policy: exp.Policy}
		}
	}
	return Drift{Path: path, Status: StatusUpToDate, Policy: exp.Policy}
}

// appendLinePresent reports whether line is present in content (trim-matched,
// line by line), matching the ensureLine logic in registry.go.
func appendLinePresent(content, line string) bool {
	s := bufio.NewScanner(strings.NewReader(content))
	want := strings.TrimSpace(line)
	for s.Scan() {
		if strings.TrimSpace(s.Text()) == want {
			return true
		}
	}
	return false
}
