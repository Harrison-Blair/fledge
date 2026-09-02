package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/profile"
	"fledge/internal/session"
	"fledge/internal/session/record"
)

func TestSpawnPlacements(t *testing.T) {
	ratio := 0.35
	for _, test := range []struct {
		name      string
		caller    Caller
		opts      SpawnOptions
		panes     []herdr.Pane
		wantCalls []string
		want      SpawnResult
	}{
		{
			name:      "new workspace",
			opts:      SpawnOptions{Name: "rev", Harness: "codex", Workspace: "new"},
			wantCalls: []string{"CreateWorkspace(rev)", "StartAgent(rev,codex,ws2:tab1:pane1)"},
			want:      SpawnResult{Name: "rev", Harness: "codex", WorkspaceID: "ws2", TabID: "ws2:tab1", PaneID: "ws2:tab1:pane1"},
		},
		{
			name:      "named workspace",
			caller:    Caller{WorkspaceID: "wsC"},
			opts:      SpawnOptions{Name: "rev", Harness: "codex", Workspace: "wsX"},
			wantCalls: []string{"CreateTab(wsX,rev)", "StartAgent(rev,codex,ws1:tab9:pane1)"},
			want:      SpawnResult{Name: "rev", Harness: "codex", WorkspaceID: "ws1", TabID: "ws1:tab9", PaneID: "ws1:tab9:pane1"},
		},
		{
			name:  "tab splits its focused pane",
			opts:  SpawnOptions{Name: "rev", Harness: "pi", Tab: "ws1:tab3", Split: "down", Ratio: &ratio},
			panes: []herdr.Pane{{ID: "ws1:tab3:pane1", TabID: "ws1:tab3"}, {ID: "ws1:tab3:pane2", TabID: "ws1:tab3", Focused: true}, {ID: "ws1:tab4:pane1", TabID: "ws1:tab4", Focused: true}},
			wantCalls: []string{"Panes()", "SplitPane(ws1:tab3:pane2,down,0.35)",
				"StartAgent(rev,pi,ws1:tab3:pane7)"},
			want: SpawnResult{Name: "rev", Harness: "pi", WorkspaceID: "ws1", TabID: "ws1:tab3", PaneID: "ws1:tab3:pane7"},
		},
		{
			name:      "tab without a focused pane splits the first one",
			opts:      SpawnOptions{Name: "rev", Harness: "pi", Tab: "ws1:tab3"},
			panes:     []herdr.Pane{{ID: "ws1:tab4:pane1", TabID: "ws1:tab4", Focused: true}, {ID: "ws1:tab3:pane1", TabID: "ws1:tab3"}, {ID: "ws1:tab3:pane2", TabID: "ws1:tab3"}},
			wantCalls: []string{"Panes()", "SplitPane(ws1:tab3:pane1,right,-)", "StartAgent(rev,pi,ws1:tab3:pane7)"},
			want:      SpawnResult{Name: "rev", Harness: "pi", WorkspaceID: "ws1", TabID: "ws1:tab3", PaneID: "ws1:tab3:pane7"},
		},
		{
			name:      "pane splits directly",
			opts:      SpawnOptions{Name: "rev", Harness: "claude", Pane: "ws1:tab3:pane2", Ratio: &ratio},
			wantCalls: []string{"SplitPane(ws1:tab3:pane2,right,0.35)", "StartAgent(rev,claude,ws1:tab3:pane7)"},
			want:      SpawnResult{Name: "rev", Harness: "claude", WorkspaceID: "ws1", TabID: "ws1:tab3", PaneID: "ws1:tab3:pane7"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeHerder()
			client.panes = test.panes
			// Every case is an explicit placement. Invalid managed-workspace
			// paths prove these branches do not touch the sidecar or project lock.
			test.caller.Root = "relative-unused-root"
			test.caller.RecordPath = "relative-unused-record"

			result, err := Spawn(context.Background(), client, test.caller, test.opts)
			if err != nil {
				t.Fatalf("Spawn() error = %v", err)
			}
			if !reflect.DeepEqual(client.calls, test.wantCalls) {
				t.Fatalf("calls = %#v, want %#v", client.calls, test.wantCalls)
			}
			if result != test.want {
				t.Fatalf("result = %#v, want %#v", result, test.want)
			}
		})
	}
}

func TestSpawnDefaultPlacementReusesManagedAgentsWorkspace(t *testing.T) {
	for _, harness := range []string{"pi", "claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			caller := managedSpawnCaller(t, map[string]string{"orchestrator": "ws-orchestrator", "agents": "ws-agents"})
			client := newFakeHerder()
			client.workspaces = []herdr.Workspace{{ID: "ws-orchestrator"}, {ID: "ws-agents"}, {ID: "ws-focused", Focused: true}}

			result, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: harness})
			if err != nil {
				t.Fatalf("Spawn() error = %v", err)
			}
			wantCalls := []string{"Workspaces()", "CreateTab(ws-agents,rev)", "StartAgent(rev," + harness + ",ws1:tab9:pane1)"}
			if !reflect.DeepEqual(client.calls, wantCalls) {
				t.Fatalf("calls = %#v, want %#v", client.calls, wantCalls)
			}
			want := SpawnResult{Name: "rev", Harness: harness, WorkspaceID: "ws1", TabID: "ws1:tab9", PaneID: "ws1:tab9:pane1"}
			if result != want {
				t.Fatalf("result = %#v, want %#v", result, want)
			}
		})
	}
}

func TestSpawnDefaultPlacementRecreatesDestroyedAgentsWorkspaceOnce(t *testing.T) {
	caller := managedSpawnCaller(t, map[string]string{"orchestrator": "ws-orchestrator", "agents": "ws-destroyed"})
	client := newFakeHerder()
	client.workspaces = []herdr.Workspace{{ID: "ws-orchestrator", Label: "renamed orchestrator"}}
	client.newWorkspace = herdr.WorkspaceCreated{
		Workspace: herdr.Workspace{ID: "ws-agents-new"},
		Tab:       herdr.Tab{ID: "ws-agents-new:root", WorkspaceID: "ws-agents-new"},
		RootPane:  herdr.Pane{ID: "ws-agents-new:root:pane", WorkspaceID: "ws-agents-new", TabID: "ws-agents-new:root"},
	}

	for range 2 {
		if _, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "claude"}); err != nil {
			t.Fatalf("Spawn() error = %v", err)
		}
	}
	wantCalls := []string{
		"Workspaces()",
		"CreateWorkspace(f-agents:" + filepath.Base(caller.Root) + ")",
		"CreateTab(ws-agents-new,rev)",
		"StartAgent(rev,claude,ws1:tab9:pane1)",
		"Workspaces()",
		"CreateTab(ws-agents-new,rev)",
		"StartAgent(rev,claude,ws1:tab9:pane1)",
	}
	if !reflect.DeepEqual(client.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", client.calls, wantCalls)
	}
	ids, err := record.ReadWorkspaces(caller.RecordPath)
	if err != nil {
		t.Fatal(err)
	}
	if ids["orchestrator"] != "ws-orchestrator" || ids["agents"] != "ws-agents-new" {
		t.Fatalf("persisted workspaces = %#v", ids)
	}
}

