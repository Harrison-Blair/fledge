package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"fledge/internal/herdr"
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
			opts:      SpawnOptions{Name: "rev", Kind: "claude"},
			wantCalls: []string{"CreateTab(wsC,rev)", "StartAgent(rev,claude,ws1:tab9:pane1)"},
			want:      SpawnResult{Name: "rev", Kind: "claude", WorkspaceID: "ws1", TabID: "ws1:tab9", PaneID: "ws1:tab9:pane1"},
		},
		{
			name:      "default placement falls back to the focused workspace",
			caller:    Caller{Session: "fledge-demo-00000001"},
			opts:      SpawnOptions{Name: "rev", Kind: "claude", Label: "review pass"},
			wantCalls: []string{"Workspaces()", "CreateTab(wsF,review pass)", "StartAgent(rev,claude,ws1:tab9:pane1)"},
			want:      SpawnResult{Name: "rev", Kind: "claude", WorkspaceID: "ws1", TabID: "ws1:tab9", PaneID: "ws1:tab9:pane1"},
		},
		{
			name:      "new workspace",
			opts:      SpawnOptions{Name: "rev", Kind: "codex", Workspace: "new"},
			wantCalls: []string{"CreateWorkspace(rev)", "StartAgent(rev,codex,ws2:tab1:pane1)"},
			want:      SpawnResult{Name: "rev", Kind: "codex", WorkspaceID: "ws2", TabID: "ws2:tab1", PaneID: "ws2:tab1:pane1"},
		},
		{
			name:      "named workspace",
			caller:    Caller{WorkspaceID: "wsC"},
			opts:      SpawnOptions{Name: "rev", Kind: "codex", Workspace: "wsX"},
			wantCalls: []string{"CreateTab(wsX,rev)", "StartAgent(rev,codex,ws1:tab9:pane1)"},
			want:      SpawnResult{Name: "rev", Kind: "codex", WorkspaceID: "ws1", TabID: "ws1:tab9", PaneID: "ws1:tab9:pane1"},
		},
		{
			name:  "tab splits its focused pane",
			opts:  SpawnOptions{Name: "rev", Kind: "pi", Tab: "ws1:tab3", Split: "down", Ratio: &ratio},
			panes: []herdr.Pane{{ID: "ws1:tab3:pane1", TabID: "ws1:tab3"}, {ID: "ws1:tab3:pane2", TabID: "ws1:tab3", Focused: true}, {ID: "ws1:tab4:pane1", TabID: "ws1:tab4", Focused: true}},
			wantCalls: []string{"Panes(ws1)", "SplitPane(ws1:tab3:pane2,down,0.35)",
				"StartAgent(rev,pi,ws1:tab3:pane7)"},
			want: SpawnResult{Name: "rev", Kind: "pi", WorkspaceID: "ws1", TabID: "ws1:tab3", PaneID: "ws1:tab3:pane7"},
		},
		{
			name:      "tab without a focused pane splits the first one",
			opts:      SpawnOptions{Name: "rev", Kind: "pi", Tab: "ws1:tab3"},
			panes:     []herdr.Pane{{ID: "ws1:tab4:pane1", TabID: "ws1:tab4", Focused: true}, {ID: "ws1:tab3:pane1", TabID: "ws1:tab3"}, {ID: "ws1:tab3:pane2", TabID: "ws1:tab3"}},
			wantCalls: []string{"Panes(ws1)", "SplitPane(ws1:tab3:pane1,right,-)", "StartAgent(rev,pi,ws1:tab3:pane7)"},
			want:      SpawnResult{Name: "rev", Kind: "pi", WorkspaceID: "ws1", TabID: "ws1:tab3", PaneID: "ws1:tab3:pane7"},
		},
		{
			name:      "pane splits directly",
			opts:      SpawnOptions{Name: "rev", Kind: "cursor", Pane: "ws1:tab3:pane2", Ratio: &ratio},
			wantCalls: []string{"SplitPane(ws1:tab3:pane2,right,0.35)", "StartAgent(rev,cursor,ws1:tab3:pane7)"},
			want:      SpawnResult{Name: "rev", Kind: "cursor", WorkspaceID: "ws1", TabID: "ws1:tab3", PaneID: "ws1:tab3:pane7"},
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
			opts.Name, opts.Kind = "rev", "claude"

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

func TestSpawnRejectsInvalidOptionsWithoutCallingHerder(t *testing.T) {
	ratio := 0.5
	for _, test := range []struct {
		name    string
		opts    SpawnOptions
		wantErr string
	}{
		{name: "empty name", opts: SpawnOptions{Kind: "claude"}, wantErr: "must match"},
		{name: "uppercase name", opts: SpawnOptions{Name: "Rev", Kind: "claude"}, wantErr: "must match"},
		{name: "name starting with a digit", opts: SpawnOptions{Name: "1rev", Kind: "claude"}, wantErr: "must match"},
		{name: "over-long name", opts: SpawnOptions{Name: "a" + strings.Repeat("b", 32), Kind: "claude"}, wantErr: "must match"},
		{name: "missing kind", opts: SpawnOptions{Name: "rev"}, wantErr: "kind is required"},
		{name: "workspace and tab", opts: SpawnOptions{Name: "rev", Kind: "claude", Workspace: "new", Tab: "ws1:tab3"}, wantErr: "at most one"},
		{name: "tab and pane", opts: SpawnOptions{Name: "rev", Kind: "claude", Tab: "ws1:tab3", Pane: "ws1:tab3:pane1"}, wantErr: "at most one"},
		{name: "workspace and pane", opts: SpawnOptions{Name: "rev", Kind: "claude", Workspace: "wsX", Pane: "ws1:tab3:pane1"}, wantErr: "at most one"},
		{name: "unknown split", opts: SpawnOptions{Name: "rev", Kind: "claude", Pane: "ws1:tab3:pane1", Split: "sideways"}, wantErr: "must be right or down"},
		{name: "split without a split placement", opts: SpawnOptions{Name: "rev", Kind: "claude", Split: "down"}, wantErr: "split applies"},
		{name: "split with a workspace", opts: SpawnOptions{Name: "rev", Kind: "claude", Workspace: "new", Split: "right"}, wantErr: "split applies"},
		{name: "ratio without a split placement", opts: SpawnOptions{Name: "rev", Kind: "claude", Ratio: &ratio}, wantErr: "ratio applies"},
		{name: "ratio with a workspace", opts: SpawnOptions{Name: "rev", Kind: "claude", Workspace: "new", Ratio: &ratio}, wantErr: "ratio applies"},
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
		{name: "single character name", opts: SpawnOptions{Name: "a", Kind: "claude"}},
		{name: "longest name", opts: SpawnOptions{Name: "a" + strings.Repeat("b", 31), Kind: "claude"}},
		{name: "punctuated name", opts: SpawnOptions{Name: "rev-2_b", Kind: "claude"}},
		{name: "down split", opts: SpawnOptions{Name: "rev", Kind: "claude", Pane: "ws1:tab3:pane1", Split: "down"}},
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
	client.errs["StartAgent"] = want
	client.errs["ClosePane"] = errors.New("close failed")

	_, err := Spawn(context.Background(), client, Caller{}, SpawnOptions{Name: "rev", Kind: "claude", Pane: "ws1:tab3:pane2"})
	if !errors.Is(err, want) {
		t.Fatalf("Spawn() error = %v, want %v", err, want)
	}
	if !strings.Contains(err.Error(), "ws1:tab3:pane7") {
		t.Fatalf("Spawn() error = %v, want it to name the new pane", err)
	}
	wantCalls := []string{"SplitPane(ws1:tab3:pane2,right,-)", "StartAgent(rev,claude,ws1:tab3:pane7)", "ClosePane(ws1:tab3:pane7)"}
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
			opts:      SpawnOptions{Name: "rev", Kind: "claude"},
			wantErr:   "no focused workspace",
			wantCalls: []string{"Workspaces()"},
		},
		{
			name:      "empty tab",
			opts:      SpawnOptions{Name: "rev", Kind: "claude", Tab: "ws1:tab3"},
			panes:     []herdr.Pane{{ID: "ws1:tab4:pane1", TabID: "ws1:tab4"}},
			wantErr:   `tab "ws1:tab3" has no panes`,
			wantCalls: []string{"Panes(ws1)"},
		},
		{
			name:      "tab listing failure",
			opts:      SpawnOptions{Name: "rev", Kind: "claude", Tab: "ws1:tab3"},
			errs:      map[string]error{"Panes": errors.New("pane list failed")},
			wantErr:   "pane list failed",
			wantCalls: []string{"Panes(ws1)"},
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
