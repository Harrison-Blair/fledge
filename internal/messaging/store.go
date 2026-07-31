package messaging

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

var (
	ErrUnavailable = errors.New("message log unavailable")
	ErrCorrupt     = errors.New("message log corrupt")
)

type Store struct {
	ProjectRoot string
	Dir         string
	Now         func() time.Time
}

// WithLifecycleLock serializes multi-event lifecycle transitions such as
// final unresolved failures followed by run.closed.
func (s *Store) WithLifecycleLock(runID string, fn func() error) error {
	if !validRunID(runID) {
		return fmt.Errorf("%w: invalid run ID", ErrUnavailable)
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	ran := false
	err := fsutil.WithFlock(s.runPath(runID)+".lifecycle.lock", func() error {
		ran = true
		return fn()
	})
	if !ran && errors.Is(err, fsutil.ErrLock) {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return err
}

func NewStore(projectRoot string) *Store {
	return &Store{
		ProjectRoot: projectRoot,
		Dir:         filepath.Join(projectRoot, ".fledge", "logs", "agents"),
		Now:         time.Now,
	}
}

func (s *Store) Ensure() error {
	for _, dir := range []string{filepath.Dir(s.Dir), s.Dir} {
		if info, err := os.Lstat(dir); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("%w: %s is not a real directory", ErrUnavailable, dir)
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: inspect log directory: %v", ErrUnavailable, err)
		}
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("%w: create log directory: %v", ErrUnavailable, err)
	}
	for _, dir := range []string{filepath.Dir(s.Dir), s.Dir} {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("%w: secure log directory: %v", ErrUnavailable, err)
		}
	}
	return nil
}

func NewID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:])
	return prefix + strings.ToLower(encoded), nil
}

