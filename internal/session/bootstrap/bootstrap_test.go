package bootstrap_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/session/bootstrap"
	"fledge/internal/session/sessiontest"
	"fledge/internal/session/types"
)

func bootstrapArgs(choice types.AgentChoice, log *bytes.Buffer) bootstrap.Input {
	return bootstrap.Input{Root: "/projects/my-project", Choice: choice, Log: log}
}

func TestBootstrapPreparesSessionAndStartsAgentWithModel(t *testing.T) {
	server := sessiontest.ReadyBootstrapper()
	var log bytes.Buffer

	err := bootstrap.Run(context.Background(), server, bootstrapArgs(types.AgentChoice{Harness: "claude", Model: "opus"}, &log), sessiontest.FastTiming())
	if err != nil {
		t.Fatalf("bootstrap.Run() error = %v", err)
	}
	if server.StatusCalls != 3 {
		t.Fatalf("Status calls = %d, want polling until running", server.StatusCalls)
	}
	if want := []sessiontest.RenameCall{{ID: "w1", Label: "fledge:my-project"}}; !reflect.DeepEqual(server.RenamedWorkspace, want) {
		t.Fatalf("workspace renames = %#v, want %#v", server.RenamedWorkspace, want)
	}
	if want := []sessiontest.RenameCall{{ID: "w1:t2", Label: "orchestrator"}}; !reflect.DeepEqual(server.RenamedTab, want) {
		t.Fatalf("tab renames = %#v, want %#v", server.RenamedTab, want)
	}
	want := []herdr.StartAgentOptions{{
		Name:   "orchestrator",
		Kind:   "claude",
		PaneID: "w1:p2",
		Args:   []string{"--model", "opus"},
	}}
	if !reflect.DeepEqual(server.Started, want) {
		t.Fatalf("StartAgent options = %#v, want %#v", server.Started, want)
	}

	report := log.String()
	for _, step := range []string{"Herder server running", "w1:p2", "fledge:my-project", "orchestrator", "started claude"} {
		if !strings.Contains(report, step) {
			t.Fatalf("log = %q, want a line about %q", report, step)
		}
	}
}

func TestBootstrapOmitsModelArgumentWhenUnset(t *testing.T) {
	server := sessiontest.ReadyBootstrapper()

	err := bootstrap.Run(context.Background(), server, bootstrapArgs(types.AgentChoice{Harness: "pi"}, &bytes.Buffer{}), sessiontest.FastTiming())
	if err != nil {
		t.Fatalf("bootstrap.Run() error = %v", err)
	}
	if len(server.Started) != 1 {
		t.Fatalf("StartAgent calls = %d, want 1", len(server.Started))
	}
	if args := server.Started[0].Args; len(args) != 0 {
		t.Fatalf("StartAgent args = %#v, want none", args)
	}
}

func TestBootstrapShellOnlyStartsNoAgent(t *testing.T) {
	server := sessiontest.ReadyBootstrapper()
	var log bytes.Buffer

	err := bootstrap.Run(context.Background(), server, bootstrapArgs(types.AgentChoice{}, &log), sessiontest.FastTiming())
	if err != nil {
		t.Fatalf("bootstrap.Run() error = %v", err)
	}
	if len(server.Started) != 0 {
		t.Fatalf("StartAgent calls = %#v, want none", server.Started)
	}
	if len(server.RenamedTab) != 1 {
		t.Fatalf("tab renames = %#v, want the orchestrator rename", server.RenamedTab)
	}
}

func TestBootstrapReportsServerThatNeverStarts(t *testing.T) {
	server := &sessiontest.FakeBootstrapper{Statuses: []sessiontest.StatusResult{{Running: false}}}
	var log bytes.Buffer
	timing := sessiontest.FastTiming()
	timing.Deadline = 20 * time.Millisecond

	err := bootstrap.Run(context.Background(), server, bootstrapArgs(types.AgentChoice{Harness: "pi"}, &log), timing)
	if err == nil || !strings.Contains(err.Error(), "did not start") {
		t.Fatalf("bootstrap.Run() error = %v, want a server-start failure", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bootstrap.Run() error = %v, want a deadline", err)
	}
	if server.WorkspaceCalls != 0 {
		t.Fatalf("Workspaces calls = %d, want none before the server runs", server.WorkspaceCalls)
	}
	if !strings.Contains(log.String(), "failed") {
		t.Fatalf("log = %q, want the failure recorded", log.String())
	}
}

func TestBootstrapStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &sessiontest.FakeBootstrapper{Statuses: []sessiontest.StatusResult{{Running: false}}}
	server.OnStatus = func(call int) {
		if call == 2 {
			cancel()
		}
	}

	err := bootstrap.Run(ctx, server, bootstrapArgs(types.AgentChoice{Harness: "pi"}, &bytes.Buffer{}), sessiontest.FastTiming())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("bootstrap.Run() error = %v, want context.Canceled", err)
	}
}

