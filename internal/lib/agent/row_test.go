package agent

import (
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestNewAgentRow(t *testing.T) {
	name, kind, cwd := "worker", "claude", "/repo"
	row := NewAgentRow(herdr.Pane{Name: &name, Agent: &kind, AgentStatus: "idle", WorkspaceID: "w1", TabID: "w1:t1", PaneID: "w1:p1", Cwd: &cwd})
	if *row.Name != name || *row.Harness != kind || *row.AgentStatus != "idle" || *row.WorkspaceID != "w1" || *row.TabID != "w1:t1" || *row.PaneID != "w1:p1" || *row.Cwd != cwd {
		t.Fatalf("%+v", row)
	}
	if empty := NewAgentRow(herdr.Pane{}); empty.AgentStatus != nil || empty.PaneID != nil || empty.WorkspaceID != nil || empty.TabID != nil {
		t.Fatalf("empty values must be null: %+v", empty)
	}
}