func (s *Store) StartRun(header RunHeader) (string, error) {
	if err := s.Ensure(); err != nil {
		return "", err
	}
	runID, err := NewID("run_")
	if err != nil {
		return "", fmt.Errorf("%w: generate run ID: %v", ErrUnavailable, err)
	}
	path := s.runPath(runID)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return "", fmt.Errorf("%w: create run log: %v", ErrUnavailable, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("%w: secure run log: %v", ErrUnavailable, err)
	}
	_ = file.Close()
	if header.StartedAt.IsZero() {
		header.StartedAt = s.now()
	}
	if _, err := s.Append(runID, Event{Type: EventRunStarted, Header: &header}); err != nil {
		_ = os.Remove(path)
		_ = os.Remove(path + ".lock")
		return "", err
	}
	if dir, err := os.Open(s.Dir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return runID, nil
}

func (s *Store) Append(runID string, event Event) (Event, error) {
	if !validRunID(runID) {
		return Event{}, fmt.Errorf("%w: invalid run ID", ErrUnavailable)
	}
	if err := s.Ensure(); err != nil {
		return Event{}, err
	}
	var appended Event
	ran := false
	err := fsutil.WithFlock(s.runPath(runID)+".lock", func() error {
		ran = true
		var err error
		appended, err = s.appendLocked(runID, event)
		return err
	})
	if !ran && errors.Is(err, fsutil.ErrLock) {
		return Event{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return appended, err
}

func (s *Store) appendLocked(runID string, event Event) (Event, error) {
	file, err := os.OpenFile(s.runPath(runID), os.O_RDWR, 0o600)
	if err != nil {
		return Event{}, fmt.Errorf("%w: open run log: %v", ErrUnavailable, err)
	}
	defer file.Close()
	events, err := readAndRepair(file, runID)
	if err != nil {
		return Event{}, err
	}
	if (len(events) == 0) != (event.Type == EventRunStarted) {
		return Event{}, fmt.Errorf("%w: run header must be the first and only run.started event", ErrCorrupt)
	}
	event.SchemaVersion = SchemaVersion
	event.RunID = runID
	event.Sequence = uint64(len(events) + 1)
	event.Timestamp = event.Timestamp.UTC()
	if event.Timestamp.IsZero() {
		event.Timestamp = s.now()
	}
	if event.ID == "" {
		event.ID, err = NewID("evt_")
		if err != nil {
			return Event{}, fmt.Errorf("%w: generate event ID: %v", ErrUnavailable, err)
		}
	}
	data, err := json.Marshal(event)
	if err != nil {
		return Event{}, fmt.Errorf("%w: encode event: %v", ErrUnavailable, err)
	}
	data = append(data, '\n')
	if _, err := file.Seek(0, 2); err != nil {
		return Event{}, fmt.Errorf("%w: seek run log: %v", ErrUnavailable, err)
	}
	// One write preserves record atomicity while the advisory lock serializes
	// all cooperating writers.
	n, err := file.Write(data)
	if err != nil || n != len(data) {
		if err == nil {
			err = errors.New("short write")
		}
		return Event{}, fmt.Errorf("%w: append event: %v", ErrUnavailable, err)
	}
	if err := file.Sync(); err != nil {
		return Event{}, fmt.Errorf("%w: sync event: %v", ErrUnavailable, err)
	}
	return event, nil
}

func (s *Store) ReadRun(runID string) (*Run, error) {
	events, err := s.readEvents(runID)
	if err != nil {
		return nil, err
	}
	return Reconstruct(runID, events)
}

func (s *Store) ReadRuns() ([]*Run, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []*Run{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: list runs: %v", ErrUnavailable, err)
	}
	runs := make([]*Run, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		runID := strings.TrimSuffix(name, ".jsonl")
		if !validRunID(runID) {
			continue
		}
		run, err := s.ReadRun(runID)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].StartedAt.Equal(runs[j].StartedAt) {
			return runs[i].ID > runs[j].ID
		}
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	return runs, nil
}

func (s *Store) FindMessage(id string) (*Message, *Run, error) {
	runs, err := s.ReadRuns()
	if err != nil {
		return nil, nil, err
	}
	for _, run := range runs {
		for _, message := range run.Messages {
			if message.ID == id {
				return message, run, nil
			}
		}
	}
	return nil, nil, fs.ErrNotExist
}

func (s *Store) readEvents(runID string) ([]Event, error) {
	if !validRunID(runID) {
		return nil, fs.ErrNotExist
	}
	var events []Event
	ran := false
	err := fsutil.WithFlock(s.runPath(runID)+".lock", func() error {
		ran = true
		file, err := os.OpenFile(s.runPath(runID), os.O_RDWR, 0o600)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return fmt.Errorf("%w: open run log: %v", ErrUnavailable, err)
		}
		defer file.Close()
		events, err = readAndRepair(file, runID)
		return err
	})
	if !ran && errors.Is(err, fsutil.ErrLock) {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if err != nil {
		return nil, err
	}
	return events, nil
}

func readAndRepair(file *os.File, runID string) ([]Event, error) {
	data, err := os.ReadFile(file.Name())
	if err != nil {
		return nil, fmt.Errorf("%w: read run log: %v", ErrUnavailable, err)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		last := bytes.LastIndexByte(data, '\n')
		size := int64(last + 1)
		if err := file.Truncate(size); err != nil {
			return nil, fmt.Errorf("%w: repair crash tail: %v", ErrUnavailable, err)
		}
		if err := file.Sync(); err != nil {
			return nil, fmt.Errorf("%w: sync crash-tail repair: %v", ErrUnavailable, err)
		}
		data = data[:last+1]
	}
	lines := bytes.Split(data, []byte{'\n'})
	events := make([]Event, 0, len(lines))
	for index, line := range lines {
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("%w: malformed record %d: %v", ErrCorrupt, index+1, err)
		}
		expected := uint64(len(events) + 1)
		if event.SchemaVersion != SchemaVersion || event.RunID != runID || event.Sequence != expected ||
			event.ID == "" || event.Type == "" || event.Timestamp.IsZero() {
			return nil, fmt.Errorf("%w: invalid record %d", ErrCorrupt, index+1)
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) runPath(runID string) string { return filepath.Join(s.Dir, runID+".jsonl") }
func (s *Store) now() time.Time              { return s.Now().UTC() }

func validRunID(id string) bool {
	if !strings.HasPrefix(id, "run_") || len(id) <= len("run_") {
		return false
	}
	for _, r := range id[len("run_"):] {
		if (r < 'a' || r > 'z') && (r < '2' || r > '7') {
			return false
		}
	}
	return true
}
