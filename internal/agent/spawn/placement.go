package spawn

import (
	"context"
	"fmt"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func workspace(snap *herdr.Snapshot, name, id string) (string, error) {
	if name == "" && id == "" {
		return "", nil
	}
	if id != "" {
		for _, w := range snap.Workspaces {
			if w.ID == id {
				return id, nil
			}
		}
		return "", libagent.Invalid("workspace ID %q does not exist", id)
	}
	matches := []string{}
	for _, w := range snap.Workspaces {
		if w.Label == name {
			matches = append(matches, w.ID)
		}
	}
	if len(matches) > 1 {
		return "", libagent.Invalid("workspace name %q is ambiguous: %s", name, strings.Join(matches, ", "))
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", nil
}
func tab(snap *herdr.Snapshot, ws, name, id string) (*herdr.Tab, error) {
	matches := []herdr.Tab{}
	for _, t := range snap.Tabs {
		if id != "" && t.ID == id {
			if ws != "" && ws != t.WorkspaceID {
				return nil, libagent.Invalid("tab %s does not belong to workspace %s", id, ws)
			}
			return &t, nil
		}
		if id == "" && t.WorkspaceID == ws && t.Label == name {
			matches = append(matches, t)
		}
	}
	if id != "" {
		return nil, libagent.Invalid("tab ID %q does not exist", id)
	}
	if len(matches) > 1 {
		ids := []string{}
		for _, t := range matches {
			ids = append(ids, t.ID)
		}
		return nil, libagent.Invalid("tab name %q is ambiguous: %s", name, strings.Join(ids, ", "))
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}
	return nil, nil
}
func anchor(snap *herdr.Snapshot, t herdr.Tab) (string, error) {
	for _, l := range snap.Layouts {
		if l.TabID == t.ID && l.WorkspaceID == t.WorkspaceID {
			for _, p := range snap.Panes {
				if p.PaneID == l.FocusedPaneID && p.TabID == t.ID && p.WorkspaceID == t.WorkspaceID {
					return p.PaneID, nil
				}
			}
		}
	}
	return "", libagent.Protocol("selected tab has no valid focused pane in snapshot")
}
func (s *spawner) caller(ctx context.Context) (herdr.Pane, error) {
	if s.CallerPane == "" {
		return herdr.Pane{}, libagent.Invalid("implicit placement requires HERDR_PANE_ID; provide an explicit target")
	}
	var r herdr.PaneResult
	err := s.Call(ctx, "pane.current", map[string]any{"caller_pane_id": s.CallerPane}, &r)
	if err == nil && (r.Type != "pane_current" || !libagent.ValidPane(r.Pane)) {
		err = libagent.AtPhase("pane.current", libagent.Protocol("incomplete pane.current result"))
	}
	return r.Pane, err
}
func shellParams(o Options) map[string]any {
	p := map[string]any{"focus": false}
	if o.Cwd != "" {
		p["cwd"] = o.Cwd
	}
	if len(o.Env) > 0 {
		env := map[string]string{}
		for _, entry := range o.Env {
			k, v, _ := strings.Cut(entry, "=")
			env[k] = v
		}
		p["env"] = env
	}
	return p
}
func (s *spawner) ordinaryPlacement(ctx context.Context, o Options, snap *herdr.Snapshot, out *libagent.Outcome) (herdr.Pane, error) {
	if o.Pane != "" {
		for _, p := range snap.Panes {
			if p.PaneID == o.Pane {
				out.Effects = append(out.Effects, libagent.Effect{Action: "reused", Kind: "pane", ID: p.PaneID})
				return p, nil
			}
		}
		return herdr.Pane{}, libagent.Invalid("pane ID %q does not exist", o.Pane)
	}
	ws, err := workspace(snap, o.Workspace, o.WorkspaceID)
	if err != nil {
		return herdr.Pane{}, err
	}
	if o.TabID != "" {
		t, err := tab(snap, ws, "", o.TabID)
		if err != nil {
			return herdr.Pane{}, err
		}
		if o.Workspace != "" && ws == "" {
			return herdr.Pane{}, libagent.Invalid("workspace %q does not own selected tab", o.Workspace)
		}
		ws = t.WorkspaceID
	}
	createWorkspace := o.Workspace != "" && ws == ""
	source := ""
	if ws == "" && (!createWorkspace || s.CallerPane != "") {
		p, err := s.caller(ctx)
		if err != nil && (!createWorkspace || ctx.Err() != nil) {
			return herdr.Pane{}, err
		}
		if err == nil {
			source = p.WorkspaceID
		}
		if !createWorkspace {
			ws = source
		}
	}
	if createWorkspace {
		params := shellParams(o)
		params["label"] = o.Workspace
		if source != "" {
			params["source_workspace_id"] = source
		}
		var r herdr.CreatedResult
		err = s.Call(ctx, "workspace.create", params, &r)
		if err == nil {
			recordCreated(out, r, true)
			if r.Type != "workspace_created" || !validCreated(r, true) {
				err = libagent.Protocol("incomplete workspace.create result")
			}
		}
		if err != nil {
			out.Fail(err, "workspace.create", true)
			return herdr.Pane{}, err
		}
		return s.initialTab(ctx, o, r, out)
	}
	return s.placeInWorkspace(ctx, o, ws, snap, out)
}
func validCreated(r herdr.CreatedResult, workspace bool) bool {
	return libagent.ValidPane(r.RootPane) && r.Tab.ID == r.RootPane.TabID && r.Tab.WorkspaceID == r.RootPane.WorkspaceID && (!workspace || r.Workspace.ID == r.RootPane.WorkspaceID)
}
func recordCreated(out *libagent.Outcome, r herdr.CreatedResult, workspace bool) {
	if workspace && r.Workspace.ID != "" {
		out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "workspace", ID: r.Workspace.ID})
	}
	if r.Tab.ID != "" {
		out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "tab", ID: r.Tab.ID})
	}
	if r.RootPane.PaneID != "" {
		out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "pane", ID: r.RootPane.PaneID})
		setPlacement(out.Result.(*Result), r.RootPane)
	}
}
func (s *spawner) initialTab(ctx context.Context, o Options, r herdr.CreatedResult, out *libagent.Outcome) (herdr.Pane, error) {
	if o.Tab != "" && r.Tab.Label != o.Tab {
		var renamed herdr.TabResult
		err := s.Call(ctx, "tab.rename", map[string]any{"tab_id": r.Tab.ID, "label": o.Tab}, &renamed)
		if err == nil && (renamed.Type != "tab_info" || renamed.Tab.ID != r.Tab.ID || renamed.Tab.WorkspaceID != r.Tab.WorkspaceID) {
			err = libagent.Protocol("incomplete tab.rename result")
		}
		if err != nil {
			out.Fail(err, "tab.rename", true)
			return herdr.Pane{}, err
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "tab", ID: r.Tab.ID})
	}
	return r.RootPane, nil
}
func (s *spawner) placeInWorkspace(ctx context.Context, o Options, ws string, snap *herdr.Snapshot, out *libagent.Outcome) (herdr.Pane, error) {
	if ws == "" {
		return herdr.Pane{}, fmt.Errorf("destination workspace could not be resolved")
	}
	var selected *herdr.Tab
	var err error
	if o.Tab != "" || o.TabID != "" {
		selected, err = tab(snap, ws, o.Tab, o.TabID)
		if err != nil {
			return herdr.Pane{}, err
		}
	}
	params := shellParams(o)
	params["workspace_id"] = ws
	if selected == nil {
		if o.Tab != "" {
			params["label"] = o.Tab
		}
		var r herdr.CreatedResult
		err = s.Call(ctx, "tab.create", params, &r)
		if err == nil {
			recordCreated(out, r, false)
			if r.Type != "tab_created" || !validCreated(r, false) || r.Tab.WorkspaceID != ws {
				err = libagent.Protocol("incomplete tab.create result")
			}
		}
		if err != nil {
			out.Fail(err, "tab.create", true)
		}
		return r.RootPane, err
	}
	target, err := anchor(snap, *selected)
	if err != nil {
		return herdr.Pane{}, err
	}
	params["target_pane_id"] = target
	params["direction"] = o.Direction
	if o.Ratio != nil {
		params["ratio"] = *o.Ratio
	}
	var r herdr.PaneResult
	err = s.Call(ctx, "pane.split", params, &r)
	if err == nil {
		if r.Pane.PaneID != "" {
			out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "pane", ID: r.Pane.PaneID})
			setPlacement(out.Result.(*Result), r.Pane)
		}
		if r.Type != "pane_info" || !libagent.ValidPane(r.Pane) || r.Pane.TabID != selected.ID || r.Pane.WorkspaceID != ws {
			err = libagent.Protocol("incomplete pane.split result")
		}
	}
	if err != nil {
		out.Fail(err, "pane.split", true)
	} else {
		out.Result.(*Result).Split = true
	}
	return r.Pane, err
}
