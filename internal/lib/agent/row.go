package agent

import (
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

type AgentRow struct {
	Name        *string `json:"name"`
	Harness     *string `json:"harness"`
	AgentStatus *string `json:"agent_status"`
	WorkspaceID *string `json:"workspace_id"`
	TabID       *string `json:"tab_id"`
	PaneID      *string `json:"pane_id"`
	Cwd         *string `json:"cwd"`
}

// NewAgentRow reports a live pane's agent fields, preserving empty values as null.
func NewAgentRow(p herdr.Pane) AgentRow {
	return AgentRow{Name: p.Name, Harness: p.Agent, AgentStatus: cli.Pointer(p.AgentStatus), WorkspaceID: cli.Pointer(p.WorkspaceID), TabID: cli.Pointer(p.TabID), PaneID: cli.Pointer(p.PaneID), Cwd: p.Cwd}
}