func TestSpawnDefaultPlacementChecksBothManagedRolesForIdentityConflicts(t *testing.T) {
	caller := managedSpawnCaller(t, map[string]string{"orchestrator": "ws-shared", "agents": "ws-shared"})
	client := newFakeHerder()
	client.workspaces = []herdr.Workspace{{ID: "ws-shared", Label: "renamed"}}

	_, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "claude"})
	var conflict *session.RoleConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Spawn() error = %v, want RoleConflictError", err)
	}
	if conflict.FirstRole != session.OrchestratorWorkspaceRole || conflict.ConflictingRole != session.AgentsWorkspaceRole {
		t.Fatalf("conflict = %#v", conflict)
	}
	if !reflect.DeepEqual(client.calls, []string{"Workspaces()"}) {
		t.Fatalf("calls = %#v, want one workspace list and no placement/start", client.calls)
	}
}

func TestSpawnDefaultPlacementPropagatesManagedWorkspaceErrorsBeforeStart(t *testing.T) {
	t.Run("missing orchestrator", func(t *testing.T) {
		caller := managedSpawnCaller(t, nil)
		client := newFakeHerder()
		client.workspaces = nil
		_, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "claude"})
		var missing *session.MissingError
		if !errors.As(err, &missing) || !reflect.DeepEqual(client.calls, []string{"Workspaces()"}) {
			t.Fatalf("Spawn() error/calls = %v/%#v, want missing orchestrator before placement", err, client.calls)
		}
	})

	t.Run("ambiguous agents", func(t *testing.T) {
		caller := managedSpawnCaller(t, map[string]string{"orchestrator": "ws-orchestrator"})
		label := "f-agents:" + filepath.Base(caller.Root)
		client := newFakeHerder()
		client.workspaces = []herdr.Workspace{{ID: "ws-orchestrator"}, {ID: "ws-a", Label: label}, {ID: "ws-b", Label: label}}
		_, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "claude"})
		var ambiguous *session.AmbiguousError
		if !errors.As(err, &ambiguous) || !reflect.DeepEqual(client.calls, []string{"Workspaces()"}) {
			t.Fatalf("Spawn() error/calls = %v/%#v, want ambiguity before placement", err, client.calls)
		}
	})

	t.Run("workspace list", func(t *testing.T) {
		caller := managedSpawnCaller(t, map[string]string{"orchestrator": "ws-orchestrator", "agents": "ws-agents"})
		client := newFakeHerder()
		want := errors.New("list failed")
		client.errs["Workspaces"] = want
		_, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "claude"})
		if !errors.Is(err, want) || !reflect.DeepEqual(client.calls, []string{"Workspaces()"}) {
			t.Fatalf("Spawn() error/calls = %v/%#v", err, client.calls)
		}
	})

	t.Run("workspace create", func(t *testing.T) {
		caller := managedSpawnCaller(t, map[string]string{"orchestrator": "ws-orchestrator", "agents": "ws-destroyed"})
		client := newFakeHerder()
		client.workspaces = []herdr.Workspace{{ID: "ws-orchestrator"}}
		want := errors.New("create failed")
		client.errs["CreateWorkspace"] = want
		_, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "claude"})
		if !errors.Is(err, want) || len(client.calls) != 2 || strings.HasPrefix(client.calls[len(client.calls)-1], "StartAgent(") {
			t.Fatalf("Spawn() error/calls = %v/%#v", err, client.calls)
		}
	})

	t.Run("record read", func(t *testing.T) {
		caller := managedSpawnCaller(t, nil)
		if err := os.WriteFile(filepath.Join(caller.RecordPath, record.WorkspacesFileName), []byte("not json\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		client := newFakeHerder()
		_, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "claude"})
		if err == nil || !strings.Contains(err.Error(), "read managed workspaces") || len(client.calls) != 0 {
			t.Fatalf("Spawn() error/calls = %v/%#v", err, client.calls)
		}
	})

	t.Run("project lock", func(t *testing.T) {
		root := t.TempDir()
		caller := Caller{Root: root, RecordPath: filepath.Join(root, ".fledge", "sessions", "missing")}
		client := newFakeHerder()
		_, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "claude"})
		if err == nil || !strings.Contains(err.Error(), "lock project") || len(client.calls) != 0 {
			t.Fatalf("Spawn() error/calls = %v/%#v", err, client.calls)
		}
	})

	t.Run("record write", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write a read-only record directory")
		}
		caller := managedSpawnCaller(t, map[string]string{"orchestrator": "ws-orchestrator", "agents": "ws-destroyed"})
		client := newFakeHerder()
		client.workspaces = []herdr.Workspace{{ID: "ws-orchestrator"}, {ID: "ws-agents", Label: "f-agents:" + filepath.Base(caller.Root)}}
		if err := os.Chmod(caller.RecordPath, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(caller.RecordPath, 0o755) })
		_, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "claude"})
		if err == nil || !strings.Contains(err.Error(), "write managed workspaces") || !reflect.DeepEqual(client.calls, []string{"Workspaces()"}) {
			t.Fatalf("Spawn() error/calls = %v/%#v", err, client.calls)
		}
	})
}

func TestSpawnCleansProfileArtifactWhenManagedWorkspaceEnsureFails(t *testing.T) {
	caller := managedSpawnCaller(t, map[string]string{"orchestrator": "ws-orchestrator", "agents": "ws-agents"})
	configured := profile.Profile{Name: "fledge-test", Instructions: "pinned instructions"}
	client := newFakeHerder()
	client.errs["Workspaces"] = errors.New("workspace lookup failed")

	_, err := Spawn(context.Background(), client, caller, SpawnOptions{Name: "rev", Harness: "pi", Profile: &configured})
	if err == nil || !strings.Contains(err.Error(), "workspace lookup failed") {
		t.Fatalf("Spawn() error = %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(caller.RecordPath, agentProfilesDirectory))
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("profile artifact cleanup entries/error = %#v/%v", entries, readErr)
	}
}

func TestSpawnDefaultPlacementRejectsMissingRootBeforeEffects(t *testing.T) {
	client := newFakeHerder()
	_, err := Spawn(context.Background(), client, Caller{RecordPath: t.TempDir()}, SpawnOptions{Name: "rev", Harness: "claude"})
	if err == nil || !strings.Contains(err.Error(), "project root") {
		t.Fatalf("Spawn() error = %v, want missing project root", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("Herder calls = %#v, want none", client.calls)
	}
}

func TestSpawnPassesModelBeforeHarnessArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		opts SpawnOptions
		want []string
	}{
		{name: "model only", opts: SpawnOptions{Model: "opus"}, want: []string{"--model", "opus"}},
		{name: "harness arguments only", opts: SpawnOptions{Args: []string{"--extra"}}, want: []string{"--extra"}},
		{name: "model then harness arguments", opts: SpawnOptions{Model: "opus", Args: []string{"--extra", "value"}}, want: []string{"--model", "opus", "--extra", "value"}},
		{name: "neither", opts: SpawnOptions{}, want: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeHerder()
			opts := test.opts
			opts.Name, opts.Harness, opts.Workspace = "rev", "claude", "wsC"

			result, err := Spawn(context.Background(), client, Caller{}, opts)
			if err != nil {
				t.Fatalf("Spawn() error = %v", err)
			}
			if !reflect.DeepEqual(client.startArgs, test.want) {
				t.Fatalf("start args = %#v, want %#v", client.startArgs, test.want)
			}
			if result.Model != test.opts.Model {
				t.Fatalf("result model = %q, want %q", result.Model, test.opts.Model)
			}
		})
	}
}