func TestBootstrapRetriesHerderRejections(t *testing.T) {
	rejected := &herdr.Error{Operation: "agent start", Code: "agent_pane_busy", Message: "wording does not control retries"}
	server := sessiontest.ReadyBootstrapper()
	server.StartErrs = []error{rejected, rejected, nil}

	err := bootstrap.Run(context.Background(), server, bootstrapArgs(types.AgentChoice{Harness: "codex"}, &bytes.Buffer{}), sessiontest.FastTiming())
	if err != nil {
		t.Fatalf("bootstrap.Run() error = %v", err)
	}
	if len(server.Started) != 3 {
		t.Fatalf("StartAgent calls = %d, want two retries", len(server.Started))
	}
}

func TestBootstrapStopsRetryingAtTheLimit(t *testing.T) {
	rejected := &herdr.Error{Operation: "agent start", Code: "agent_pane_busy", Message: "no shell prompt"}
	server := sessiontest.ReadyBootstrapper()
	server.StartErrs = []error{rejected}
	var log bytes.Buffer

	err := bootstrap.Run(context.Background(), server, bootstrapArgs(types.AgentChoice{Harness: "codex"}, &log), sessiontest.FastTiming())
	if !errors.Is(err, rejected) {
		t.Fatalf("bootstrap.Run() error = %v, want %v", err, rejected)
	}
	if len(server.Started) != 3 {
		t.Fatalf("StartAgent calls = %d, want the retry limit", len(server.Started))
	}
	if !strings.Contains(log.String(), "failed") {
		t.Fatalf("log = %q, want the failure recorded", log.String())
	}
}

func TestBootstrapDoesNotRetryOtherFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "transport error", err: errors.New("connection refused")},
		{name: "not ready", err: &herdr.Error{Operation: "agent start", Code: "agent_not_ready", Message: "busy"}},
		{name: "pane unavailable", err: &herdr.Error{Operation: "agent start", Code: "agent_pane_unavailable", Message: "busy"}},
		{name: "old similar code", err: &herdr.Error{Operation: "agent start", Code: "pane_busy", Message: "no shell prompt"}},
		{name: "unknown code", err: &herdr.Error{Operation: "agent start", Code: "future_failure", Message: "busy"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := sessiontest.ReadyBootstrapper()
			server.StartErrs = []error{test.err}

			err := bootstrap.Run(context.Background(), server, bootstrapArgs(types.AgentChoice{Harness: "codex"}, &bytes.Buffer{}), sessiontest.FastTiming())
			if !errors.Is(err, test.err) {
				t.Fatalf("bootstrap.Run() error = %v, want %v", err, test.err)
			}
			if len(server.Started) != 1 {
				t.Fatalf("StartAgent calls = %d, want no retry", len(server.Started))
			}
		})
	}
}

func TestBootstrapKeepsFailureDetailWhenTheDeadlinePasses(t *testing.T) {
	killed := errors.New("signal: killed")
	server := sessiontest.StartedBootstrapper()
	server.OnRenameWorkspace = func(ctx context.Context) error {
		<-ctx.Done()
		return killed
	}
	timing := sessiontest.FastTiming()
	timing.Deadline = 20 * time.Millisecond

	err := bootstrap.Run(context.Background(), server, bootstrapArgs(types.AgentChoice{Harness: "pi"}, &bytes.Buffer{}), timing)
	if !errors.Is(err, killed) {
		t.Fatalf("bootstrap.Run() error = %v, want the failure detail kept", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("bootstrap.Run() error = %v, want a deadline reported as a failure", err)
	}
}
