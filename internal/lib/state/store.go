package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

const (
	lockName       = "lock"
	recordSuffix   = ".json"
	createAttempts = 8
)

// Store keeps JSON records under a root directory, one file per record at
// <root>/<kind>/<id>.json.
type Store struct {
	root  string
	newID func() (string, error)
}

// NotFoundError reports that a record does not exist.
type NotFoundError struct {
	Kind string
	ID   string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("state: %s record %s not found", e.Kind, e.ID)
}

// Open prepares root as a state directory, creating it and its lock file when
// missing. The parent of root must already exist; Open never creates it. It
// does not resolve Git roots or write ignore files.
func Open(root string) (*Store, error) {
	if err := ensureDir(root); err != nil {
		return nil, fmt.Errorf("state: create %s: %w", root, err)
	}
	lock, err := os.OpenFile(filepath.Join(root, lockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("state: create lock: %w", err)
	}
	if err := lock.Close(); err != nil {
		return nil, fmt.Errorf("state: create lock: %w", err)
	}
	// Sync even when the entries already existed: another process may have
	// created them without having synced yet.
	if err := syncDir(filepath.Dir(root)); err != nil {
		return nil, err
	}
	if err := syncDir(root); err != nil {
		return nil, err
	}
	return &Store{root: root, newID: randomID}, nil
}

// OpenExisting opens root for lookups without creating anything, not even the
// lock file. It fails with an error matching fs.ErrNotExist when root is
// missing, or syscall.ENOTDIR when root is not a directory. Update, Exclusive,
// and Create still work on the returned store; they create the lock file or
// kind directory as needed, as on a store from Open.
func OpenExisting(root string) (*Store, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("state: open %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("state: open %s: %w", root, syscall.ENOTDIR)
	}
	return &Store{root: root, newID: randomID}, nil
}

// Create stores the value returned by build under a new random id and returns
// that id. An existing record, live or archived, is never overwritten or
// shadowed; colliding ids are retried a bounded number of times. Create
// claims the live path, then gives it back if the id is archived. That is
// race-free with or without the lock: Archive and Unarchive link a record's
// new path before removing its old one, so a record is always at one of the
// two, and one archived after the claim was live and so blocked the claim.
func (s *Store) Create(kind string, build func(id string) any) (string, error) {
	dir, err := s.kindDir(kind)
	if err != nil {
		return "", err
	}
	if err := ensureDir(dir); err != nil {
		return "", fmt.Errorf("state: create %s: %w", dir, err)
	}
	// Sync even when the kind directory already existed, in case its creator
	// has not synced it into root yet.
	if err := syncDir(s.root); err != nil {
		return "", err
	}
	for range createAttempts {
		id, err := s.newID()
		if err != nil {
			return "", fmt.Errorf("state: generate id: %w", err)
		}
		data, err := encode(build(id))
		if err != nil {
			return "", fmt.Errorf("state: encode %s record %s: %w", kind, id, err)
		}
		path := filepath.Join(dir, id+recordSuffix)
		err = writeExclusive(path, data)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		createStep()
		if _, err := os.Stat(archivePath(path)); errors.Is(err, fs.ErrNotExist) {
			return id, nil
		} else if err != nil {
			return "", fmt.Errorf("state: create %s: %w", path, err)
		}
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("state: create %s: %w", path, err)
		}
	}
	return "", fmt.Errorf("state: no free %s id after %d attempts", kind, createAttempts)
}

// Get decodes the record into v, live or archived. A missing record returns
// *NotFoundError. The live path is read first: archiving moves a record from
// there under the lock, so a lookup racing it still finds the archived copy.
func (s *Store) Get(kind, id string, v any) error {
	path, err := s.recordPath(kind, id)
	if err != nil {
		return err
	}
	err = read(path, kind, id, v)
	var missing *NotFoundError
	if errors.As(err, &missing) {
		return read(archivePath(path), kind, id, v)
	}
	return err
}

// List returns the ids of every unarchived record of kind in sorted order. A
// kind with no records yet returns an empty list.
func (s *Store) List(kind string) ([]string, error) {
	dir, err := s.kindDir(kind)
	if err != nil {
		return nil, err
	}
	return listDir(dir)
}

// ListArchived returns the ids of every archived record of kind in sorted
// order, for readers that need ended records too. A record whose archiving
// was interrupted can appear in both List and ListArchived.
func (s *Store) ListArchived(kind string) ([]string, error) {
	dir, err := s.kindDir(kind)
	if err != nil {
		return nil, err
	}
	return listDir(filepath.Join(dir, archiveDir))
}

// listDir returns the sorted record ids in dir, or none when dir is missing.
func listDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state: list %s: %w", dir, err)
	}
	ids := []string{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), recordSuffix)
		if ok && ValidID(id) && entry.Type().IsRegular() {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// Update holds the store lock while it decodes the current record into v, runs
// mutate, and atomically writes v back, in the archive when the record is
// archived. If mutate fails the record is left unchanged. The lock is released
// before Update returns.
func (s *Store) Update(kind, id string, v any, mutate func() error) error {
	return s.Exclusive(func(tx *Tx) error {
		if err := tx.Get(kind, id, v); err != nil {
			return err
		}
		if err := mutate(); err != nil {
			return err
		}
		return tx.Put(kind, id, v)
	})
}

// Exclusive holds the store lock while fn runs, serializing fn with every
// other Exclusive and Update on this store directory, in any process. fn reads
// and writes through tx; it must not call Update or Exclusive, which would
// deadlock on the lock it holds. Keep fn short: it must not make Herdr
// requests or do other blocking I/O beyond these store operations.
func (s *Store) Exclusive(fn func(tx *Tx) error) error {
	unlock, err := lock(filepath.Join(s.root, lockName))
	if err != nil {
		return err
	}
	defer unlock()
	return fn(&Tx{s: s})
}

func (s *Store) kindDir(kind string) (string, error) {
	if kind == "" || kind == "." || kind == ".." || strings.ContainsAny(kind, `/\`) {
		return "", fmt.Errorf("state: invalid kind %q", kind)
	}
	return filepath.Join(s.root, kind), nil
}

func (s *Store) recordPath(kind, id string) (string, error) {
	dir, err := s.kindDir(kind)
	if err != nil {
		return "", err
	}
	if !ValidID(id) {
		return "", fmt.Errorf("state: invalid id %q", id)
	}
	return filepath.Join(dir, id+recordSuffix), nil
}

func read(path, kind, id string, v any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &NotFoundError{Kind: kind, ID: id}
	}
	if err != nil {
		return fmt.Errorf("state: read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("state: decode %s: %w", path, err)
	}
	return nil
}

func encode(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func randomID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// ValidID reports whether id is 8 lowercase hexadecimal characters, the form of
// every record id.
func ValidID(id string) bool {
	if len(id) != 8 {
		return false
	}
	for _, c := range id {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}
