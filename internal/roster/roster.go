// Package roster allocates worker species names (the species half of the
// <role>-<species> naming scheme) and persists current assignments to a
// flock-guarded JSON state file, mirroring internal/spec/ids.go's .alloc.lock
// pattern.
//
// Boundary note: this package works entirely in species-token space and knows
// nothing about roles. Assign returns bare species tokens ("adelie", or
// "adelie-2" once all 18 bases are in use); composing the full <role>-<species>
// name from the token and the caller's role/--pair information is the CLI
// layer's concern (FTHR-054). A pair shares one species across two members, so
// Assign returns that species token twice and Release matches on the token,
// freeing the species only once every member has been released.
package roster

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Species is the canonical, ordered list of the 18 worker species. It is the
// single source of truth for allocation order.
var Species = [18]string{
	"adelie", "emperor", "gentoo", "king", "chinstrap", "little",
	"african", "humboldt", "magellanic", "galapagos", "yelloweyed",
	"fiordland", "snares", "erectcrested", "rockhopper", "royal",
	"macaroni", "northernrockhopper",
}

// Entry is one live species assignment. Members holds the token(s) sharing the
// species (one for a solo worker, two for a pair); Released is a parallel slice
// tracking each member's released state, so the species frees only once every
// member is released.
type Entry struct {
	Species  string   `json:"species"`
	Members  []string `json:"members"`
	Released []bool   `json:"released"`
	Feather  string   `json:"feather"`
}

func (e *Entry) hasUnreleased() bool {
	for _, r := range e.Released {
		if !r {
			return true
		}
	}
	return false
}

// stateFileName is the JSON state file; lockName is its sibling flock file. A
// dedicated roster dir (chosen by the caller) keeps .fledge/broods/'s scope of
// lock records clean.
const (
	stateFileName = "roster.json"
	lockName      = ".roster.lock"
)

// Assign reserves the first unused species for feather and returns its
// token(s): one for a solo worker, two identical tokens for a pair. It scans
// live entries under an exclusive flock; if every one of the 18 base species
// has an unreleased member, it overflows to the first unused numeric-suffixed
// variant ("adelie-2", "emperor-2", ..., "adelie-3", ...).
func Assign(dir, feather string, pair bool) ([]string, error) {
	unlock, err := lockRosterDir(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()

	entries, err := load(dir)
	if err != nil {
		return nil, err
	}

	token := firstFreeToken(entries)
	count := 1
	if pair {
		count = 2
	}
	members := make([]string, count)
	released := make([]bool, count)
	for i := range members {
		members[i] = token
	}

	entries = append(entries, Entry{
		Species:  token,
		Members:  members,
		Released: released,
		Feather:  feather,
	})
	if err := save(dir, entries); err != nil {
		return nil, err
	}
	return members, nil
}

// Release marks one member named token as released. If that was the last
// unreleased member of its entry, the entry is removed and the species becomes
// available again. It errors if no unreleased member matches.
func Release(dir, name string) error {
	unlock, err := lockRosterDir(dir)
	if err != nil {
		return err
	}
	defer unlock()

	entries, err := load(dir)
	if err != nil {
		return err
	}

	for i := range entries {
		e := &entries[i]
		for j, m := range e.Members {
			if m == name && !e.Released[j] {
				e.Released[j] = true
				if !e.hasUnreleased() {
					entries = append(entries[:i], entries[i+1:]...)
				}
				return save(dir, entries)
			}
		}
	}
	return fmt.Errorf("roster: no unreleased member named %q", name)
}

// List returns all live (non-fully-released) entries. Since fully-released
// entries are removed on Release, every persisted entry is live.
func List(dir string) ([]Entry, error) {
	unlock, err := lockRosterDir(dir)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return load(dir)
}

// firstFreeToken returns the lowest unused species token, scanning suffix
// level first then canonical species order: bare species (level 1) before any
// "-2" (level 2), and within a level in Species order.
func firstFreeToken(entries []Entry) string {
	live := make(map[string]bool)
	for i := range entries {
		if entries[i].hasUnreleased() {
			live[entries[i].Species] = true
		}
	}
	for level := 1; ; level++ {
		for _, sp := range Species {
			tok := sp
			if level > 1 {
				tok = fmt.Sprintf("%s-%d", sp, level)
			}
			if !live[tok] {
				return tok
			}
		}
	}
}

// load reads the roster state file; a missing file is an empty roster.
func load(dir string) ([]Entry, error) {
	b, err := os.ReadFile(filepath.Join(dir, stateFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, fmt.Errorf("roster: corrupt state file: %w", err)
	}
	return entries, nil
}

// save writes the roster state file. All callers hold the flock.
func save(dir string, entries []Entry) error {
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, stateFileName), b, 0o644)
}

// lockRosterDir acquires an exclusive flock on dir's roster lock file, creating
// dir and the lock file if absent, and returns a func that releases the lock
// and closes the file. Mirrors internal/spec/ids.go's lockAllocDir.
func lockRosterDir(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
