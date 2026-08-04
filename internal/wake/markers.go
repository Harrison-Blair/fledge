package wake

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	markersFilename = "markers.json"
	markersVersion  = 1
)

// StatusSeen records how far the watcher has consumed one worker's status file.
type StatusSeen struct {
	Size      int64 `json:"size"`
	MtimeUnix int64 `json:"mtime_unix"`
	Offset    int64 `json:"offset"`
}

// Markers is the watcher's suppression state: what it has already seen and
// already woken for. It is advisory — a lost or unreadable markers file costs
// at most a duplicate wake, so decoding never fails.
type Markers struct {
	Version    int                   `json:"version"`
	StatusSeen map[string]StatusSeen `json:"status_seen"`
	// Terminal names the workers that already reported done or failed. It is
	// suppression state like the rest: held here rather than in the watcher
	// process so a restart does not report a finished worker as vanished, and
	// so the rollback that retries an unqueued observation retracts it too.
	Terminal        map[string]bool  `json:"terminal,omitempty"`
	EventEscalated  map[string]bool  `json:"event_escalated"`
	DoneGrace       map[string]int64 `json:"done_grace"`
	KnownAgents     []string         `json:"known_agents"`
	LastWakeUnix    int64            `json:"last_wake_unix"`
	HeartbeatStreak int              `json:"heartbeat_streak"`
}

// LoadMarkers returns the stored suppression markers. A missing, unreadable, or
// out-of-version markers file yields empty markers rather than an error.
func (l *Ledger) LoadMarkers() (Markers, error) {
	var markers Markers
	err := l.withLock(func() error {
		var err error
		markers, err = l.readMarkers()
		return err
	})
	return markers, err
}

// SaveMarkers replaces the stored suppression markers atomically. The stored
// version is always the current one.
func (l *Ledger) SaveMarkers(markers Markers) error {
	markers.Version = markersVersion
	contents, err := json.MarshalIndent(markers, "", "  ")
	if err != nil {
		return fmt.Errorf("encode wake markers: %w", err)
	}
	contents = append(contents, '\n')
	return l.withLock(func() error { return writeFileAtomically(l.markersPath(), contents) })
}

func (l *Ledger) readMarkers() (Markers, error) {
	path := l.markersPath()
	if err := rejectSymlink(path); err != nil {
		return Markers{}, err
	}
	file, err := openRegular(path, os.O_RDONLY, 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return emptyMarkers(), nil
	}
	if err != nil {
		return Markers{}, fmt.Errorf("open wake markers %q: %w", path, err)
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		return Markers{}, fmt.Errorf("read wake markers %q: %w", path, err)
	}
	return decodeMarkers(contents), nil
}

// decodeMarkers reads stored markers, degrading to empty markers whenever the
// contents cannot be trusted.
func decodeMarkers(contents []byte) Markers {
	var markers Markers
	if err := json.Unmarshal(contents, &markers); err != nil || markers.Version != markersVersion {
		return emptyMarkers()
	}
	return markers
}

func emptyMarkers() Markers { return Markers{Version: markersVersion} }

func (l *Ledger) markersPath() string { return filepath.Join(l.watchPath(), markersFilename) }
