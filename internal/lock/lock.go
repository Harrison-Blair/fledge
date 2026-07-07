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
	Task    string `json:"feather"`
	Owner   string `json:"owner"`
	PID     int    `json:"pid"`
	Created string `json:"created"`
	Branch  string `json:"branch"`
}

// HeldError reports an acquisition conflict with the current holder.
type HeldError struct{ Existing Record }

func (e *HeldError) Error() string {
	return fmt.Sprintf("brood already held by %s since %s", e.Existing.Owner, e.Existing.Created)
}

func lockPath(dir, task string) string { return filepath.Join(dir, task+".brood") }

// Acquire atomically creates the brood file; *HeldError if already held.
func Acquire(dir string, rec Record) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(lockPath(dir, rec.Task), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			if existing, gerr := Get(dir, rec.Task); gerr == nil {
				return &HeldError{Existing: *existing}
			}
			return &HeldError{Existing: Record{Task: rec.Task, Owner: "unknown"}}
		}
		return err
	}
	enc := json.NewEncoder(f)
	if err := enc.Encode(rec); err != nil {
		f.Close()
		os.Remove(lockPath(dir, rec.Task))
		return err
	}
	return f.Close()
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

// List returns all held broods sorted by feather ID; empty when dir is missing.
func List(dir string) ([]Record, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".brood") {
			continue
		}
		rec, err := Get(dir, strings.TrimSuffix(e.Name(), ".brood"))
		if err != nil {
			return nil, err
		}
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Task < out[j].Task })
	return out, nil
}