func TestSpawnDeliversProfilesThroughNativeHarnessArguments(t *testing.T) {
	configured := profile.Profile{Name: "fledge-test", Instructions: "do the work\n"}
	for _, test := range []struct {
		name       string
		harness    string
		model      string
		wantPrefix []string
		fileFlag   string
	}{
		{
			name:       "Pi file",
			harness:    "pi",
			model:      "provider/model",
			wantPrefix: []string{"--model", "provider/model", "--append-system-prompt"},
			fileFlag:   "--append-system-prompt",
		},
		{
			name:       "Claude file",
			harness:    "claude",
			model:      "opus",
			wantPrefix: []string{"--model", "opus", "--append-system-prompt-file"},
			fileFlag:   "--append-system-prompt-file",
		},
		{
			name:       "Codex inline TOML",
			harness:    "codex",
			model:      "gpt",
			wantPrefix: []string{"--model", "gpt", "-c", `developer_instructions="do the work\n"`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			recordPath := t.TempDir()
			client := newFakeHerder()
			result, err := Spawn(context.Background(), client, Caller{RecordPath: recordPath, WorkspaceID: "wsC"}, SpawnOptions{
				Name:      "rev",
				Harness:   test.harness,
				Model:     test.model,
				Profile:   &configured,
				Workspace: "wsC",
				Args:      []string{"--default", "one", "--explicit", "two"},
			})
			if err != nil {
				t.Fatalf("Spawn() error = %v", err)
			}
			if result.Profile != configured.Name || result.Harness != test.harness {
				t.Fatalf("result = %#v, want harness and profile", result)
			}

			wantArgs := append([]string(nil), test.wantPrefix...)
			if test.fileFlag != "" {
				if len(client.startArgs) < 4 || client.startArgs[2] != test.fileFlag {
					t.Fatalf("start args = %#v, want native file flag", client.startArgs)
				}
				path := client.startArgs[3]
				if filepath.Dir(filepath.Dir(path)) != filepath.Join(recordPath, agentProfilesDirectory) {
					t.Fatalf("profile path = %q, want beneath current session record", path)
				}
				contents, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read retained profile artifact: %v", err)
				}
				if string(contents) != configured.Instructions {
					t.Fatalf("profile instructions = %q, want exact snapshot %q", contents, configured.Instructions)
				}
				wantArgs = append(wantArgs, path)
			}
			wantArgs = append(wantArgs, "--default", "one", "--explicit", "two")
			if !reflect.DeepEqual(client.startArgs, wantArgs) {
				t.Fatalf("start args = %#v, want %#v", client.startArgs, wantArgs)
			}
		})
	}
}

func TestSpawnCleansProfileArtifactWhenAgentStartFails(t *testing.T) {
	recordPath := t.TempDir()
	configured := profile.Profile{Name: "fledge-test", Instructions: "pinned instructions"}
	client := newFakeHerder()
	client.errs["StartAgent"] = errors.New("start failed")

	_, err := Spawn(context.Background(), client, Caller{RecordPath: recordPath}, SpawnOptions{
		Name: "rev", Harness: "claude", Model: "opus", Profile: &configured, Workspace: "wsC",
	})
	if err == nil || !strings.Contains(err.Error(), "start failed") {
		t.Fatalf("Spawn() error = %v, want start failure", err)
	}
	if len(client.startArgs) != 4 {
		t.Fatalf("start args = %#v, want model and profile path", client.startArgs)
	}
	if _, err := os.Lstat(client.startArgs[3]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("profile artifact still exists after failed start: %v", err)
	}
}

func TestSpawnCleansProfileArtifactWhenPlacementFails(t *testing.T) {
	recordPath := t.TempDir()
	configured := profile.Profile{Name: "fledge-test", Instructions: "pinned instructions"}
	client := newFakeHerder()
	client.errs["CreateTab"] = errors.New("placement failed")

	_, err := Spawn(context.Background(), client, Caller{RecordPath: recordPath}, SpawnOptions{
		Name: "rev", Harness: "pi", Model: "provider/model", Profile: &configured, Workspace: "wsC",
	})
	if err == nil || !strings.Contains(err.Error(), "placement failed") {
		t.Fatalf("Spawn() error = %v, want placement failure", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(recordPath, agentProfilesDirectory))
	if readErr != nil {
		t.Fatalf("read profile artifact directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("profile artifacts remain after placement failure: %#v", entries)
	}
}

func TestSpawnRejectsProfileConflictsBeforePaneMutation(t *testing.T) {
	configured := profile.Profile{Name: "fledge-test", Instructions: "instructions"}
	for _, test := range []struct {
		name    string
		harness string
		args    []string
		want    string
	}{
		{name: "Pi conflict", harness: "pi", args: []string{"--append-system-prompt=mine"}, want: "conflicts"},
		{name: "Claude conflict", harness: "claude", args: []string{"--system-prompt-file", "mine"}, want: "conflicts"},
		{name: "Codex conflict", harness: "codex", args: []string{"-c", `developer_instructions="mine"`}, want: "conflicts"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeHerder()
			recordPath := t.TempDir()
			_, err := Spawn(context.Background(), client, Caller{RecordPath: recordPath, WorkspaceID: "wsC"}, SpawnOptions{
				Name: "rev", Harness: test.harness, Profile: &configured, Args: test.args,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Spawn() error = %v, want %q", err, test.want)
			}
			if len(client.calls) != 0 {
				t.Fatalf("Herder calls = %#v, want no pane mutation", client.calls)
			}
			if _, err := os.Lstat(filepath.Join(recordPath, agentProfilesDirectory)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("profile artifact directory changed before rejection: %v", err)
			}
		})
	}
}

func TestSpawnRejectsSymlinkedProfileArtifactDirectoryBeforePaneMutation(t *testing.T) {
	recordPath := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(recordPath, agentProfilesDirectory)); err != nil {
		t.Fatal(err)
	}
	configured := profile.Profile{Name: "fledge-test", Instructions: "instructions"}
	client := newFakeHerder()

	_, err := Spawn(context.Background(), client, Caller{RecordPath: recordPath, WorkspaceID: "wsC"}, SpawnOptions{
		Name: "rev", Harness: "pi", Model: "provider/model", Profile: &configured,
	})
	if err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("Spawn() error = %v, want symlink rejection", err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("Herder calls = %#v, want no pane mutation", client.calls)
	}
}

func TestSpawnRejectsInvalidOptionsWithoutCallingHerder(t *testing.T) {
	ratio := 0.5
	for _, test := range []struct {
		name    string
		opts    SpawnOptions
		wantErr string
	}{
		{name: "empty name", opts: SpawnOptions{Harness: "claude"}, wantErr: "must match"},
		{name: "uppercase name", opts: SpawnOptions{Name: "Rev", Harness: "claude"}, wantErr: "must match"},
		{name: "name starting with a digit", opts: SpawnOptions{Name: "1rev", Harness: "claude"}, wantErr: "must match"},
		{name: "over-long name", opts: SpawnOptions{Name: "a" + strings.Repeat("b", 32), Harness: "claude"}, wantErr: "must match"},
		{name: "missing harness", opts: SpawnOptions{Name: "rev"}, wantErr: "harness is required"},
		{name: "OpenCode", opts: SpawnOptions{Name: "rev", Harness: "opencode"}, wantErr: "unsupported harness"},
		{name: "unknown harness", opts: SpawnOptions{Name: "rev", Harness: "gemini"}, wantErr: "unsupported harness"},
		{name: "workspace and tab", opts: SpawnOptions{Name: "rev", Harness: "claude", Workspace: "new", Tab: "ws1:tab3"}, wantErr: "at most one"},
		{name: "tab and pane", opts: SpawnOptions{Name: "rev", Harness: "claude", Tab: "ws1:tab3", Pane: "ws1:tab3:pane1"}, wantErr: "at most one"},
		{name: "workspace and pane", opts: SpawnOptions{Name: "rev", Harness: "claude", Workspace: "wsX", Pane: "ws1:tab3:pane1"}, wantErr: "at most one"},
		{name: "unknown split", opts: SpawnOptions{Name: "rev", Harness: "claude", Pane: "ws1:tab3:pane1", Split: "sideways"}, wantErr: "must be right or down"},
		{name: "split without a split placement", opts: SpawnOptions{Name: "rev", Harness: "claude", Split: "down"}, wantErr: "split applies"},
		{name: "split with a workspace", opts: SpawnOptions{Name: "rev", Harness: "claude", Workspace: "new", Split: "right"}, wantErr: "split applies"},
		{name: "ratio without a split placement", opts: SpawnOptions{Name: "rev", Harness: "claude", Ratio: &ratio}, wantErr: "ratio applies"},
		{name: "ratio with a workspace", opts: SpawnOptions{Name: "rev", Harness: "claude", Workspace: "new", Ratio: &ratio}, wantErr: "ratio applies"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeHerder()

			_, err := Spawn(context.Background(), client, Caller{WorkspaceID: "wsC"}, test.opts)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Spawn() error = %v, want %q", err, test.wantErr)
			}
			if len(client.calls) != 0 {
				t.Fatalf("Spawn() called Herder after a validation failure: %#v", client.calls)
			}
		})
	}
}

