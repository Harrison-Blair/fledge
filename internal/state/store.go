package state

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

const SchemaVersion = 1

type Agent struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Model        string `json:"model,omitempty"`
	Placement    string `json:"placement,omitempty"`
	CWD          string `json:"cwd"`
	TabID        string `json:"tab_id"`
	PaneID       string `json:"pane_id"`
	ActivationID string `json:"activation_id,omitempty"`
}

type Session struct {
	SchemaVersion           int              `json:"schema_version"`
	ProjectRoot             string           `json:"project_root"`
	Session                 string           `json:"session"`
	Socket                  string           `json:"socket,omitempty"`
	WorkspaceID             string           `json:"workspace_id,omitempty"`
	OrchestratorTabID       string           `json:"orchestrator_tab_id,omitempty"`
	OrchestratorPaneID      string           `json:"orchestrator_pane_id,omitempty"`
	OrchestratorInitialized bool             `json:"orchestrator_initialized,omitempty"`
	StopGeneration          uint64           `json:"stop_generation,omitempty"`
	ActiveRunID             string           `json:"active_run_id,omitempty"`
	Agents                  map[string]Agent `json:"agents"`
}

type Store struct {
	Root string
}

func New(root string) (*Store, error) {
	if root == "" {
		root = os.Getenv("XDG_STATE_HOME")
		if root == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("find state home: %w", err)
			}
			root = filepath.Join(home, ".local", "state")
		}
		root = filepath.Join(root, "fledge")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, fmt.Errorf("secure state directory: %w", err)
	}
	return &Store{Root: root}, nil
}

func (s *Store) path(session string) string {
	sum := sha256.Sum256([]byte(session))
	return filepath.Join(s.Root, fmt.Sprintf("%x.json", sum[:12]))
}

// WithLocked serializes all reads and writes for a session. A successful
// callback is persisted using fsync and atomic rename.
func (s *Store) WithLocked(session, projectRoot string, fn func(*Session) error) error {
	path := s.path(session)
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open state lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock state: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck

	st := Session{
		SchemaVersion: SchemaVersion,
		ProjectRoot:   projectRoot,
		Session:       session,
		Agents:        map[string]Agent{},
	}
	data, readErr := os.ReadFile(path)
	if readErr == nil {
		if err := json.Unmarshal(data, &st); err != nil {
			return fmt.Errorf("decode state: %w", err)
		}
		if st.Agents == nil {
			st.Agents = map[string]Agent{}
		}
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return fmt.Errorf("read state: %w", readErr)
	}
	if err := validateSession(st, session, projectRoot); err != nil {
		return err
	}
	if err := fn(&st); err != nil {
		return err
	}
	st.SchemaVersion, st.ProjectRoot, st.Session = SchemaVersion, projectRoot, session
	return writeAtomic(path, st)
}

func (s *Store) Read(session, projectRoot string) (Session, error) {
	var result Session
	err := s.WithLocked(session, projectRoot, func(st *Session) error {
		result = *st
		result.Agents = make(map[string]Agent, len(st.Agents))
		for k, v := range st.Agents {
			result.Agents[k] = v
		}
		return nil
	})
	return result, err
}

// ReadExisting reads a persisted session without creating lock or state files
// and without rewriting or normalizing the stored value. The boolean reports
// whether the session file exists.
func (s *Store) ReadExisting(session, projectRoot string) (Session, bool, error) {
	data, err := os.ReadFile(s.path(session))
	if errors.Is(err, fs.ErrNotExist) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, true, fmt.Errorf("read state: %w", err)
	}
	var st Session
	if err := json.Unmarshal(data, &st); err != nil {
		return Session{}, true, fmt.Errorf("decode state: %w", err)
	}
	if err := validateSession(st, session, projectRoot); err != nil {
		return Session{}, true, err
	}
	return st, true, nil
}

func validateSession(st Session, session, projectRoot string) error {
	if st.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported state schema %d", st.SchemaVersion)
	}
	if st.ProjectRoot != "" && st.ProjectRoot != projectRoot {
		return fmt.Errorf("session %q belongs to project %s", session, st.ProjectRoot)
	}
	return nil
}

func writeAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary state: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	ok = true
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
