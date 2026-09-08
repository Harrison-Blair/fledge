package sessiontest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/session/bootstrap"
)

// StatusResult is one scripted answer from FakeBootstrapper.Status.
type StatusResult struct {
	Running bool
	Err     error
}

// RenameCall records one workspace or tab rename.
type RenameCall struct {
	ID    string
	Label string
}

// FakeBootstrapper scripts one Herder server. Sequenced results repeat their
// last entry once exhausted.
type FakeBootstrapper struct {
	mu sync.Mutex

	Statuses   []StatusResult
	workspaces []herdr.Workspace
	panes      []herdr.Pane
	StartErrs  []error
	CreateErr  error
	CloseErr   error

	OnStatus          func(int)
	OnStart           func(context.Context, int)
	OnRenameWorkspace func(context.Context) error

	StatusCalls      int
	WorkspaceCalls   int
	paneCalls        int
	RenamedWorkspace []RenameCall
	RenamedTab       []RenameCall
	Created          []herdr.WorkspaceCreated
	Closed           []string
	Started          []herdr.StartAgentOptions
}

func (f *FakeBootstrapper) Status(context.Context) (herdr.Status, error) {
	f.mu.Lock()
	f.StatusCalls++
	call := f.StatusCalls
	result := f.Statuses[min(call, len(f.Statuses))-1]
	notify := f.OnStatus
	f.mu.Unlock()

	if notify != nil {
		notify(call)
	}
	return herdr.Status{Running: result.Running}, result.Err
}

func (f *FakeBootstrapper) Workspaces(context.Context) ([]herdr.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.WorkspaceCalls++
	return append([]herdr.Workspace(nil), f.workspaces...), nil
}

func (f *FakeBootstrapper) Panes(context.Context, string) ([]herdr.Pane, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paneCalls++
	return append([]herdr.Pane(nil), f.panes...), nil
}

func (f *FakeBootstrapper) RenameWorkspace(ctx context.Context, id, label string) error {
	f.mu.Lock()
	f.RenamedWorkspace = append(f.RenamedWorkspace, RenameCall{ID: id, Label: label})
	for i := range f.workspaces {
		if f.workspaces[i].ID == id {
			f.workspaces[i].Label = label
		}
	}
	blocked := f.OnRenameWorkspace
	f.mu.Unlock()

	if blocked != nil {
		return blocked(ctx)
	}
	return nil
}

func (f *FakeBootstrapper) RenameTab(_ context.Context, id, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.RenamedTab = append(f.RenamedTab, RenameCall{ID: id, Label: label})
	return nil
}

func (f *FakeBootstrapper) CreateWorkspace(_ context.Context, label string) (herdr.WorkspaceCreated, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CreateErr != nil {
		return herdr.WorkspaceCreated{}, f.CreateErr
	}
	id := fmt.Sprintf("w%d", len(f.workspaces)+1)
	created := herdr.WorkspaceCreated{
		Workspace: herdr.Workspace{ID: id, Label: label},
		Tab:       herdr.Tab{ID: id + ":t1", WorkspaceID: id},
		RootPane:  herdr.Pane{ID: id + ":p1", WorkspaceID: id, TabID: id + ":t1"},
	}
	f.Created = append(f.Created, created)
	f.workspaces = append(f.workspaces, created.Workspace)
	return created, nil
}

func (f *FakeBootstrapper) CloseWorkspace(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Closed = append(f.Closed, id)
	if f.CloseErr != nil {
		return f.CloseErr
	}
	for i := range f.workspaces {
		if f.workspaces[i].ID == id {
			f.workspaces = append(f.workspaces[:i], f.workspaces[i+1:]...)
			break
		}
	}
	return nil
}

func (f *FakeBootstrapper) StartAgent(ctx context.Context, options herdr.StartAgentOptions) (herdr.Agent, error) {
	f.mu.Lock()
	f.Started = append(f.Started, options)
	call := len(f.Started)
	var err error
	if len(f.StartErrs) != 0 {
		err = f.StartErrs[min(call, len(f.StartErrs))-1]
	}
	notify := f.OnStart
	f.mu.Unlock()

	if notify != nil {
		notify(ctx, call)
	}
	return herdr.Agent{}, err
}

// ReadyBootstrapper is a Herder server that reaches running state after two
// polls and publishes one workspace with two panes.
func ReadyBootstrapper() *FakeBootstrapper {
	return &FakeBootstrapper{
		Statuses: []StatusResult{
			{Err: errors.New("socket missing")},
			{Running: false},
			{Running: true},
		},
		workspaces: []herdr.Workspace{
			{ID: "w1", ActiveTabID: "w1:t2"},
		},
		panes: []herdr.Pane{
			{ID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t1"},
			{ID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t2"},
		},
	}
}

// StartedBootstrapper is a Herder server that is already running.
func StartedBootstrapper() *FakeBootstrapper {
	server := ReadyBootstrapper()
	server.Statuses = []StatusResult{{Running: true}}
	return server
}

// FastTiming polls quickly enough for tests to finish promptly.
func FastTiming() bootstrap.Timing {
	return bootstrap.Timing{
		Poll:     time.Millisecond,
		Deadline: 2 * time.Second,
	}
}

// ErrorReader fails every read with Err.
type ErrorReader struct {
	Err error
}

func (r ErrorReader) Read([]byte) (int, error) {
	return 0, r.Err
}

// CountingReader fails every read and counts the attempts.
type CountingReader struct {
	Reads int
}

func (r *CountingReader) Read([]byte) (int, error) {
	r.Reads++
	return 0, errors.New("entropy should not be read")
}

// NewProject creates an empty project checkout with a .fledge directory.
func NewProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".fledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// WriteRecord publishes a raw session record config under root.
func WriteRecord(t *testing.T, root, name, config string) string {
	t.Helper()
	recordDir := filepath.Join(root, ".fledge", "sessions", name)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recordDir, "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return recordDir
}
