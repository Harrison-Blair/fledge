package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/session/record"
	"fledge/internal/session/workspace"
)

func TestEnsureWorkspacesUsesStoredIdentityRegardlessOfLabel(t *testing.T) {
	root, recordPath := managedWorkspaceRecord(t)
	if err := record.WriteWorkspaces(recordPath, map[string]string{"orchestrator": "w-stored"}); err != nil {
		t.Fatal(err)
	}
	stored := herdr.Workspace{ID: "w-stored", Label: "renamed by user", Number: 99, Focused: true}
	server := &fakeWorkspaceServer{live: []herdr.Workspace{
		{ID: "w-label", Label: "f:" + filepath.Base(root), Number: 1},
		stored,
	}}

	for range 2 {
		got, err := EnsureWorkspaces(context.Background(), root, recordPath, server, OrchestratorWorkspaceRole)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got[OrchestratorWorkspaceRole], stored) {
			t.Fatalf("stored result = %#v, want actual workspace %#v", got[OrchestratorWorkspaceRole], stored)
		}
	}
	if server.CreateCalls() != 0 || len(server.CloseCalls()) != 0 {
		t.Fatalf("creates/closes = %d/%q, want 0/none", server.CreateCalls(), server.CloseCalls())
	}
	ids, err := record.ReadWorkspaces(recordPath)
	if err != nil || ids["orchestrator"] != "w-stored" {
		t.Fatalf("persisted IDs = %#v, %v", ids, err)
	}
}

func TestEnsureWorkspacesAdoptsMissingAndStaleState(t *testing.T) {
	tests := []struct {
		name   string
		stored map[string]string
		live   []herdr.Workspace
		wantID string
	}{
		{
			name:   "missing sidecar upgrade",
			live:   []herdr.Workspace{{ID: "w-existing", Label: "f-agents:project"}},
			wantID: "w-existing",
		},
		{
			name:   "stale ID re-adoption",
			stored: map[string]string{"agents": "w-gone"},
			live:   []herdr.Workspace{{ID: "w-replacement", Label: "f-agents:project"}},
			wantID: "w-replacement",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, recordPath := managedWorkspaceRecordNamed(t, "project")
			if test.stored != nil {
				if err := record.WriteWorkspaces(recordPath, test.stored); err != nil {
					t.Fatal(err)
				}
			}
			server := &fakeWorkspaceServer{live: test.live}
			got, err := EnsureWorkspaces(context.Background(), root, recordPath, server, AgentsWorkspaceRole)
			if err != nil {
				t.Fatal(err)
			}
			if got[AgentsWorkspaceRole].ID != test.wantID || server.CreateCalls() != 0 {
				t.Fatalf("result/create count = %#v/%d, want %q/0", got, server.CreateCalls(), test.wantID)
			}
			ids, err := record.ReadWorkspaces(recordPath)
			if err != nil || ids["agents"] != test.wantID {
				t.Fatalf("persisted IDs = %#v, %v; want agents=%q", ids, err, test.wantID)
			}
		})
	}
}

