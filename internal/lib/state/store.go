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

// Create stores the value returned by build under a new random id and returns
// that id. An existing record is never overwritten; colliding ids are retried a
// bounded number of times.
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
		err = writeExclusive(filepath.Join(dir, id+recordSuffix), data)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return id, nil
	}
	return "", fmt.Errorf("state: no free %s id after %d attempts", kind, createAttempts)
}

// Get decodes the record into v. A missing record returns *NotFoundError.
func (s *Store) Get(kind, id string, v any) error {
	path, err := s.recordPath(kind, id)
	if err != nil {
		return err
	}
	return read(path, kind, id, v)
}

// List returns the ids of every record of kind in sorted order. A kind with no
// records yet returns an empty list.
func (s *Store) List(kind string) ([]string, error) {
	dir, err := s.kindDir(kind)
	if err != nil {
		return nil, err
	}
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
		if ok && validID(id) && entry.Type().IsRegular() {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// Update holds the store lock while it decodes the current record into v, runs
// mutate, and atomically writes v back. If mutate fails the record is left
// unchanged. The lock is released before Update returns.
func (s *Store) Update(kind, id string, v any, mutate func() error) error {
	path, err := s.recordPath(kind, id)
	if err != nil {
		return err
	}
	unlock, err := lock(filepath.Join(s.root, lockName))
	if err != nil {
		return err
	}
	defer unlock()
	if err := read(path, kind, id, v); err != nil {
		return err
	}
	if err := mutate(); err != nil {
		return err
	}
	data, err := encode(v)
	if err != nil {
		return fmt.Errorf("state: encode %s: %w", path, err)
	}
	return writeReplace(path, data)
}

// Exclusive holds the store lock while fn runs, serializing fn with every
// other Exclusive and Update on this store directory, in any process. fn may
// call Get, List, and Create, which never take the lock, but must not call
// Update or Exclusive, which would deadlock. Keep fn short: it must not make
// Herdr requests or do other blocking I/O beyond these store reads and creates.
func (s *Store) Exclusive(fn func() error) error {
	unlock, err := lock(filepath.Join(s.root, lockName))
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
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
	if !validID(id) {
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

func validID(id string) bool {
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
