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
	return s.newTab(ctx, o, ws, out)
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

// tabLabel labels a tab spawn creates: --tab, else the agent's name.
func (o Options) tabLabel() string {
	if o.Tab != "" {
		return o.Tab
	}
	return o.Name
}
func (s *spawner) initialTab(ctx context.Context, o Options, r herdr.CreatedResult, out *libagent.Outcome) (herdr.Pane, error) {
	if label := o.tabLabel(); r.Tab.Label != label {
		var renamed herdr.TabResult
		err := s.Call(ctx, "tab.rename", map[string]any{"tab_id": r.Tab.ID, "label": label}, &renamed)
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

// newTab creates a tab labeled --tab, else the agent's name, in workspace
// ws. Labels need not be unique; spawn never reuses or splits a tab.
func (s *spawner) newTab(ctx context.Context, o Options, ws string, out *libagent.Outcome) (herdr.Pane, error) {
	if ws == "" {
		return herdr.Pane{}, fmt.Errorf("destination workspace could not be resolved")
	}
	params := shellParams(o)
	params["workspace_id"] = ws
	params["label"] = o.tabLabel()
	var r herdr.CreatedResult
	err := s.Call(ctx, "tab.create", params, &r)
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
