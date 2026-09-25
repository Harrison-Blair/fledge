package agent

import (
	"context"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Rename names the agent a and confirms the same terminal now carries name.
func (c Client) Rename(ctx context.Context, a herdr.AgentDetails, name string) (herdr.AgentDetails, error) {
	var r herdr.AgentResult
	err := c.Call(ctx, "agent.rename", map[string]any{"target": a.PaneID, "name": name}, &r)
	if err == nil && (r.Type != "agent_info" || !ValidAgentInfo(r.Agent) || r.Agent.TerminalID != a.TerminalID || r.Agent.Name == nil || *r.Agent.Name != name) {
		err = Protocol("incomplete or mismatched agent.rename result")
	}
	return r.Agent, err
}

// Label sets p's pane label to name and, when p is alone in its tab, the tab
// label too; a tab shared with other panes keeps its label. It records an
// updated effect for each label it sets and fails out at the first failed
// request.
func (c Client) Label(ctx context.Context, p herdr.Pane, name string, out *Outcome) error {
	var pane herdr.PaneResult
	err := c.Call(ctx, "pane.rename", map[string]any{"pane_id": p.PaneID, "label": name}, &pane)
	if err == nil && (pane.Type != "pane_info" || pane.Pane.PaneID != p.PaneID || pane.Pane.TabID != p.TabID) {
		err = Protocol("incomplete or mismatched pane.rename result")
	}
	if err != nil {
		out.Fail(err, "pane.rename", true)
		return err
	}
	out.Effects = append(out.Effects, Effect{Action: "updated", Kind: "pane_label", ID: p.PaneID})
	var tab herdr.TabResult
	err = c.Call(ctx, "tab.get", map[string]any{"tab_id": p.TabID}, &tab)
	if err == nil && (tab.Type != "tab_info" || tab.Tab.ID != p.TabID || tab.Tab.PaneCount == nil) {
		err = Protocol("incomplete or mismatched tab.get result")
	}
	if err != nil {
		out.Fail(err, "tab.get", false)
		return err
	}
	if *tab.Tab.PaneCount != 1 || tab.Tab.Label == name {
		return nil
	}
	err = c.Call(ctx, "tab.rename", map[string]any{"tab_id": p.TabID, "label": name}, &tab)
	if err == nil && (tab.Type != "tab_info" || tab.Tab.ID != p.TabID) {
		err = Protocol("incomplete or mismatched tab.rename result")
	}
	if err != nil {
		out.Fail(err, "tab.rename", true)
		return err
	}
	out.Effects = append(out.Effects, Effect{Action: "updated", Kind: "tab", ID: p.TabID})
	return nil
}
