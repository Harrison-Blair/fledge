// Package lock implements advisory feather claim (brood) files under .fledge/broods/.
package lock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Record is the JSON content of one .fledge/broods/<FTHR-ID>.brood file.
type Record struct {
	Task     string `json:"feather"`
	Owner    string `json:"owner"`
	PID      int    `json:"pid"`
	Created  string `json:"created"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
}

// HeldError reports an acquisition conflict with the current holder.
type HeldError struct{ Existing Record }

func (e *HeldError) Error() string {
	return fmt.Sprintf("brood already held by %s since %s", e.Existing.Owner, e.Existing.Created)
}

func lockPath(dir, task string) string { return filepath.Join(dir, task+".brood") }

// Acquire atomically creates the brood file; *HeldError if already held.
//
// The record is written to a temp file in dir and placed via os.Link, which
// fails with EEXIST when the claim is already held (preserving the previous
// O_EXCL exclusivity guard) and otherwise makes the fully-written file appear
// in one atomic step, so a reader never observes a partial or zero-length
// .brood file.
func Acquire(dir string, rec Record) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".fledge-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Link(tmpName, lockPath(dir, rec.Task)); err != nil {
		cleanup()
		if os.IsExist(err) {
			if existing, gerr := Get(dir, rec.Task); gerr == nil {
				return &HeldError{Existing: *existing}
			}
			return &HeldError{Existing: Record{Task: rec.Task, Owner: "unknown"}}
		}
		return err
	}
	cleanup()
	return nil
}

// Release removes the brood file; errors if not held.
func Release(dir, task string) error {
	err := os.Remove(lockPath(dir, task))
	if os.IsNotExist(err) {
		return fmt.Errorf("%s is not brooded", task)
	}
	return err
}

// Get reads one brood record.
func Get(dir, task string) (*Record, error) {
	b, err := os.ReadFile(lockPath(dir, task))
	if err != nil {
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, fmt.Errorf("corrupt brood file for %s: %w", task, err)
	}
	return &rec, nil
}

// List returns all held broods sorted by feather ID; empty when dir is
// missing. An individual .brood file that fails to parse is skipped rather
// than aborting the whole listing; its filename is returned in skipped
// (sorted) so callers can surface the corruption instead of swallowing it.
func List(dir string) (out []Record, skipped []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".brood") {
			continue
		}
		rec, gerr := Get(dir, strings.TrimSuffix(e.Name(), ".brood"))
		if gerr != nil {
			skipped = append(skipped, e.Name())
			continue
		}
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Task < out[j].Task })
	sort.Strings(skipped)
	return out, skipped, nil
}