func TestSpawnAcceptsBoundaryNamesAndSplits(t *testing.T) {
	for _, test := range []struct {
		name string
		opts SpawnOptions
	}{
		{name: "single character name", opts: SpawnOptions{Name: "a", Harness: "claude", Workspace: "wsC"}},
		{name: "longest name", opts: SpawnOptions{Name: "a" + strings.Repeat("b", 31), Harness: "claude", Workspace: "wsC"}},
		{name: "punctuated name", opts: SpawnOptions{Name: "rev-2_b", Harness: "claude", Workspace: "wsC"}},
		{name: "down split", opts: SpawnOptions{Name: "rev", Harness: "claude", Pane: "ws1:tab3:pane1", Split: "down"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Spawn(context.Background(), newFakeHerder(), Caller{}, test.opts); err != nil {
				t.Fatalf("Spawn() error = %v", err)
			}
		})
	}
}

func TestSpawnClosesTheNewPaneWhenTheAgentFailsToStart(t *testing.T) {
	client := newFakeHerder()
	want := errors.New("harness never reached the prompt")
	closeErr := errors.New("close failed")
	client.errs["StartAgent"] = want
	client.errs["ClosePane"] = closeErr

	_, err := Spawn(context.Background(), client, Caller{}, SpawnOptions{Name: "rev", Harness: "claude", Pane: "ws1:tab3:pane2"})
	if !errors.Is(err, want) {
		t.Fatalf("Spawn() error = %v, want %v", err, want)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("Spawn() error = %v, want cleanup failure %v", err, closeErr)
	}
	if !strings.Contains(err.Error(), "ws1:tab3:pane7") {
		t.Fatalf("Spawn() error = %v, want it to name the new pane", err)
	}
	wantCalls := []string{"SplitPane(ws1:tab3:pane2,right,-)", "StartAgent(rev,claude,ws1:tab3:pane7)", "ClosePane(ws1:tab3:pane7)"}
	if !reflect.DeepEqual(client.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", client.calls, wantCalls)
	}
}

func TestSpawnFailureUsesFreshCleanupContextAndPreservesAllFailures(t *testing.T) {
	type contextKey string
	const key contextKey = "request"

	startErr := errors.New("start subprocess failed")
	cleanupErr := errors.New("close pane failed")
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key, "preserved"))
	defer cancel()

	client := newFakeHerder()
	client.onStartAgent = func(context.Context) error {
		cancel()
		return startErr
	}
	client.onClosePane = func(cleanupCtx context.Context) error {
		if err := cleanupCtx.Err(); err != nil {
			t.Errorf("cleanup context already ended: %v", err)
		}
		if got := cleanupCtx.Value(key); got != "preserved" {
			t.Errorf("cleanup context value = %v, want preserved", got)
		}
		deadline, ok := cleanupCtx.Deadline()
		if !ok {
			t.Error("cleanup context has no deadline")
		} else if remaining := time.Until(deadline); remaining < 4*time.Second || remaining > cleanupTimeout {
			t.Errorf("cleanup deadline is %v away, want approximately %v", remaining, cleanupTimeout)
		}
		return cleanupErr
	}

	_, err := Spawn(ctx, client, Caller{}, SpawnOptions{Name: "rev", Harness: "claude", Pane: "source-pane"})
	for _, want := range []error{startErr, context.Canceled, cleanupErr} {
		if !errors.Is(err, want) {
			t.Errorf("Spawn() error = %v, want it to include %v", err, want)
		}
	}
	wantCalls := []string{"SplitPane(source-pane,right,-)", "StartAgent(rev,claude,ws1:tab3:pane7)", "ClosePane(ws1:tab3:pane7)"}
	if !reflect.DeepEqual(client.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", client.calls, wantCalls)
	}
}

