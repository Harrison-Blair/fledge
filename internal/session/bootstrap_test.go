package session

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"fledge/internal/herdr"
)

type statusResult struct {
	running bool
	err     error
}

type renameCall struct {
	id    string
	label string
}

// fakeBootstrapper scripts one Herder server. Sequenced results repeat their
// last entry once exhausted.
type fakeBootstrapper struct {
	mu sync.Mutex

	statuses   []statusResult
	workspaces []herdr.Workspace
	panes      []herdr.Pane
	startErrs  []error

	onStatus func(int)
	onStart  func(int)

	statusCalls      int
	workspaceCalls   int
	paneCalls        int
	renamedWorkspace []renameCall
	renamedTab       []renameCall
	started          []herdr.StartAgentOptions
}

func (f *fakeBootstrapper) Status(context.Context) (herdr.Status, error) {
	f.mu.Lock()
	f.statusCalls++
	call := f.statusCalls
	result := f.statuses[min(call, len(f.statuses))-1]
	notify := f.onStatus
	f.mu.Unlock()

	if notify != nil {
		notify(call)
	}
	return herdr.Status{Running: result.running}, result.err
}

func (f *fakeBootstrapper) Workspaces(context.Context) ([]herdr.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workspaceCalls++
	return f.workspaces, nil
}

func (f *fakeBootstrapper) Panes(context.Context, string) ([]herdr.Pane, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paneCalls++
	return f.panes, nil
}

func (f *fakeBootstrapper) RenameWorkspace(_ context.Context, id, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renamedWorkspace = append(f.renamedWorkspace, renameCall{id: id, label: label})
	return nil
}

func (f *fakeBootstrapper) RenameTab(_ context.Context, id, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renamedTab = append(f.renamedTab, renameCall{id: id, label: label})
	return nil
}

func (f *fakeBootstrapper) StartAgent(_ context.Context, options herdr.StartAgentOptions) (herdr.Agent, error) {
	f.mu.Lock()
	f.started = append(f.started, options)
	call := len(f.started)
	var err error
	if len(f.startErrs) != 0 {
		err = f.startErrs[min(call, len(f.startErrs))-1]
	}
	notify := f.onStart
	f.mu.Unlock()

	if notify != nil {
		notify(call)
	}
	return herdr.Agent{}, err
}

