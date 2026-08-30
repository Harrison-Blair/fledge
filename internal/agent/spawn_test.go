package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/profile"
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
			name:      "default placement uses the caller workspace",
			caller:    Caller{WorkspaceID: "wsC", PaneID: "wsC:tab1:pane1"},
			opts:      SpawnOptions{Name: "rev", Harness: "claude"},
			wantCalls: []string{"CreateTab(wsC,rev)", "StartAgent(rev,claude,ws1:tab9:pane1)"},
			want:      SpawnResult{Name: "rev", Harness: "claude", WorkspaceID: "ws1", TabID: "ws1:tab9", PaneID: "ws1:tab9:pane1"},
		},
		{
			name:      "default placement falls back to the focused workspace",
			caller:    Caller{Session: "fledge-demo-00000001"},
			opts:      SpawnOptions{Name: "rev", Harness: "claude", Label: "review pass"},
			wantCalls: []string{"Workspaces()", "CreateTab(wsF,review pass)", "StartAgent(rev,claude,ws1:tab9:pane1)"},
			want:      SpawnResult{Name: "rev", Harness: "claude", WorkspaceID: "ws1", TabID: "ws1:tab9", PaneID: "ws1:tab9:pane1"},
		},
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
			opts:      SpawnOptions{Name: "rev", Harness: "cursor", Pane: "ws1:tab3:pane2", Ratio: &ratio},
			wantCalls: []string{"SplitPane(ws1:tab3:pane2,right,0.35)", "StartAgent(rev,cursor,ws1:tab3:pane7)"},
			want:      SpawnResult{Name: "rev", Harness: "cursor", WorkspaceID: "ws1", TabID: "ws1:tab3", PaneID: "ws1:tab3:pane7"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newFakeHerder()
			client.panes = test.panes

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
			opts.Name, opts.Harness = "rev", "claude"

			result, err := Spawn(context.Background(), client, Caller{WorkspaceID: "wsC"}, opts)
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
				Name:    "rev",
				Harness: test.harness,
				Model:   test.model,
				Profile: &configured,
				Args:    []string{"--default", "one", "--explicit", "two"},
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

	_, err := Spawn(context.Background(), client, Caller{RecordPath: recordPath, WorkspaceID: "wsC"}, SpawnOptions{
		Name: "rev", Harness: "claude", Model: "opus", Profile: &configured,
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

	_, err := Spawn(context.Background(), client, Caller{RecordPath: recordPath, WorkspaceID: "wsC"}, SpawnOptions{
		Name: "rev", Harness: "pi", Model: "provider/model", Profile: &configured,
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

func TestSpawnRejectsProfileConflictsAndUnsupportedDeliveryBeforePaneMutation(t *testing.T) {
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
		{name: "Cursor profile", harness: "cursor", want: "does not support native profile delivery"},
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

func TestSpawnCursorWithoutProfile(t *testing.T) {
	client := newFakeHerder()
	result, err := Spawn(context.Background(), client, Caller{WorkspaceID: "wsC"}, SpawnOptions{Name: "rev", Harness: "cursor", Model: "auto"})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	if result.Harness != "cursor" || result.Profile != "" {
		t.Fatalf("result = %#v, want unprofiled Cursor", result)
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
		{name: "single character name", opts: SpawnOptions{Name: "a", Harness: "claude"}},
		{name: "longest name", opts: SpawnOptions{Name: "a" + strings.Repeat("b", 31), Harness: "claude"}},
		{name: "punctuated name", opts: SpawnOptions{Name: "rev-2_b", Harness: "claude"}},
		{name: "down split", opts: SpawnOptions{Name: "rev", Harness: "claude", Pane: "ws1:tab3:pane1", Split: "down"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Spawn(context.Background(), newFakeHerder(), Caller{WorkspaceID: "wsC"}, test.opts); err != nil {
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
			name:      "no focused workspace",
			opts:      SpawnOptions{Name: "rev", Harness: "claude"},
			wantErr:   "no focused workspace",
			wantCalls: []string{"Workspaces()"},
		},
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
