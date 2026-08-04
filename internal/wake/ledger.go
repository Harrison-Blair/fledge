// Package wake stores the watcher's durable wake ledger for the active Fledge
// session. Wakes are appended to an at-least-once log before any suppression
// marker advances, so an interrupted watcher replays what it never delivered.
package wake

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/statedir"
)

const (
	logFilename  = "ledger.jsonl"
	lockFilename = "ledger.lock"
)

// ErrCorruptLog reports a complete but unreadable wake ledger line.
var ErrCorruptLog = errors.New("corrupt wake ledger")

// Option customizes a Ledger, primarily to make IDs and timestamps testable.
type Option func(*Ledger)

// WithClock replaces the wall clock used for new entries.
func WithClock(clock func() time.Time) Option {
	return func(l *Ledger) { l.clock = clock }
}

// WithIDGenerator replaces the cryptographically random wake ID generator.
func WithIDGenerator(generator func() (string, error)) Option {
	return func(l *Ledger) { l.generateID = generator }
}

// Ledger owns one Herdr session's wake log and suppression markers beneath
// root/.fledge/tmp/<session>/watch.
type Ledger struct {
	root       string
	session    string
	clock      func() time.Time
	generateID func() (string, error)
}

// New constructs a Ledger for the named Herdr session. It does not create or
// modify any files; an unusable session name is reported by the first
// operation that touches the filesystem.
func New(root, session string, options ...Option) *Ledger {
	l := &Ledger{root: root, session: session, clock: time.Now, generateID: randomID}
	for _, option := range options {
		option(l)
	}
	return l
}

// Ensure creates the watcher's state directory. It leaves an existing ledger
// untouched.
func (l *Ledger) Ensure() error { return l.ensureStateDirectory() }

// Append durably queues one wake and returns it. Repeated wakes for the same
// kind and key are collapsed by Pending, not by Append.
func (l *Ledger) Append(kind Kind, key, reason string) (Record, error) {
	if !ValidKind(kind) {
		return Record{}, fmt.Errorf("unknown wake kind %q", kind)
	}
	var created Record
	err := l.withLock(func() error {
		entries, err := l.load()
		if err != nil {
			return err
		}
		id, err := l.uniqueID(entries)
		if err != nil {
			return err
		}
		e := entry{Kind: entryQueued, ID: id, WakeKind: kind, Key: key, Reason: reason, Time: l.now()}
		if err := l.appendEntries([]entry{e}); err != nil {
			return err
		}
		created = Record{ID: e.ID, IDs: []string{e.ID}, WakeKind: e.WakeKind, Key: e.Key, Reason: e.Reason, Time: e.Time}
		return nil
	})
	return created, err
}

// Pending returns the wakes still owed to the orchestrator, deduplicated by
// wake kind and key in first-seen order.
func (l *Ledger) Pending() ([]Record, error) {
	var records []Record
	err := l.withLock(func() error {
		entries, err := l.load()
		if err != nil {
			return err
		}
		records = foldPending(entries)
		return nil
	})
	return records, err
}

// MarkDelivered retires the named wakes, recording the orchestrator message
// that carried them.
func (l *Ledger) MarkDelivered(ids []string, messageID string) error {
	if strings.TrimSpace(messageID) == "" {
		return errors.New("wake delivery is missing its message ID")
	}
	if len(ids) == 0 {
		return nil
	}
	return l.withLock(func() error {
		// Reading first repairs a torn tail so the appended lines stay parsable.
		if _, err := l.load(); err != nil {
			return err
		}
		entries := make([]entry, 0, len(ids))
		for _, id := range ids {
			entries = append(entries, entry{Kind: entryDelivered, ID: id, MessageID: messageID, Time: l.now()})
		}
		return l.appendEntries(entries)
	})
}

// Compact rewrites the ledger with only the wakes still owed to the
// orchestrator, discarding retired wakes together with the delivery markers
// that retired them. Pending wakes keep their IDs, order, and content, so
// Pending reads the same before and after. The rewrite is atomic: an
// interrupted Compact leaves the previous ledger intact.
func (l *Ledger) Compact() error {
	return l.withLock(func() error {
		entries, err := l.load()
		if err != nil {
			return err
		}
		kept := retainPending(entries)
		if len(kept) == len(entries) {
			return nil
		}
		contents, err := encodeEntries(kept)
		if err != nil {
			return err
		}
		return writeFileAtomically(l.logPath(), contents)
	})
}

