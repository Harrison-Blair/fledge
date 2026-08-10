// Package fswatch reports filesystem changes through the host's native change
// notification API. It exists so the dispatcher and its supervising process
// can both wait on the session ledger and on singleton state without ever
// timing a poll, on every platform Fledge builds for.
package fswatch

import (
	"errors"
	"path/filepath"
	"strings"
)

// Watcher signals that watched entries may have changed. Signals are
// coalesced: one pending signal stands for any number of changes, so a reader
// must always re-read the state it cares about rather than counting events.
// Errors carries at most one terminal error, after which no further signal
// arrives.
type Watcher interface {
	Events() <-chan struct{}
	Errors() <-chan error
	Close() error
}

// Directory signals on any change to a direct entry of dir. Use it when the
// interesting state is not one named file, such as a lock whose release is only
// observable through the neighbouring files its owner removes on exit.
func Directory(dir string) (Watcher, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("watch directory path is empty")
	}
	return open(dir)
}

// File signals on changes to the entry named by path, including its creation
// and removal. It watches the containing directory, so a signal can stand for a
// change to any neighbour; every reader re-reads the state it cares about, so a
// spurious wake is harmless. The containing directory must already exist; the
// file itself need not.
func File(path string) (Watcher, error) {
	name := filepath.Base(path)
	if strings.TrimSpace(path) == "" || name == "." || name == string(filepath.Separator) {
		return nil, errors.New("watch file path does not name an entry")
	}
	return open(filepath.Dir(path))
}