func TestEnsureWorkspacesReportsAmbiguousAndNoncreatableMissing(t *testing.T) {
	t.Run("ambiguous", func(t *testing.T) {
		root, recordPath := managedWorkspaceRecordNamed(t, "project")
		if err := record.WriteWorkspaces(recordPath, map[string]string{"future-role": "future-id"}); err != nil {
			t.Fatal(err)
		}
		server := &fakeWorkspaceServer{live: []herdr.Workspace{
			{ID: "w-z", Label: "f-agents:project"},
			{ID: "w-a", Label: "f-agents:project"},
		}}
		before, err := os.ReadFile(filepath.Join(recordPath, record.WorkspacesFileName))
		if err != nil {
			t.Fatal(err)
		}
		_, err = EnsureWorkspaces(context.Background(), root, recordPath, server, AgentsWorkspaceRole)
		var ambiguous *AmbiguousError
		if !errors.As(err, &ambiguous) {
			t.Fatalf("error = %v, want AmbiguousError", err)
		}
		if ambiguous.Role != AgentsWorkspaceRole || ambiguous.Label != "f-agents:project" || !reflect.DeepEqual(ambiguous.WorkspaceIDs, []string{"w-a", "w-z"}) {
			t.Fatalf("AmbiguousError = %#v", ambiguous)
		}
		if !strings.Contains(err.Error(), "close or rename") {
			t.Fatalf("error = %v, want actionable repair", err)
		}
		after, readErr := os.ReadFile(filepath.Join(recordPath, record.WorkspacesFileName))
		if readErr != nil || string(after) != string(before) {
			t.Fatalf("state changed on ambiguity: before=%q after=%q err=%v", before, after, readErr)
		}
	})

	t.Run("non-creatable missing", func(t *testing.T) {
		root, recordPath := managedWorkspaceRecordNamed(t, "project")
		server := &fakeWorkspaceServer{}
		_, err := EnsureWorkspaces(context.Background(), root, recordPath, server, OrchestratorWorkspaceRole)
		var missing *MissingError
		if !errors.As(err, &missing) || missing.Role != OrchestratorWorkspaceRole || missing.Label != "f:project" {
			t.Fatalf("error = %#v, want orchestrator MissingError", err)
		}
		if server.CreateCalls() != 0 {
			t.Fatalf("CreateWorkspace calls = %d, want 0", server.CreateCalls())
		}
	})
}

func TestEnsureWorkspacesCreatesCreatableRoleAndPreservesRoot(t *testing.T) {
	root, recordPath := managedWorkspaceRecordNamed(t, "my-project")
	created := herdr.WorkspaceCreated{
		Workspace: herdr.Workspace{ID: "w-new", Label: "f-agents:my-project", Number: 7},
		Tab:       herdr.Tab{ID: "t-root", WorkspaceID: "w-new"},
		RootPane:  herdr.Pane{ID: "p-root", WorkspaceID: "w-new", TabID: "t-root"},
	}
	server := &fakeWorkspaceServer{created: []herdr.WorkspaceCreated{created}}
	got, err := EnsureWorkspaces(context.Background(), root, recordPath, server, AgentsWorkspaceRole)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got[AgentsWorkspaceRole], created.Workspace) {
		t.Fatalf("result = %#v, want %#v", got, created.Workspace)
	}
	if labels := server.CreateLabels(); !reflect.DeepEqual(labels, []string{"f-agents:my-project"}) {
		t.Fatalf("CreateWorkspace labels = %q", labels)
	}
	// The server seam has no focus operation, and success performs no close: the
	// root tab and pane returned by CreateWorkspace remain owned by the caller's
	// live workspace.
	if closes := server.CloseCalls(); len(closes) != 0 {
		t.Fatalf("CloseWorkspace calls = %q, want none", closes)
	}
	ids, err := record.ReadWorkspaces(recordPath)
	if err != nil || ids["agents"] != "w-new" {
		t.Fatalf("persisted IDs = %#v, %v", ids, err)
	}
}

func TestEnsureWorkspacesMultiRoleDeterministicAndPreservesUnknownRoles(t *testing.T) {
	root, recordPath := managedWorkspaceRecordNamed(t, "project")
	if err := record.WriteWorkspaces(recordPath, map[string]string{"future-role": "w-future"}); err != nil {
		t.Fatal(err)
	}
	server := &fakeWorkspaceServer{
		live:    []herdr.Workspace{{ID: "w-orchestrator", Label: "f:project"}},
		created: []herdr.WorkspaceCreated{{Workspace: herdr.Workspace{ID: "w-agents", Label: "f-agents:project"}}},
	}
	got, err := EnsureWorkspaces(context.Background(), root, recordPath, server, AgentsWorkspaceRole, OrchestratorWorkspaceRole)
	if err != nil {
		t.Fatal(err)
	}
	if got[OrchestratorWorkspaceRole].ID != "w-orchestrator" || got[AgentsWorkspaceRole].ID != "w-agents" {
		t.Fatalf("result = %#v", got)
	}
	if server.ListCalls() != 1 || server.CreateCalls() != 1 {
		t.Fatalf("list/create calls = %d/%d, want 1/1", server.ListCalls(), server.CreateCalls())
	}
	ids, err := record.ReadWorkspaces(recordPath)
	want := map[string]string{"future-role": "w-future", "orchestrator": "w-orchestrator", "agents": "w-agents"}
	if err != nil || !reflect.DeepEqual(ids, want) {
		t.Fatalf("persisted IDs = %#v, %v; want %#v", ids, err, want)
	}
}