func TestSpawnDoesNotAttributeCancellationThatBeginsDuringCleanup(t *testing.T) {
	startErr := errors.New("start failed")
	cleanupErr := errors.New("cleanup failed")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := newFakeHerder()
	client.errs["StartAgent"] = startErr
	client.onClosePane = func(context.Context) error {
		cancel()
		return cleanupErr
	}

	_, err := Spawn(ctx, client, Caller{}, SpawnOptions{Name: "rev", Harness: "claude", Pane: "source-pane"})
	if !errors.Is(err, startErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("Spawn() error = %v, want start and cleanup failures", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("Spawn() error = %v, must not attribute cleanup-time cancellation", err)
	}
}

func TestSpawnTreatsTabIDsAsOpaqueAcrossWorkspaces(t *testing.T) {
	client := newFakeHerder()
	client.panes = []herdr.Pane{
		{ID: "other-pane", WorkspaceID: "workspace-a", TabID: "other-tab", Focused: true},
		{ID: "first-target-pane", WorkspaceID: "workspace-b", TabID: "opaque-tab-id"},
		{ID: "focused-target-pane", WorkspaceID: "workspace-b", TabID: "opaque-tab-id", Focused: true},
	}

	_, err := Spawn(context.Background(), client, Caller{}, SpawnOptions{Name: "rev", Harness: "pi", Tab: "opaque-tab-id"})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	wantCalls := []string{"Panes()", "SplitPane(focused-target-pane,right,-)", "StartAgent(rev,pi,ws1:tab3:pane7)"}
	if !reflect.DeepEqual(client.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", client.calls, wantCalls)
	}
}

func TestSpawnPropagatesPlacementFailures(t *testing.T) {
	for _, test := range []struct {
		name      string
		opts      SpawnOptions
		caller    Caller
		panes     []herdr.Pane
		errs      map[string]error
		wantErr   string
		wantCalls []string
	}{
		{
			name:      "empty tab",
			opts:      SpawnOptions{Name: "rev", Harness: "claude", Tab: "ws1:tab3"},
			panes:     []herdr.Pane{{ID: "ws1:tab4:pane1", TabID: "ws1:tab4"}},
			wantErr:   `tab "ws1:tab3" has no panes`,
			wantCalls: []string{"Panes()"},
		},
		{
			name:      "tab listing failure",
			opts:      SpawnOptions{Name: "rev", Harness: "claude", Tab: "ws1:tab3"},
			errs:      map[string]error{"Panes": errors.New("pane list failed")},
			wantErr:   "pane list failed",
			wantCalls: []string{"Panes()"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeHerder()
			client.workspaces = []herdr.Workspace{{ID: "wsA"}}
			client.panes = test.panes
			if test.errs != nil {
				client.errs = test.errs
			}

			_, err := Spawn(context.Background(), client, test.caller, test.opts)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Spawn() error = %v, want %q", err, test.wantErr)
			}
			if !reflect.DeepEqual(client.calls, test.wantCalls) {
				t.Fatalf("calls = %#v, want %#v", client.calls, test.wantCalls)
			}
		})
	}
}

func TestValidateInitialPrompt(t *testing.T) {
	for _, test := range []struct {
		name    string
		text    string
		wantErr string // empty means the prompt is valid
		notErr  string // when set, the error must not mention this
	}{
		{name: "empty", text: "", wantErr: "empty"},
		{name: "whitespace only is not empty", text: "   \n\t  ", wantErr: ""},
		{name: "exact limit ASCII", text: strings.Repeat("a", maxInitialPromptBytes), wantErr: ""},
		{name: "exact limit multibyte UTF-8", text: strings.Repeat("é", maxInitialPromptBytes/2), wantErr: ""},
		{name: "one byte over limit", text: strings.Repeat("a", maxInitialPromptBytes+1), wantErr: "exceed"},
		{name: "invalid UTF-8", text: "ok\xffbad", wantErr: "UTF-8"},
		{name: "in-bound NUL", text: "before\x00after", wantErr: "NUL"},
		{name: "oversize outranks invalid UTF-8", text: strings.Repeat("a", maxInitialPromptBytes) + "\xff\xff", wantErr: "exceed", notErr: "UTF-8"},
		{name: "invalid UTF-8 outranks NUL", text: "\xff\x00", wantErr: "UTF-8", notErr: "NUL"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Guard the multibyte fixture: the boundary must be bytes, not runes.
			if strings.Contains(test.name, "multibyte") && len(test.text) != maxInitialPromptBytes {
				t.Fatalf("multibyte fixture is %d bytes, want exactly %d", len(test.text), maxInitialPromptBytes)
			}
			err := ValidateInitialPrompt(test.text)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateInitialPrompt() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateInitialPrompt() error = %v, want containing %q", err, test.wantErr)
			}
			if test.notErr != "" && strings.Contains(err.Error(), test.notErr) {
				t.Fatalf("ValidateInitialPrompt() error = %v, must not mention %q (wrong precedence)", err, test.notErr)
			}
		})
	}
}

func TestSpawnRejectsInvalidInitialPromptBeforeHerderCallOrArtifact(t *testing.T) {
	configured := profile.Profile{Name: "fledge-test", Instructions: "instructions"}
	for _, test := range []struct {
		name   string
		prompt string
	}{
		{name: "empty", prompt: ""},
		{name: "oversize", prompt: strings.Repeat("a", maxInitialPromptBytes+1)},
		{name: "invalid UTF-8", prompt: "bad\xff"},
		{name: "in-bound NUL", prompt: "has\x00nul"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeHerder()
			recordPath := t.TempDir()
			prompt := test.prompt
			result, err := Spawn(context.Background(), client, Caller{RecordPath: recordPath, WorkspaceID: "wsC"}, SpawnOptions{
				Name: "rev", Harness: "claude", Profile: &configured, InitialPrompt: &prompt,
			})
			if err == nil {
				t.Fatalf("Spawn() error = nil, want an initial prompt rejection")
			}
			if result != (SpawnResult{}) {
				t.Fatalf("result = %#v, want the zero result", result)
			}
			if len(client.calls) != 0 {
				t.Fatalf("Herder calls = %#v, want none before the validation failure", client.calls)
			}
			if _, err := os.Lstat(filepath.Join(recordPath, agentProfilesDirectory)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("profile artifact directory changed before rejection: %v", err)
			}
		})
	}
}

func TestSpawnWithoutInitialPromptDoesNotPrompt(t *testing.T) {
	client := newFakeHerder()

	result, err := Spawn(context.Background(), client, Caller{}, SpawnOptions{Name: "rev", Harness: "claude", Workspace: "wsC"})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	for _, call := range client.calls {
		if strings.HasPrefix(call, "PromptAgent(") {
			t.Fatalf("Spawn() prompted without an initial prompt: %#v", client.calls)
		}
	}
	if !reflect.DeepEqual(client.promptOpts, herdr.PromptOptions{}) {
		t.Fatalf("prompt options = %#v, want none", client.promptOpts)
	}
	want := SpawnResult{Name: "rev", Harness: "claude", WorkspaceID: "ws1", TabID: "ws1:tab9", PaneID: "ws1:tab9:pane1"}
	if result != want {
		t.Fatalf("result = %#v, want %#v", result, want)
	}
}

