package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// archiveDir holds archived records beneath their kind directory. List skips
// it as it skips every directory; Get and Tx.Put still find its records.
const archiveDir = "archive"

// Tx reads and writes records of its store while Exclusive holds the store
// lock. It is valid only until the Exclusive fn that received it returns.
type Tx struct{ s *Store }

// Get is Store.Get under the lock.
func (tx *Tx) Get(kind, id string, v any) error { return tx.s.Get(kind, id, v) }

// List is Store.List under the lock.
func (tx *Tx) List(kind string) ([]string, error) { return tx.s.List(kind) }

// Create is Store.Create under the lock.
func (tx *Tx) Create(kind string, build func(id string) any) (string, error) {
	return tx.s.Create(kind, build)
}

// Put atomically replaces the existing record id with v, in the archive when
// the record is archived. A missing record returns *NotFoundError.
func (tx *Tx) Put(kind, id string, v any) error {
	path, err := tx.locate(kind, id)
	if err != nil {
		return err
	}
	data, err := encode(v)
	if err != nil {
		return fmt.Errorf("state: encode %s: %w", path, err)
	}
	return writeReplace(path, data)
}

// Archive moves record id out of List's view into the archive, keeping it
// readable by id. An archived record is left as is; a missing one returns
// *NotFoundError. It never replaces an archived record of the same id.
func (tx *Tx) Archive(kind, id string) error {
	live, err := tx.s.recordPath(kind, id)
	if err != nil {
		return err
	}
	path, err := tx.locate(kind, id)
	if err != nil || path != live {
		return err
	}
	dir := filepath.Dir(archivePath(live))
	if err := ensureDir(dir); err != nil {
		return fmt.Errorf("state: create %s: %w", dir, err)
	}
	// Sync even when the archive already existed, in case its creator has
	// not synced it into the kind directory yet.
	if err := syncDir(filepath.Dir(live)); err != nil {
		return err
	}
	return move(live, archivePath(live))
}

// Unarchive returns archived record id to List's view. A record that is not
// archived is left as is; a missing one returns *NotFoundError.
func (tx *Tx) Unarchive(kind, id string) error {
	live, err := tx.s.recordPath(kind, id)
	if err != nil {
		return err
	}
	path, err := tx.locate(kind, id)
	if err != nil || path == live {
		return err
	}
	return move(path, live)
}

// move renames from to to without replacing an existing to, which fails with
// an error matching fs.ErrExist. A to that is already from's file, left by an
// interrupted move, completes the move. The new link is synced before the old
// one is removed, so a crash never loses the record.
func move(from, to string) error {
	if err := os.Link(from, to); errors.Is(err, fs.ErrExist) {
		a, aerr := os.Stat(from)
		b, berr := os.Stat(to)
		if aerr != nil || berr != nil || !os.SameFile(a, b) {
			return fmt.Errorf("state: move %s: %w", from, err)
		}
	} else if err != nil {
		return fmt.Errorf("state: move %s: %w", from, err)
	}
	if err := syncDir(filepath.Dir(to)); err != nil {
		return err
	}
	if err := os.Remove(from); err != nil {
		return fmt.Errorf("state: move %s: %w", from, err)
	}
	return syncDir(filepath.Dir(from))
}

// locate returns the path of record id, live or archived.
func (tx *Tx) locate(kind, id string) (string, error) {
	live, err := tx.s.recordPath(kind, id)
	if err != nil {
		return "", err
	}
	for _, path := range []string{live, archivePath(live)} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("state: read %s: %w", path, err)
		}
	}
	return "", &NotFoundError{Kind: kind, ID: id}
}

func archivePath(live string) string {
	return filepath.Join(filepath.Dir(live), archiveDir, filepath.Base(live))
}