func TestEnsureWorkspacesRejectsCrossRoleWorkspaceIdentity(t *testing.T) {
	tests := []struct {
		name   string
		stored map[string]string
		live   herdr.Workspace
	}{
		{
			name:   "duplicate persisted IDs",
			stored: map[string]string{"orchestrator": "w-shared", "agents": "w-shared"},
			live:   herdr.Workspace{ID: "w-shared", Label: "renamed"},
		},
		{
			name:   "stored identity collides with label adoption",
			stored: map[string]string{"orchestrator": "w-shared"},
			live:   herdr.Workspace{ID: "w-shared", Label: "f-agents:project"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, recordPath := managedWorkspaceRecordNamed(t, "project")
			if err := record.WriteWorkspaces(recordPath, test.stored); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(recordPath, record.WorkspacesFileName))
			if err != nil {
				t.Fatal(err)
			}
			server := &fakeWorkspaceServer{live: []herdr.Workspace{test.live}}
			_, err = EnsureWorkspaces(context.Background(), root, recordPath, server,
				AgentsWorkspaceRole, OrchestratorWorkspaceRole)
			var conflict *RoleConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("error = %v, want RoleConflictError", err)
			}
			if conflict.WorkspaceID != "w-shared" || conflict.FirstRole != OrchestratorWorkspaceRole || conflict.ConflictingRole != AgentsWorkspaceRole {
				t.Fatalf("RoleConflictError = %#v", conflict)
			}
			if !strings.Contains(err.Error(), "distinct workspace") {
				t.Fatalf("error = %v, want actionable distinct-workspace repair", err)
			}
			after, readErr := os.ReadFile(filepath.Join(recordPath, record.WorkspacesFileName))
			if readErr != nil || string(after) != string(before) {
				t.Fatalf("state changed on role conflict: before=%q after=%q err=%v", before, after, readErr)
			}
			if len(server.CloseCalls()) != 0 {
				t.Fatalf("role conflict closed non-created workspace: %q", server.CloseCalls())
			}
		})
	}
}