func TestSpawnDeliversInitialPromptAfterStartAgent(t *testing.T) {
	client := newFakeHerder()
	// A non-nil, non-trivial response must be discarded, not surfaced.
	client.prompted = json.RawMessage(`{"type":"agent_prompted","id":"cli:agent:prompt"}`)
	prompt := "line one\nline two\twith \"quotes\", a trailing space \nand unicode 世界 ✅"

	result, err := Spawn(context.Background(), client, Caller{}, SpawnOptions{
		Name: "rev", Harness: "claude", Workspace: "wsC", InitialPrompt: &prompt,
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	// PromptAgent runs exactly once, strictly after the agent starts.
	wantCalls := []string{"CreateTab(wsC,rev)", "StartAgent(rev,claude,ws1:tab9:pane1)", "PromptAgent(rev)"}
	if !reflect.DeepEqual(client.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", client.calls, wantCalls)
	}

	// Delivery is fire-and-forget with the exact prompt bytes preserved.
	wantOpts := herdr.PromptOptions{Target: "rev", Text: prompt, Wait: false, Until: nil, TimeoutMS: 0}
	if !reflect.DeepEqual(client.promptOpts, wantOpts) {
		t.Fatalf("prompt options = %#v, want %#v", client.promptOpts, wantOpts)
	}
	if client.promptOpts.Text != prompt {
		t.Fatalf("prompt text = %q, want the exact bytes %q", client.promptOpts.Text, prompt)
	}

	want := SpawnResult{Name: "rev", Harness: "claude", WorkspaceID: "ws1", TabID: "ws1:tab9", PaneID: "ws1:tab9:pane1"}
	if result != want {
		t.Fatalf("result = %#v, want %#v (raw response must be ignored)", result, want)
	}
}

func TestSpawnDoesNotPromptWhenLaunchFailsBeforeDelivery(t *testing.T) {
	configured := profile.Profile{Name: "fledge-test", Instructions: "pinned instructions"}
	prompt := "deliver me once launched"
	for _, test := range []struct {
		name    string
		caller  Caller
		opts    SpawnOptions
		errs    map[string]error
		wantErr string
	}{
		{
			name:    "validation failure",
			caller:  Caller{WorkspaceID: "wsC"},
			opts:    SpawnOptions{Name: "Rev", Harness: "claude", InitialPrompt: &prompt},
			wantErr: "must match",
		},
		{
			name:    "profile preparation conflict",
			caller:  Caller{WorkspaceID: "wsC"},
			opts:    SpawnOptions{Name: "rev", Harness: "claude", Profile: &configured, Args: []string{"--system-prompt-file", "mine"}, InitialPrompt: &prompt},
			wantErr: "conflicts",
		},
		{
			name:    "placement failure",
			caller:  Caller{},
			opts:    SpawnOptions{Name: "rev", Harness: "claude", Workspace: "wsC", InitialPrompt: &prompt},
			errs:    map[string]error{"CreateTab": errors.New("placement failed")},
			wantErr: "placement failed",
		},
		{
			name:    "start failure",
			caller:  Caller{},
			opts:    SpawnOptions{Name: "rev", Harness: "claude", Pane: "ws1:tab3:pane2", InitialPrompt: &prompt},
			errs:    map[string]error{"StartAgent": errors.New("start failed")},
			wantErr: "start failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeHerder()
			if test.errs != nil {
				client.errs = test.errs
			}
			result, err := Spawn(context.Background(), client, test.caller, test.opts)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Spawn() error = %v, want %q", err, test.wantErr)
			}
			if result != (SpawnResult{}) {
				t.Fatalf("result = %#v, want the zero result", result)
			}
			for _, call := range client.calls {
				if strings.HasPrefix(call, "PromptAgent(") {
					t.Fatalf("Spawn() prompted after a pre-delivery failure: %#v", client.calls)
				}
			}
		})
	}
}

func TestSpawnInitialPromptDeliveryFailureIsSafeAndPreservesLaunch(t *testing.T) {
	recordPath := t.TempDir()
	configured := profile.Profile{Name: "fledge-test", Instructions: "pinned instructions\n"}
	prompt := "PROMPT_SECRET_PAYLOAD then a second line"
	cause := &herdr.Error{
		Operation: "cli:agent:prompt OPERATION_SECRET",
		Code:      "agent_blocked",
		Message:   "MESSAGE_SECRET the target is busy",
	}
	client := newFakeHerder()
	client.errs["PromptAgent"] = cause

	result, err := Spawn(context.Background(), client, Caller{RecordPath: recordPath}, SpawnOptions{
		Name: "rev", Harness: "claude", Model: "opus", Profile: &configured, Workspace: "wsC", InitialPrompt: &prompt,
	})

	// The launched agent and its placement coordinates survive delivery failure.
	if result.Name != "rev" || result.Harness != "claude" || result.Profile != configured.Name {
		t.Fatalf("result = %#v, want the launched agent", result)
	}
	if result.WorkspaceID == "" || result.TabID == "" || result.PaneID == "" {
		t.Fatalf("result = %#v, want retained placement coordinates", result)
	}

	// The error chain carries the typed marker and the original cause.
	var promptErr *InitialPromptError
	if !errors.As(err, &promptErr) {
		t.Fatalf("Spawn() error = %v, want a *InitialPromptError in the chain", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("Spawn() error = %v, want the original cause in the chain", err)
	}
	if got := promptErr.SafeCode(); got != "agent_blocked" {
		t.Fatalf("SafeCode() = %q, want agent_blocked", got)
	}

	// The rendered string leaks neither the prompt, the cause, nor Herder detail.
	for _, secret := range []string{"PROMPT_SECRET_PAYLOAD", "OPERATION_SECRET", "MESSAGE_SECRET", "agent_blocked", cause.Error()} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("Spawn() error string %q leaks %q", err.Error(), secret)
		}
	}

	// The pane is never closed and the profile artifact remains byte-for-byte.
	for _, call := range client.calls {
		if strings.HasPrefix(call, "ClosePane(") {
			t.Fatalf("Spawn() closed the pane after a delivery failure: %#v", client.calls)
		}
	}
	if len(client.startArgs) < 4 || client.startArgs[2] != "--append-system-prompt-file" {
		t.Fatalf("start args = %#v, want the profile file flag and path", client.startArgs)
	}
	contents, readErr := os.ReadFile(client.startArgs[3])
	if readErr != nil {
		t.Fatalf("profile artifact missing after delivery failure: %v", readErr)
	}
	if string(contents) != configured.Instructions {
		t.Fatalf("profile artifact = %q, want the exact snapshot %q", contents, configured.Instructions)
	}
}

func managedSpawnCaller(t *testing.T, ids map[string]string) Caller {
	t.Helper()
	const sessionName = "fledge-spawn-00000001"
	root := agentProject(t, sessionName)
	recordPath := filepath.Join(root, ".fledge", "sessions", sessionName)
	if ids != nil {
		if err := record.WriteWorkspaces(recordPath, ids); err != nil {
			t.Fatal(err)
		}
	}
	return Caller{Session: sessionName, Root: root, RecordPath: recordPath, WorkspaceID: "caller-workspace", PaneID: "caller-pane"}
}

func TestInitialPromptErrorSafeCode(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
		want  string
	}{
		{name: "nil cause", cause: nil, want: "unknown"},
		{name: "whitelisted blocked", cause: &herdr.Error{Operation: "op", Code: "agent_blocked", Message: "busy"}, want: "agent_blocked"},
		{name: "whitelisted pane not found", cause: &herdr.Error{Operation: "op", Code: "agent_pane_not_found", Message: "gone"}, want: "agent_pane_not_found"},
		{name: "wrapped whitelisted code", cause: fmt.Errorf("prompt agent: %w", &herdr.Error{Code: "agent_blocked"}), want: "agent_blocked"},
		{name: "non-agent structured code", cause: &herdr.Error{Code: "pane_not_found"}, want: "unknown"},
		{name: "arbitrary malicious code", cause: &herdr.Error{Code: "'; DROP TABLE agents; --"}, want: "unknown"},
		{name: "plain transport error", cause: errors.New("connection reset by peer"), want: "unknown"},
		{name: "context cancellation", cause: context.Canceled, want: "unknown"},
		{name: "context deadline", cause: context.DeadlineExceeded, want: "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := (&InitialPromptError{Cause: test.cause}).SafeCode(); got != test.want {
				t.Fatalf("SafeCode() = %q, want %q", got, test.want)
			}
		})
	}

	t.Run("nil receiver", func(t *testing.T) {
		var e *InitialPromptError
		if got := e.SafeCode(); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown", got)
		}
	})
}

