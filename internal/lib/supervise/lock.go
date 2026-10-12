package supervise

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrHeld reports that another process holds the daemon lock.
var ErrHeld = errors.New("the supervision daemon lock is held by another process")

const lockName = "lock"

// Dir is the daemon directory of session in the repository whose primary
// checkout is root. Characters other than letters, digits, '.', '_', and '-'
// become '_'; an empty session is _none.
func Dir(root, session string) string {
	return DirIn(filepath.Join(root, ".fledge", "state"), session)
}

// DirIn is Dir for the repository whose state store directory is stateDir.
func DirIn(stateDir, session string) string {
	name := strings.Map(func(r rune) rune {
		if 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, session)
	if name == "" {
		name = "_none"
	}
	if strings.Trim(name, ".") == "" {
		name = "_" + name
	}
	return filepath.Join(stateDir, "supervise", name)
}

// Status is what a live daemon reports in its lock file. HeartbeatAt is
// rewritten every 10 seconds; it shows a hung daemon, not liveness. Watched
// lists the agent record ids it watches.
type Status struct {
	PID         int      `json:"pid"`
	Version     string   `json:"version"`
	StartedAt   string   `json:"started_at"`
	HeartbeatAt string   `json:"heartbeat_at"`
	Watched     []string `json:"watched"`
}

// Lock is the held daemon lock.
type Lock struct{ f *os.File }

// Acquire takes the daemon lock in dir without waiting, creating dir as
// needed, and fails with ErrHeld when a live daemon holds it.
func Acquire(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("supervise: create %s: %w", dir, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("supervise: open lock: %w", err)
	}
	if err := flock(f, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Write replaces the status in the held lock file in place, so the file the
// lock is on never changes.
func (l *Lock) Write(s Status) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := l.f.Truncate(0); err != nil {
		return fmt.Errorf("supervise: write status: %w", err)
	}
	if _, err := l.f.WriteAt(append(data, '\n'), 0); err != nil {
		return fmt.Errorf("supervise: write status: %w", err)
	}
	return nil
}

// Release gives up the lock.
func (l *Lock) Release() { l.f.Close() }

// Read reports whether a daemon holds the lock in dir and, when one does, its
// last written status. A missing directory is no daemon.
func Read(dir string) (Status, bool, error) {
	f, err := os.Open(filepath.Join(dir, lockName))
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, false, nil
	}
	if err != nil {
		return Status{}, false, fmt.Errorf("supervise: open lock: %w", err)
	}
	defer f.Close()
	err = flock(f, syscall.LOCK_SH|syscall.LOCK_NB)
	if err == nil {
		return Status{}, false, nil
	}
	if !errors.Is(err, ErrHeld) {
		return Status{}, false, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return Status{}, true, fmt.Errorf("supervise: read status: %w", err)
	}
	var s Status
	// A daemon that has not written its status yet is live with none.
	if len(data) > 0 {
		if err := json.Unmarshal(data, &s); err != nil {
			return Status{}, true, fmt.Errorf("supervise: decode status: %w", err)
		}
	}
	return s, true, nil
}

func flock(f *os.File, how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		switch {
		case err == syscall.EINTR:
			continue
		case err == syscall.EWOULDBLOCK:
			return ErrHeld
		case err != nil:
			return fmt.Errorf("supervise: lock: %w", err)
		}
		return nil
	}
}