func TestEnsureWorkspacesValidatesInputsBeforeEffects(t *testing.T) {
	root, recordPath := managedWorkspaceRecord(t)
	server := &fakeWorkspaceServer{}
	tests := []struct {
		name       string
		ctx        context.Context
		root       string
		recordPath string
		server     WorkspaceServer
		roles      []WorkspaceRole
		want       string
	}{
		{name: "nil context", root: root, recordPath: recordPath, server: server, roles: []WorkspaceRole{AgentsWorkspaceRole}, want: "context is nil"},
		{name: "nil server", ctx: context.Background(), root: root, recordPath: recordPath, roles: []WorkspaceRole{AgentsWorkspaceRole}, want: "server is nil"},
		{name: "empty root", ctx: context.Background(), recordPath: recordPath, server: server, roles: []WorkspaceRole{AgentsWorkspaceRole}, want: "project root is empty"},
		{name: "relative root", ctx: context.Background(), root: "relative", recordPath: recordPath, server: server, roles: []WorkspaceRole{AgentsWorkspaceRole}, want: "clean absolute"},
		{name: "empty record", ctx: context.Background(), root: root, server: server, roles: []WorkspaceRole{AgentsWorkspaceRole}, want: "record path is empty"},
		{name: "record outside root", ctx: context.Background(), root: root, recordPath: t.TempDir(), server: server, roles: []WorkspaceRole{AgentsWorkspaceRole}, want: "not a session record"},
		{name: "unknown role", ctx: context.Background(), root: root, recordPath: recordPath, server: server, roles: []WorkspaceRole{"future"}, want: "unregistered"},
		{name: "duplicate role", ctx: context.Background(), root: root, recordPath: recordPath, server: server, roles: []WorkspaceRole{AgentsWorkspaceRole, AgentsWorkspaceRole}, want: "duplicate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := EnsureWorkspaces(test.ctx, test.root, test.recordPath, test.server, test.roles...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
	if server.ListCalls() != 0 || server.CreateCalls() != 0 {
		t.Fatalf("server calls after validation failures = list %d/create %d", server.ListCalls(), server.CreateCalls())
	}

	empty, err := EnsureWorkspaces(context.Background(), root, recordPath, server)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty role request = %#v, %v; want empty map", empty, err)
	}
	if server.ListCalls() != 0 {
		t.Fatal("empty role request listed workspaces")
	}
}

func TestEnsureWorkspacesPropagatesOperationAndLockErrors(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		root, recordPath := managedWorkspaceRecord(t)
		if err := os.WriteFile(filepath.Join(recordPath, record.WorkspacesFileName), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		server := &fakeWorkspaceServer{}
		_, err := EnsureWorkspaces(context.Background(), root, recordPath, server, AgentsWorkspaceRole)
		if err == nil || !strings.Contains(err.Error(), "read managed workspaces") || server.ListCalls() != 0 {
			t.Fatalf("error/list calls = %v/%d", err, server.ListCalls())
		}
	})

	t.Run("list", func(t *testing.T) {
		root, recordPath := managedWorkspaceRecord(t)
		want := errors.New("list failed")
		_, err := EnsureWorkspaces(context.Background(), root, recordPath, &fakeWorkspaceServer{listErr: want}, AgentsWorkspaceRole)
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want list failure", err)
		}
	})

	t.Run("create", func(t *testing.T) {
		root, recordPath := managedWorkspaceRecord(t)
		want := errors.New("create failed")
		server := &fakeWorkspaceServer{createErr: want}
		_, err := EnsureWorkspaces(context.Background(), root, recordPath, server, AgentsWorkspaceRole)
		if !errors.Is(err, want) || len(server.CloseCalls()) != 0 {
			t.Fatalf("error/closes = %v/%q", err, server.CloseCalls())
		}
	})

	t.Run("lock", func(t *testing.T) {
		root, recordPath := managedWorkspaceRecord(t)
		want := errors.New("lock failed")
		server := &fakeWorkspaceServer{}
		_, err := ensureWorkspaces(context.Background(), root, recordPath, server,
			func(context.Context, string) (func() error, error) { return nil, want }, AgentsWorkspaceRole)
		if !errors.Is(err, want) || server.ListCalls() != 0 {
			t.Fatalf("error/list calls = %v/%d", err, server.ListCalls())
		}
	})

	t.Run("release", func(t *testing.T) {
		root, recordPath := managedWorkspaceRecord(t)
		if err := record.WriteWorkspaces(recordPath, map[string]string{"agents": "w-live"}); err != nil {
			t.Fatal(err)
		}
		want := errors.New("release failed")
		server := &fakeWorkspaceServer{live: []herdr.Workspace{{ID: "w-live", Label: "anything"}}}
		got, err := ensureWorkspaces(context.Background(), root, recordPath, server,
			func(context.Context, string) (func() error, error) {
				return func() error { return want }, nil
			}, AgentsWorkspaceRole)
		if !errors.Is(err, want) || got[AgentsWorkspaceRole].ID != "w-live" {
			t.Fatalf("result/error = %#v/%v, want live result and release error", got, err)
		}
		if len(server.CloseCalls()) != 0 {
			t.Fatalf("release failure closed stored workspace: %q", server.CloseCalls())
		}
	})

	t.Run("release after published create preserves registered workspace", func(t *testing.T) {
		root, recordPath := managedWorkspaceRecord(t)
		want := errors.New("release failed")
		server := &fakeWorkspaceServer{}
		got, err := ensureWorkspaces(context.Background(), root, recordPath, server,
			func(context.Context, string) (func() error, error) {
				return func() error { return want }, nil
			}, AgentsWorkspaceRole)
		if !errors.Is(err, want) || got[AgentsWorkspaceRole].ID != "w-created-1" {
			t.Fatalf("result/error = %#v/%v, want published workspace and release failure", got, err)
		}
		if closes := server.CloseCalls(); len(closes) != 0 {
			t.Fatalf("release failure closed published workspace: %q", closes)
		}
		ids, readErr := record.ReadWorkspaces(recordPath)
		if readErr != nil || ids["agents"] != "w-created-1" {
			t.Fatalf("persisted IDs = %#v, %v; want live published identity", ids, readErr)
		}
	})
}