func TestSpawnInitialPromptContextCancellationLeavesAgentRunning(t *testing.T) {
	prompt := "deliver under cancellation"
	client := newFakeHerder()
	// PromptAgent surfaces a cancellation observed during delivery.
	client.errs["PromptAgent"] = context.Canceled

	result, err := Spawn(context.Background(), client, Caller{}, SpawnOptions{
		Name: "rev", Harness: "claude", Workspace: "wsC", InitialPrompt: &prompt,
	})

	if result == (SpawnResult{}) {
		t.Fatalf("result = %#v, want the launched agent retained", result)
	}
	var promptErr *InitialPromptError
	if !errors.As(err, &promptErr) {
		t.Fatalf("Spawn() error = %v, want *InitialPromptError", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Spawn() error = %v, want context.Canceled in the chain", err)
	}
	if got := promptErr.SafeCode(); got != "unknown" {
		t.Fatalf("SafeCode() = %q, want unknown for context cancellation", got)
	}

	promptCalls := 0
	for _, call := range client.calls {
		if strings.HasPrefix(call, "PromptAgent(") {
			promptCalls++
		}
		if strings.HasPrefix(call, "ClosePane(") {
			t.Fatalf("Spawn() cleaned up after cancellation: %#v", client.calls)
		}
	}
	if promptCalls != 1 {
		t.Fatalf("PromptAgent called %d times, want exactly one (no retry, no poll)", promptCalls)
	}
}

// forgingAsError spoofs errors.As: its As sets the target to a forged
// whitelisted *herdr.Error and reports a match. A correct SafeCode must never
// consult this hook.
type forgingAsError struct{}

func (forgingAsError) Error() string { return "totally legitimate" }

func (forgingAsError) As(target any) bool {
	if p, ok := target.(**herdr.Error); ok {
		*p = &herdr.Error{Operation: "forged", Code: "agent_blocked", Message: "forged"}
		return true
	}
	return false
}

// blindAsError reports an errors.As match without ever setting the target,
// which would leave a naive errors.As classifier holding a nil *herdr.Error to
// dereference.
type blindAsError struct{}

func (blindAsError) Error() string { return "blind match" }

func (blindAsError) As(any) bool { return true }

// unwrapValue wraps one error through a plain Unwrap() error without touching
// it, so a typed-nil inner value can be reached without calling its Error.
type unwrapValue struct{ inner error }

func (unwrapValue) Error() string { return "wrapper" }

func (w unwrapValue) Unwrap() error { return w.inner }

// selfUnwrap unwraps to itself, forming a cycle a bounded traversal must
// survive.
type selfUnwrap struct{}

func (selfUnwrap) Error() string { return "cycle" }

func (s selfUnwrap) Unwrap() error { return s }

func TestInitialPromptErrorSafeCodeAdversarial(t *testing.T) {
	typedNil := (*herdr.Error)(nil)

	t.Run("direct typed-nil structured cause", func(t *testing.T) {
		if got := (&InitialPromptError{Cause: typedNil}).SafeCode(); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown", got)
		}
	})

	t.Run("wrapped typed-nil structured cause", func(t *testing.T) {
		if got := (&InitialPromptError{Cause: unwrapValue{inner: typedNil}}).SafeCode(); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown", got)
		}
	})

	t.Run("custom As forging a whitelisted code is ignored", func(t *testing.T) {
		// Guard: the naive errors.As path is genuinely fooled by this cause, so
		// SafeCode diverging to unknown proves it never took the hook.
		var viaAs *herdr.Error
		if !errors.As(error(forgingAsError{}), &viaAs) || viaAs == nil || viaAs.Code != "agent_blocked" {
			t.Fatalf("guard: errors.As did not take the forged bait; the test is no longer meaningful")
		}
		if got := (&InitialPromptError{Cause: forgingAsError{}}).SafeCode(); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown (must not consult the As hook)", got)
		}
	})

	t.Run("custom As returning true without a target is ignored", func(t *testing.T) {
		if got := (&InitialPromptError{Cause: blindAsError{}}).SafeCode(); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown (no panic, no match)", got)
		}
	})

	t.Run("joined structured errors classify by depth-first first match", func(t *testing.T) {
		blocked := &herdr.Error{Code: "agent_blocked"}
		paneGone := &herdr.Error{Code: "agent_pane_not_found"}
		if got := (&InitialPromptError{Cause: errors.Join(blocked, paneGone)}).SafeCode(); got != "agent_blocked" {
			t.Fatalf("SafeCode() = %q, want agent_blocked (first join child)", got)
		}
		if got := (&InitialPromptError{Cause: errors.Join(paneGone, blocked)}).SafeCode(); got != "agent_pane_not_found" {
			t.Fatalf("SafeCode() = %q, want agent_pane_not_found (first join child)", got)
		}
	})

	t.Run("earlier unknown structured error shadows a later whitelisted one", func(t *testing.T) {
		unknown := &herdr.Error{Code: "pane_not_found"}
		blocked := &herdr.Error{Code: "agent_blocked"}
		if got := (&InitialPromptError{Cause: errors.Join(unknown, blocked)}).SafeCode(); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown (must not skip past an earlier structured error)", got)
		}
	})

	t.Run("earlier typed-nil structured error shadows a later whitelisted one", func(t *testing.T) {
		blocked := &herdr.Error{Code: "agent_blocked"}
		if got := (&InitialPromptError{Cause: errors.Join(typedNil, blocked)}).SafeCode(); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown (typed-nil first child shadows)", got)
		}
	})

	t.Run("real wrapped whitelisted code still classifies", func(t *testing.T) {
		cause := fmt.Errorf("prompt agent: %w", &herdr.Error{Code: "agent_pane_not_found"})
		if got := (&InitialPromptError{Cause: cause}).SafeCode(); got != "agent_pane_not_found" {
			t.Fatalf("SafeCode() = %q, want agent_pane_not_found", got)
		}
	})

	t.Run("self-unwrapping cycle terminates as unknown", func(t *testing.T) {
		done := make(chan string, 1)
		go func() { done <- (&InitialPromptError{Cause: selfUnwrap{}}).SafeCode() }()
		select {
		case got := <-done:
			if got != "unknown" {
				t.Fatalf("SafeCode() = %q, want unknown", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("SafeCode() hung on a self-unwrapping cycle")
		}
	})
}

// branchNode is a multi-child unwrap node that counts how many times traversal
// calls its Unwrap. Cyclic children turn depth-only traversal into ~2^101
// visits, so these tests prove the work budget, not the clock, is what stops
// the walk.
type branchNode struct {
	children []error
	unwraps  *int
}

func (*branchNode) Error() string { return "branching node" }

func (n *branchNode) Unwrap() []error {
	*n.unwraps++
	return n.children
}

// safeCodeOrHang classifies cause under a watchdog: the deterministic
// assertions live in the callers, and the timeout only converts an unbounded
// traversal into a prompt failure instead of a stuck test run.
func safeCodeOrHang(t *testing.T, cause error) string {
	t.Helper()
	done := make(chan string, 1)
	go func() { done <- (&InitialPromptError{Cause: cause}).SafeCode() }()
	select {
	case got := <-done:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("SafeCode() did not terminate promptly")
		return ""
	}
}

func TestInitialPromptErrorSafeCodeWorkBudget(t *testing.T) {
	t.Run("branching self-cycle stays within the work budget", func(t *testing.T) {
		unwraps := 0
		self := &branchNode{unwraps: &unwraps}
		self.children = []error{self, self}

		if got := safeCodeOrHang(t, self); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown", got)
		}
		if unwraps > maxUnwrapWork {
			t.Fatalf("Unwrap called %d times, want at most the work budget %d", unwraps, maxUnwrapWork)
		}
		if unwraps <= maxUnwrapDepth {
			t.Fatalf("Unwrap called %d times, want more than %d (the budget, not the depth bound, must be what stopped a branching cycle)", unwraps, maxUnwrapDepth)
		}
	})

	t.Run("branching mutual cycle stays within the work budget", func(t *testing.T) {
		unwraps := 0
		a := &branchNode{unwraps: &unwraps}
		b := &branchNode{unwraps: &unwraps}
		a.children = []error{b, b}
		b.children = []error{a, a}

		if got := safeCodeOrHang(t, a); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown", got)
		}
		if unwraps > maxUnwrapWork {
			t.Fatalf("Unwrap called %d times, want at most the work budget %d", unwraps, maxUnwrapWork)
		}
		if unwraps <= maxUnwrapDepth {
			t.Fatalf("Unwrap called %d times, want more than %d (the budget, not the depth bound, must be what stopped a branching cycle)", unwraps, maxUnwrapDepth)
		}
	})

	t.Run("wide fanout stops at the work budget without scanning every child", func(t *testing.T) {
		unwraps := 0
		root := &branchNode{unwraps: &unwraps}
		for range 10_000 {
			root.children = append(root.children, &branchNode{unwraps: &unwraps})
		}

		if got := safeCodeOrHang(t, root); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown", got)
		}
		// Every visit of a branchNode spends one budget unit and calls Unwrap
		// exactly once: the root plus maxUnwrapWork-1 leaves, never all 10000.
		if unwraps != maxUnwrapWork {
			t.Fatalf("Unwrap called %d times, want exactly %d visits", unwraps, maxUnwrapWork)
		}
	})

	t.Run("whitelisted code on the last budgeted visit classifies", func(t *testing.T) {
		// The join costs one visit and each child one more, so the structured
		// error is exactly the maxUnwrapWork-th (final affordable) visit.
		children := make([]error, 0, maxUnwrapWork-1)
		for range maxUnwrapWork - 2 {
			children = append(children, errors.New("noise"))
		}
		children = append(children, &herdr.Error{Code: "agent_blocked"})
		if got := (&InitialPromptError{Cause: errors.Join(children...)}).SafeCode(); got != "agent_blocked" {
			t.Fatalf("SafeCode() = %q, want agent_blocked on the final budgeted visit", got)
		}
	})

	t.Run("whitelisted code one visit past the budget reports unknown", func(t *testing.T) {
		children := make([]error, 0, maxUnwrapWork)
		for range maxUnwrapWork - 1 {
			children = append(children, errors.New("noise"))
		}
		children = append(children, &herdr.Error{Code: "agent_blocked"})
		if got := (&InitialPromptError{Cause: errors.Join(children...)}).SafeCode(); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown once the budget is exhausted", got)
		}
	})

	t.Run("whitelisted code at the exact depth bound classifies", func(t *testing.T) {
		var cause error = &herdr.Error{Code: "agent_pane_not_found"}
		for range maxUnwrapDepth {
			cause = unwrapValue{inner: cause}
		}
		if got := (&InitialPromptError{Cause: cause}).SafeCode(); got != "agent_pane_not_found" {
			t.Fatalf("SafeCode() = %q, want agent_pane_not_found at depth %d", got, maxUnwrapDepth)
		}
	})

	t.Run("whitelisted code one past the depth bound reports unknown", func(t *testing.T) {
		var cause error = &herdr.Error{Code: "agent_pane_not_found"}
		for range maxUnwrapDepth + 1 {
			cause = unwrapValue{inner: cause}
		}
		if got := (&InitialPromptError{Cause: cause}).SafeCode(); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown past depth %d", got, maxUnwrapDepth)
		}
	})
}