func readyBootstrapper() *fakeBootstrapper {
	return &fakeBootstrapper{
		statuses: []statusResult{
			{err: errors.New("socket missing")},
			{running: false},
			{running: true},
		},
		workspaces: []herdr.Workspace{
			{ID: "w1", ActiveTabID: "w1:t2"},
			{ID: "w2", ActiveTabID: "w2:t1"},
		},
		panes: []herdr.Pane{
			{ID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t1"},
			{ID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t2"},
		},
	}
}

func fastTiming() bootstrapTiming {
	return bootstrapTiming{
		Poll:         time.Millisecond,
		Deadline:     2 * time.Second,
		StartRetries: 3,
		RetryDelay:   time.Millisecond,
	}
}

func bootstrapArgs(choice AgentChoice, log *bytes.Buffer) bootstrapInput {
	return bootstrapInput{Root: "/projects/my-project", Choice: choice, Log: log}
}

func TestBootstrapPreparesSessionAndStartsAgentWithModel(t *testing.T) {
	server := readyBootstrapper()
	var log bytes.Buffer

	err := bootstrap(context.Background(), server, bootstrapArgs(AgentChoice{Harness: "claude", Model: "opus"}, &log), fastTiming())
	if err != nil {
		t.Fatalf("bootstrap() error = %v", err)
	}
	if server.statusCalls != 3 {
		t.Fatalf("Status calls = %d, want polling until running", server.statusCalls)
	}
	if want := []renameCall{{id: "w1", label: "fledge-my-project"}}; !reflect.DeepEqual(server.renamedWorkspace, want) {
		t.Fatalf("workspace renames = %#v, want %#v", server.renamedWorkspace, want)
	}
	if want := []renameCall{{id: "w1:t2", label: "orchestrator"}}; !reflect.DeepEqual(server.renamedTab, want) {
		t.Fatalf("tab renames = %#v, want %#v", server.renamedTab, want)
	}
	want := []herdr.StartAgentOptions{{
		Name:   "orchestrator",
		Kind:   "claude",
		PaneID: "w1:p2",
		Args:   []string{"--model", "opus"},
	}}
	if !reflect.DeepEqual(server.started, want) {
		t.Fatalf("StartAgent options = %#v, want %#v", server.started, want)
	}

	report := log.String()
	for _, step := range []string{"Herder server running", "w1:p2", "fledge-my-project", "orchestrator", "started claude"} {
		if !strings.Contains(report, step) {
			t.Fatalf("log = %q, want a line about %q", report, step)
		}
	}
}

func TestBootstrapOmitsModelArgumentWhenUnset(t *testing.T) {
	server := readyBootstrapper()

	err := bootstrap(context.Background(), server, bootstrapArgs(AgentChoice{Harness: "pi"}, &bytes.Buffer{}), fastTiming())
	if err != nil {
		t.Fatalf("bootstrap() error = %v", err)
	}
	if len(server.started) != 1 {
		t.Fatalf("StartAgent calls = %d, want 1", len(server.started))
	}
	if args := server.started[0].Args; len(args) != 0 {
		t.Fatalf("StartAgent args = %#v, want none", args)
	}
}

func TestBootstrapShellOnlyStartsNoAgent(t *testing.T) {
	server := readyBootstrapper()
	var log bytes.Buffer

	err := bootstrap(context.Background(), server, bootstrapArgs(AgentChoice{}, &log), fastTiming())
	if err != nil {
		t.Fatalf("bootstrap() error = %v", err)
	}
	if len(server.started) != 0 {
		t.Fatalf("StartAgent calls = %#v, want none", server.started)
	}
	if len(server.renamedTab) != 1 {
		t.Fatalf("tab renames = %#v, want the orchestrator rename", server.renamedTab)
	}
}

func TestBootstrapReportsServerThatNeverStarts(t *testing.T) {
	server := &fakeBootstrapper{statuses: []statusResult{{running: false}}}
	var log bytes.Buffer
	timing := fastTiming()
	timing.Deadline = 20 * time.Millisecond

	err := bootstrap(context.Background(), server, bootstrapArgs(AgentChoice{Harness: "pi"}, &log), timing)
	if err == nil || !strings.Contains(err.Error(), "did not start") {
		t.Fatalf("bootstrap() error = %v, want a server-start failure", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bootstrap() error = %v, want a deadline", err)
	}
	if server.workspaceCalls != 0 {
		t.Fatalf("Workspaces calls = %d, want none before the server runs", server.workspaceCalls)
	}
	if !strings.Contains(log.String(), "failed") {
		t.Fatalf("log = %q, want the failure recorded", log.String())
	}
}

func TestBootstrapStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &fakeBootstrapper{statuses: []statusResult{{running: false}}}
	server.onStatus = func(call int) {
		if call == 2 {
			cancel()
		}
	}

	err := bootstrap(ctx, server, bootstrapArgs(AgentChoice{Harness: "pi"}, &bytes.Buffer{}), fastTiming())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("bootstrap() error = %v, want context.Canceled", err)
	}
}

func TestBootstrapRetriesHerderRejections(t *testing.T) {
	rejected := &herdr.Error{Operation: "agent start", Code: "pane_busy", Message: "no shell prompt"}
	server := readyBootstrapper()
	server.startErrs = []error{rejected, rejected, nil}

	err := bootstrap(context.Background(), server, bootstrapArgs(AgentChoice{Harness: "codex"}, &bytes.Buffer{}), fastTiming())
	if err != nil {
		t.Fatalf("bootstrap() error = %v", err)
	}
	if len(server.started) != 3 {
		t.Fatalf("StartAgent calls = %d, want two retries", len(server.started))
	}
}

func TestBootstrapStopsRetryingAtTheLimit(t *testing.T) {
	rejected := &herdr.Error{Operation: "agent start", Code: "pane_busy", Message: "no shell prompt"}
	server := readyBootstrapper()
	server.startErrs = []error{rejected}
	var log bytes.Buffer

	err := bootstrap(context.Background(), server, bootstrapArgs(AgentChoice{Harness: "codex"}, &log), fastTiming())
	if !errors.Is(err, rejected) {
		t.Fatalf("bootstrap() error = %v, want %v", err, rejected)
	}
	if len(server.started) != 3 {
		t.Fatalf("StartAgent calls = %d, want the retry limit", len(server.started))
	}
	if !strings.Contains(log.String(), "failed") {
		t.Fatalf("log = %q, want the failure recorded", log.String())
	}
}

func TestBootstrapDoesNotRetryOtherFailures(t *testing.T) {
	refused := errors.New("connection refused")
	server := readyBootstrapper()
	server.startErrs = []error{refused}

	err := bootstrap(context.Background(), server, bootstrapArgs(AgentChoice{Harness: "codex"}, &bytes.Buffer{}), fastTiming())
	if !errors.Is(err, refused) {
		t.Fatalf("bootstrap() error = %v, want %v", err, refused)
	}
	if len(server.started) != 1 {
		t.Fatalf("StartAgent calls = %d, want no retry", len(server.started))
	}
}
