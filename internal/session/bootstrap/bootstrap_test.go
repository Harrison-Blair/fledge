package bootstrap_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/profile"
	"fledge/internal/session/bootstrap"
	"fledge/internal/session/sessiontest"
	"fledge/internal/session/types"
	"fledge/internal/session/workspace"
)

func bootstrapArgs(choice types.AgentChoice, log *bytes.Buffer) bootstrap.Input {
	return bootstrap.Input{
		Root:   "/projects/my-project",
		Choice: choice,
		Log:    log,
		EnsureWorkspaces: func(_ context.Context, roles ...workspace.Role) (map[workspace.Role]herdr.Workspace, error) {
			ensured := make(map[workspace.Role]herdr.Workspace, len(roles))
			for i, role := range roles {
				ensured[role] = herdr.Workspace{ID: fmt.Sprintf("managed-%d", i+1)}
			}
			return ensured, nil
		},
	}
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
	if want := []sessiontest.RenameCall{{ID: "w1", Label: "f:my-project"}}; !reflect.DeepEqual(server.RenamedWorkspace, want) {
		t.Fatalf("workspace renames = %#v, want %#v", server.RenamedWorkspace, want)
	}
	if want := []sessiontest.RenameCall{{ID: "w1:t2", Label: "fledge-orchestrator"}}; !reflect.DeepEqual(server.RenamedTab, want) {
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
	for _, step := range []string{"Herder server running", "w1:p2", "f:my-project", "fledge-orchestrator", "managed workspace orchestrator is managed-1", "managed workspace agents is managed-2", "started claude"} {
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

func TestBootstrapUsesPinnedProfileThroughNativeHarnessArguments(t *testing.T) {
	configured := profile.Profile{Name: profile.OrchestratorName, Instructions: "line one\nline \"two\""}
	tests := []struct {
		name    string
		harness string
		path    string
		want    []string
	}{
		{
			name:    "pi",
			harness: "pi",
			path:    "/sessions/pi/profile.md",
			want:    []string{"--model", "selected", "--append-system-prompt", "/sessions/pi/profile.md", "--thinking", "high"},
		},
		{
			name:    "claude",
			harness: "claude",
			path:    "/sessions/claude/profile.md",
			want:    []string{"--model", "selected", "--append-system-prompt-file", "/sessions/claude/profile.md", "--thinking", "high"},
		},
		{
			name:    "codex",
			harness: "codex",
			path:    "/sessions/codex/profile.md",
			want:    []string{"--model", "selected", "-c", `developer_instructions="line one\nline \"two\""`, "--thinking", "high"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := sessiontest.ReadyBootstrapper()
			choice := types.AgentChoice{
				Harness: test.harness,
				Model:   "selected",
				Args:    []string{"--thinking", "high"},
				Profile: &configured,
			}
			in := bootstrapArgs(choice, &bytes.Buffer{})
			in.ProfileInstructionsPath = test.path

			if err := bootstrap.Run(context.Background(), server, in, sessiontest.FastTiming()); err != nil {
				t.Fatalf("bootstrap.Run() error = %v", err)
			}
			if len(server.Started) != 1 || !reflect.DeepEqual(server.Started[0].Args, test.want) {
				t.Fatalf("StartAgent calls = %#v, want args %#v", server.Started, test.want)
			}
		})
	}
}

func TestBootstrapProfileDeliveryFailureIsFatalBeforeServerMutation(t *testing.T) {
	tests := []struct {
		name    string
		choice  types.AgentChoice
		path    string
		wantErr string
	}{
		{
			name: "missing instruction artifact",
			choice: types.AgentChoice{
				Harness: "pi",
				Profile: &profile.Profile{Name: profile.OrchestratorName},
			},
			wantErr: "instruction file path is empty",
		},
		{
			name: "unsupported harness",
			choice: types.AgentChoice{
				Harness: "gemini",
				Profile: &profile.Profile{Name: profile.OrchestratorName},
			},
			path:    "/sessions/gemini/profile.md",
			wantErr: "does not support native profile delivery",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := sessiontest.ReadyBootstrapper()
			in := bootstrapArgs(test.choice, &bytes.Buffer{})
			in.ProfileInstructionsPath = test.path
			err := bootstrap.Run(context.Background(), server, in, sessiontest.FastTiming())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("bootstrap.Run() error = %v, want containing %q", err, test.wantErr)
			}
			if server.StatusCalls != 0 || len(server.RenamedWorkspace) != 0 || len(server.Started) != 0 {
				t.Fatalf("server mutated after delivery error: %#v", server)
			}
		})
	}
}

func TestBootstrapEnsureFailureStopsBeforeAgentLaunch(t *testing.T) {
	server := sessiontest.ReadyBootstrapper()
	want := errors.New("workspace reconciliation failed")
	in := bootstrapArgs(types.AgentChoice{Harness: "pi"}, &bytes.Buffer{})
	calls := 0
	in.EnsureWorkspaces = func(_ context.Context, roles ...workspace.Role) (map[workspace.Role]herdr.Workspace, error) {
		calls++
		if wantRoles := []workspace.Role{workspace.Orchestrator, workspace.Agents}; !reflect.DeepEqual(roles, wantRoles) {
			t.Fatalf("EnsureWorkspaces roles = %#v, want %#v", roles, wantRoles)
		}
		if len(server.RenamedWorkspace) != 1 || len(server.RenamedTab) != 1 {
			t.Fatalf("EnsureWorkspaces ran before both renames")
		}
		return nil, want
	}

	err := bootstrap.Run(context.Background(), server, in, sessiontest.FastTiming())
	if !errors.Is(err, want) {
		t.Fatalf("bootstrap.Run() error = %v, want %v", err, want)
	}
	if calls != 1 || len(server.Started) != 0 {
		t.Fatalf("ensure/start calls = %d/%d, want 1/0", calls, len(server.Started))
	}
}

func TestBootstrapShellOnlyEnsuresBothRolesAfterRenamesAndStartsNoAgent(t *testing.T) {
	server := sessiontest.ReadyBootstrapper()
	var log bytes.Buffer
	in := bootstrapArgs(types.AgentChoice{}, &log)
	var calls int
	in.EnsureWorkspaces = func(_ context.Context, roles ...workspace.Role) (map[workspace.Role]herdr.Workspace, error) {
		calls++
		if want := []workspace.Role{workspace.Orchestrator, workspace.Agents}; !reflect.DeepEqual(roles, want) {
			t.Fatalf("EnsureWorkspaces roles = %#v, want %#v", roles, want)
		}
		if len(server.RenamedWorkspace) != 1 || len(server.RenamedTab) != 1 {
			t.Fatalf("EnsureWorkspaces ran before renames: workspace=%#v tab=%#v", server.RenamedWorkspace, server.RenamedTab)
		}
		return map[workspace.Role]herdr.Workspace{
			workspace.Orchestrator: {ID: "w1"},
			workspace.Agents:       {ID: "w2"},
		}, nil
	}

	err := bootstrap.Run(context.Background(), server, in, sessiontest.FastTiming())
	if err != nil {
		t.Fatalf("bootstrap.Run() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("EnsureWorkspaces calls = %d, want one", calls)
	}
	if len(server.Started) != 0 {
		t.Fatalf("StartAgent calls = %#v, want none", server.Started)
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

func TestBootstrapCallsStartAgentOnceOnFailure(t *testing.T) {
	rejected := &herdr.Error{Operation: "agent start", Code: "agent_pane_busy", Message: "readiness is owned by StartAgent"}
	server := sessiontest.ReadyBootstrapper()
	server.StartErrs = []error{rejected, nil}
	var log bytes.Buffer

	err := bootstrap.Run(context.Background(), server, bootstrapArgs(types.AgentChoice{Harness: "codex"}, &log), sessiontest.FastTiming())
	if !errors.Is(err, rejected) {
		t.Fatalf("bootstrap.Run() error = %v, want %v", err, rejected)
	}
	if len(server.Started) != 1 {
		t.Fatalf("StartAgent calls = %d, want exactly one", len(server.Started))
	}
	if !strings.Contains(log.String(), "failed") {
		t.Fatalf("log = %q, want the failure recorded", log.String())
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