// wrapDepth buries leaf under n plain Unwrap() error layers, so traversal must
// descend n levels before it can see the leaf.
func wrapDepth(n int, leaf error) error {
	wrapped := leaf
	for range n {
		wrapped = unwrapValue{inner: wrapped}
	}
	return wrapped
}

// TestInitialPromptErrorSafeCodeTruncationShadowsLaterSiblings pins the
// conservative ordering contract: a branch the depth or work bound cut short
// could have hidden the true first structured error, so traversal must report
// unknown rather than move on to a later sibling — while a branch that was
// fully searched and simply held nothing lets later siblings classify.
func TestInitialPromptErrorSafeCodeTruncationShadowsLaterSiblings(t *testing.T) {
	blocked := &herdr.Error{Code: "agent_blocked"}

	t.Run("depth-truncated earlier branch shadows a later whitelisted sibling", func(t *testing.T) {
		deep := wrapDepth(maxUnwrapDepth+1, errors.New("leaf"))
		if got := safeCodeOrHang(t, errors.Join(deep, blocked)); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown (the truncated branch could hide the first structured error)", got)
		}
	})

	t.Run("work-truncated earlier branch shadows a later whitelisted sibling", func(t *testing.T) {
		// The wide branch stays at depth 2 but holds more nodes than the whole
		// work budget, so truncation here is purely budget exhaustion.
		unwraps := 0
		wide := &branchNode{unwraps: &unwraps}
		for range maxUnwrapWork + 50 {
			wide.children = append(wide.children, errors.New("noise"))
		}
		if got := safeCodeOrHang(t, errors.Join(wide, blocked)); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown (the budget-exhausted branch could hide the first structured error)", got)
		}
		if unwraps != 1 {
			t.Fatalf("Unwrap called %d times, want exactly once for the single wide node", unwraps)
		}
	})

	t.Run("fully searched absent earlier branch lets a later whitelisted sibling classify", func(t *testing.T) {
		absent := errors.Join(errors.New("noise"), unwrapValue{inner: errors.New("leaf")})
		if got := (&InitialPromptError{Cause: errors.Join(absent, &herdr.Error{Code: "agent_pane_not_found"})}).SafeCode(); got != "agent_pane_not_found" {
			t.Fatalf("SafeCode() = %q, want agent_pane_not_found (an exhaustively absent branch must not shadow)", got)
		}
	})

	t.Run("truncation nested inside wrappers and joins propagates unknown", func(t *testing.T) {
		deep := fmt.Errorf("inner: %w", wrapDepth(maxUnwrapDepth+1, errors.New("leaf")))
		cause := fmt.Errorf("outer: %w", errors.Join(deep, blocked))
		if got := safeCodeOrHang(t, cause); got != "unknown" {
			t.Fatalf("SafeCode() = %q, want unknown (nested truncation must reach the top)", got)
		}
	})
}
