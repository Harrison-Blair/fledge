package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/herdr"
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
		return "", invalid("workspace ID %q does not exist", id)
	}
	matches := []string{}
	for _, w := range snap.Workspaces {
		if w.Label == name {
			matches = append(matches, w.ID)
		}
	}
	if len(matches) > 1 {
		return "", invalid("workspace name %q is ambiguous: %s", name, strings.Join(matches, ", "))
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
				return nil, invalid("tab %s does not belong to workspace %s", id, ws)
			}
			return &t, nil
		}
		if id == "" && t.WorkspaceID == ws && t.Label == name {
			matches = append(matches, t)
		}
	}
	if id != "" {
		return nil, invalid("tab ID %q does not exist", id)
	}
	if len(matches) > 1 {
		ids := []string{}
		for _, t := range matches {
			ids = append(ids, t.ID)
		}
		return nil, invalid("tab name %q is ambiguous: %s", name, strings.Join(ids, ", "))
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
	return "", protocol("selected tab has no valid focused pane in snapshot")
}
func (s *Service) caller(ctx context.Context) (herdr.Pane, error) {
	if s.CallerPane == "" {
		return herdr.Pane{}, invalid("implicit placement requires HERDR_PANE_ID; provide an explicit target")
	}
	var r herdr.PaneResult
	err := s.call(ctx, "pane.current", map[string]any{"caller_pane_id": s.CallerPane}, &r)
	if err == nil && (r.Type != "pane_current" || !validPane(r.Pane)) {
		err = &phaseError{phase: "pane.current", cause: protocol("incomplete pane.current result")}
	}
	return r.Pane, err
}
func shellParams(o SpawnOptions) map[string]any {
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
func (s *Service) ordinaryPlacement(ctx context.Context, o SpawnOptions, snap *herdr.Snapshot, out *Outcome) (herdr.Pane, error) {
	if o.Pane != "" {
		for _, p := range snap.Panes {
			if p.PaneID == o.Pane {
				out.Effects = append(out.Effects, Effect{Action: "reused", Kind: "pane", ID: p.PaneID})
				return p, nil
			}
		}
		return herdr.Pane{}, invalid("pane ID %q does not exist", o.Pane)
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
			return herdr.Pane{}, invalid("workspace %q does not own selected tab", o.Workspace)
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
		err = s.call(ctx, "workspace.create", params, &r)
		if err == nil {
			recordCreated(out, r, true)
			if r.Type != "workspace_created" || !validCreated(r, true) {
				err = protocol("incomplete workspace.create result")
			}
		}
		if err != nil {
			out.fail(err, "workspace.create", true)
			return herdr.Pane{}, err
		}
		return s.initialTab(ctx, o, r, out)
	}
	return s.placeInWorkspace(ctx, o, ws, snap, out)
}
func validCreated(r herdr.CreatedResult, workspace bool) bool {
	return validPane(r.RootPane) && r.Tab.ID == r.RootPane.TabID && r.Tab.WorkspaceID == r.RootPane.WorkspaceID && (!workspace || r.Workspace.ID == r.RootPane.WorkspaceID)
}
func recordCreated(out *Outcome, r herdr.CreatedResult, workspace bool) {
	if workspace && r.Workspace.ID != "" {
		out.Effects = append(out.Effects, Effect{Action: "created", Kind: "workspace", ID: r.Workspace.ID})
	}
	if r.Tab.ID != "" {
		out.Effects = append(out.Effects, Effect{Action: "created", Kind: "tab", ID: r.Tab.ID})
	}
	if r.RootPane.PaneID != "" {
		out.Effects = append(out.Effects, Effect{Action: "created", Kind: "pane", ID: r.RootPane.PaneID})
		setPlacement(out.Result.(*SpawnResult), r.RootPane)
	}
}
func (s *Service) initialTab(ctx context.Context, o SpawnOptions, r herdr.CreatedResult, out *Outcome) (herdr.Pane, error) {
	if o.Tab != "" && r.Tab.Label != o.Tab {
		var renamed herdr.TabResult
		err := s.call(ctx, "tab.rename", map[string]any{"tab_id": r.Tab.ID, "label": o.Tab}, &renamed)
		if err == nil && (renamed.Type != "tab_info" || renamed.Tab.ID != r.Tab.ID || renamed.Tab.WorkspaceID != r.Tab.WorkspaceID) {
			err = protocol("incomplete tab.rename result")
		}
		if err != nil {
			out.fail(err, "tab.rename", true)
			return herdr.Pane{}, err
		}
		out.Effects = append(out.Effects, Effect{Action: "updated", Kind: "tab", ID: r.Tab.ID})
	}
	return r.RootPane, nil
}
func (s *Service) placeInWorkspace(ctx context.Context, o SpawnOptions, ws string, snap *herdr.Snapshot, out *Outcome) (herdr.Pane, error) {
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
		err = s.call(ctx, "tab.create", params, &r)
		if err == nil {
			recordCreated(out, r, false)
			if r.Type != "tab_created" || !validCreated(r, false) || r.Tab.WorkspaceID != ws {
				err = protocol("incomplete tab.create result")
			}
		}
		if err != nil {
			out.fail(err, "tab.create", true)
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
	err = s.call(ctx, "pane.split", params, &r)
	if err == nil {
		if r.Pane.PaneID != "" {
			out.Effects = append(out.Effects, Effect{Action: "created", Kind: "pane", ID: r.Pane.PaneID})
			setPlacement(out.Result.(*SpawnResult), r.Pane)
		}
		if r.Type != "pane_info" || !validPane(r.Pane) || r.Pane.TabID != selected.ID || r.Pane.WorkspaceID != ws {
			err = protocol("incomplete pane.split result")
		}
	}
	if err != nil {
		out.fail(err, "pane.split", true)
	} else {
		out.Result.(*SpawnResult).Split = true
	}
	return r.Pane, err
}