func (l *Ledger) load() ([]entry, error) {
	data, err := l.readAndRepairLog()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	lines := bytes.Split(data, []byte{'\n'})
	entries := make([]entry, 0, len(lines)-1)
	for index, line := range lines[:len(lines)-1] {
		var e entry
		if err := decodeEntry(line, &e); err != nil {
			return nil, fmt.Errorf("%w at line %d: %v", ErrCorruptLog, index+1, err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// readAndRepairLog returns the ledger's complete lines, truncating a trailing
// line that a crash left unterminated.
func (l *Ledger) readAndRepairLog() ([]byte, error) {
	path := l.logPath()
	if err := rejectSymlink(path); err != nil {
		return nil, err
	}
	file, err := openRegular(path, os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open wake ledger %q: %w", path, err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("secure wake ledger %q: %w", path, err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read wake ledger %q: %w", path, err)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		completeLength := int64(bytes.LastIndexByte(data, '\n') + 1)
		if err := file.Truncate(completeLength); err != nil {
			return nil, fmt.Errorf("repair wake ledger %q: %w", path, err)
		}
		if err := file.Sync(); err != nil {
			return nil, fmt.Errorf("sync repaired wake ledger %q: %w", path, err)
		}
		data = data[:completeLength]
	}
	return data, nil
}

func (l *Ledger) appendEntries(entries []entry) error {
	for _, e := range entries {
		if err := validateEntry(e); err != nil {
			return fmt.Errorf("validate wake entry: %w", err)
		}
	}
	path := l.logPath()
	if err := rejectSymlink(path); err != nil {
		return err
	}
	_, statErr := os.Lstat(path)
	creating := errors.Is(statErr, os.ErrNotExist)
	file, err := openRegular(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open wake ledger %q: %w", path, err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("secure wake ledger %q: %w", path, err)
	}
	data, err := encodeEntries(entries)
	if err != nil {
		return err
	}
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return fmt.Errorf("append wake entries: %w", err)
		}
		data = data[written:]
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync wake entries: %w", err)
	}
	if creating {
		// The first append also creates the ledger, whose directory entry only
		// becomes durable once the directory itself is synced.
		return syncDirectory(l.watchPath())
	}
	return nil
}

func encodeEntries(entries []entry) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	for _, e := range entries {
		if err := encoder.Encode(e); err != nil {
			return nil, fmt.Errorf("encode wake entry: %w", err)
		}
	}
	return buffer.Bytes(), nil
}

// writeFileAtomically replaces path with contents by way of a temporary file
// in the same directory, so an interrupted write leaves the previous contents
// in place rather than a half-written file.
func writeFileAtomically(path string, contents []byte) error {
	if err := rejectSymlink(path); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temporary wake file in %q: %w", directory, err)
	}
	temporary := file.Name()
	if err := writeAndSync(file, contents); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("close temporary wake file %q: %w", temporary, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace wake file %q: %w", path, err)
	}
	return syncDirectory(directory)
}

func writeAndSync(file *os.File, contents []byte) error {
	for len(contents) > 0 {
		written, err := file.Write(contents)
		if err != nil {
			return fmt.Errorf("write wake file %q: %w", file.Name(), err)
		}
		contents = contents[written:]
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync wake file %q: %w", file.Name(), err)
	}
	return nil
}

func (l *Ledger) withLock(operation func() error) error {
	if err := l.ensureStateDirectory(); err != nil {
		return err
	}
	unlock, err := l.acquireLock(l.lockPath())
	if err != nil {
		return err
	}
	operationErr := operation()
	return errors.Join(operationErr, unlock())
}

func (l *Ledger) ensureStateDirectory() error {
	if !statedir.ValidSessionDirName(l.session) {
		return fmt.Errorf("Herdr session name %q is not a valid wake ledger directory name", l.session)
	}
	for _, path := range []string{
		statedir.Root(l.root), statedir.Temp(l.root),
		statedir.TempSession(l.root, l.session), l.watchPath(),
	} {
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.MkdirAll(path, 0o700); err != nil {
				return fmt.Errorf("create wake state directory %q: %w", path, err)
			}
		case err != nil:
			return fmt.Errorf("inspect wake state directory %q: %w", path, err)
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("wake state directory %q must not be a symlink", path)
		case !info.IsDir():
			return fmt.Errorf("wake state path %q is not a directory", path)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure wake state directory %q: %w", path, err)
		}
	}
	return nil
}

func openRegular(path string, flags int, permission os.FileMode) (*os.File, error) {
	file, err := openFileNoFollow(path, flags, permission)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("path %q is not a regular file", path)
	}
	current, err := os.Lstat(path)
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, current) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("path %q changed while opening or is a symlink", path)
	}
	return file, nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect wake path %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("wake path %q must not be a symlink", path)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open wake state directory %q: %w", path, err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync wake state directory %q: %w", path, err)
	}
	return nil
}

func (l *Ledger) uniqueID(entries []entry) (string, error) {
	used := make(map[string]bool, len(entries))
	for _, e := range entries {
		used[e.ID] = true
	}
	for attempts := 0; attempts < 100; attempts++ {
		id, err := l.generateID()
		if err != nil {
			return "", fmt.Errorf("generate wake ID: %w", err)
		}
		if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "\r\n") {
			return "", errors.New("wake ID generator returned an invalid ID")
		}
		if !used[id] {
			return id, nil
		}
	}
	return "", errors.New("wake ID generator repeatedly returned duplicate IDs")
}

func (l *Ledger) now() time.Time { return l.clock().UTC() }

func (l *Ledger) watchPath() string { return statedir.WatchSession(l.root, l.session) }
func (l *Ledger) logPath() string   { return filepath.Join(l.watchPath(), logFilename) }
func (l *Ledger) lockPath() string  { return filepath.Join(l.watchPath(), lockFilename) }

func randomID() (string, error) {
	var value [4]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "w-" + hex.EncodeToString(value[:]), nil
}