func TestEnsureWorkspacesWriteFailureCleansOnlyCreatedAndJoinsErrors(t *testing.T) {
	root, recordPath := managedWorkspaceRecordNamed(t, "project")
	ctx, cancel := context.WithCancel(context.Background())
	closeErr := errors.New("close failed")
	releaseErr := errors.New("release failed")
	server := &fakeWorkspaceServer{
		live:     []herdr.Workspace{{ID: "w-adopted", Label: "f:project"}},
		created:  []herdr.WorkspaceCreated{{Workspace: herdr.Workspace{ID: "w-created", Label: "f-agents:project"}}},
		closeErr: closeErr,
	}
	server.onCreate = func() {
		cancel()
		if err := os.Mkdir(filepath.Join(recordPath, record.WorkspacesFileName), 0o755); err != nil {
			t.Errorf("force write failure: %v", err)
		}
	}
	var cleanupContextErr error
	server.onClose = func(ctx context.Context, _ string) { cleanupContextErr = ctx.Err() }

	_, err := ensureWorkspaces(ctx, root, recordPath, server,
		func(context.Context, string) (func() error, error) {
			return func() error { return releaseErr }, nil
		}, OrchestratorWorkspaceRole, AgentsWorkspaceRole)
	if err == nil || !strings.Contains(err.Error(), "publish") || !errors.Is(err, closeErr) || !errors.Is(err, releaseErr) {
		t.Fatalf("joined error = %v, want write, close, and release failures", err)
	}
	if got := server.CloseCalls(); !reflect.DeepEqual(got, []string{"w-created"}) {
		t.Fatalf("CloseWorkspace calls = %q, want only created workspace", got)
	}
	if cleanupContextErr != nil {
		t.Fatalf("cleanup context error = %v, want cancellation detached", cleanupContextErr)
	}
}

func TestEnsureWorkspacesConcurrentCallersCreateExactlyOnce(t *testing.T) {
	root, recordPath := managedWorkspaceRecordNamed(t, "project")
	firstListed := make(chan struct{})
	secondListed := make(chan struct{})
	allowFirst := make(chan struct{})
	var allowOnce sync.Once
	releaseFirst := func() { allowOnce.Do(func() { close(allowFirst) }) }
	defer releaseFirst()
	server := &fakeWorkspaceServer{}
	server.onList = func(call int) {
		switch call {
		case 1:
			close(firstListed)
			<-allowFirst
		case 2:
			close(secondListed)
		}
	}

	const callers = 12
	results := make(chan string, callers)
	errs := make(chan error, callers)
	var workers sync.WaitGroup
	run := func() {
		defer workers.Done()
		got, err := EnsureWorkspaces(context.Background(), root, recordPath, server, AgentsWorkspaceRole)
		if err != nil {
			errs <- err
			return
		}
		results <- got[AgentsWorkspaceRole].ID
	}

	workers.Add(1)
	go run()
	select {
	case <-firstListed:
	case <-time.After(2 * time.Second):
		t.Fatal("first caller did not list workspaces")
	}
	for range callers - 1 {
		workers.Add(1)
		go run()
	}
	// The first caller is paused after taking its empty server snapshot. Without
	// the real project lock, another caller reaches the same point in this
	// bounded window and the test detects the serialization loss directly; it
	// would then also create a second workspace from the stale snapshot.
	serialized := true
	select {
	case <-secondListed:
		serialized = false
	case <-time.After(100 * time.Millisecond):
	}
	releaseFirst()
	workers.Wait()
	close(results)
	close(errs)
	if !serialized {
		t.Error("a second caller listed workspaces while the first held the project lock")
	}
	for err := range errs {
		t.Errorf("EnsureWorkspaces() error = %v", err)
	}
	for id := range results {
		if id != "w-created-1" {
			t.Errorf("workspace ID = %q, want w-created-1", id)
		}
	}
	if server.CreateCalls() != 1 {
		t.Fatalf("CreateWorkspace calls = %d, want exactly 1", server.CreateCalls())
	}
	ids, err := record.ReadWorkspaces(recordPath)
	if err != nil || len(ids) != 1 || ids["agents"] != "w-created-1" {
		t.Fatalf("persisted IDs = %#v, %v; want one agents identity", ids, err)
	}
}

func TestRequestedWorkspaceSpecsUsesRegistryOrder(t *testing.T) {
	got, err := requestedWorkspaceSpecs([]WorkspaceRole{AgentsWorkspaceRole, OrchestratorWorkspaceRole})
	if err != nil {
		t.Fatal(err)
	}
	roles := make([]WorkspaceRole, len(got))
	for i, spec := range got {
		roles[i] = spec.Role()
	}
	if want := []WorkspaceRole{workspace.Orchestrator, workspace.Agents}; !reflect.DeepEqual(roles, want) {
		t.Fatalf("ordered roles = %#v, want %#v", roles, want)
	}
}

type fakeWorkspaceServer struct {
	mu sync.Mutex

	live      []herdr.Workspace
	created   []herdr.WorkspaceCreated
	listErr   error
	createErr error
	closeErr  error
	onCreate  func()
	onClose   func(context.Context, string)
	onList    func(int)
	listCalls int
	labels    []string
	closes    []string
}

func (f *fakeWorkspaceServer) Workspaces(context.Context) ([]herdr.Workspace, error) {
	f.mu.Lock()
	f.listCalls++
	call := f.listCalls
	live := append([]herdr.Workspace(nil), f.live...)
	err := f.listErr
	hook := f.onList
	f.mu.Unlock()
	if hook != nil {
		hook(call)
	}
	return live, err
}

func (f *fakeWorkspaceServer) CreateWorkspace(_ context.Context, label string) (herdr.WorkspaceCreated, error) {
	f.mu.Lock()
	f.labels = append(f.labels, label)
	if f.createErr != nil {
		err := f.createErr
		f.mu.Unlock()
		return herdr.WorkspaceCreated{}, err
	}
	var created herdr.WorkspaceCreated
	index := len(f.labels) - 1
	if index < len(f.created) {
		created = f.created[index]
	} else {
		created.Workspace = herdr.Workspace{ID: fmt.Sprintf("w-created-%d", len(f.labels)), Label: label}
	}
	f.live = append(f.live, created.Workspace)
	hook := f.onCreate
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return created, nil
}

func (f *fakeWorkspaceServer) CloseWorkspace(ctx context.Context, id string) error {
	f.mu.Lock()
	f.closes = append(f.closes, id)
	hook := f.onClose
	err := f.closeErr
	f.mu.Unlock()
	if hook != nil {
		hook(ctx, id)
	}
	return err
}

func (f *fakeWorkspaceServer) ListCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

func (f *fakeWorkspaceServer) CreateCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.labels)
}

func (f *fakeWorkspaceServer) CreateLabels() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.labels...)
}

func (f *fakeWorkspaceServer) CloseCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.closes...)
}

func managedWorkspaceRecord(t *testing.T) (string, string) {
	t.Helper()
	return managedWorkspaceRecordNamed(t, "project")
}

func managedWorkspaceRecordNamed(t *testing.T, projectName string) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), projectName)
	recordPath := filepath.Join(root, ".fledge", "sessions", "session")
	if err := os.MkdirAll(recordPath, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, recordPath
}
