package spawn

import (
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

// Result reports the placement, launch, and first-prompt state of a spawn.
type Result struct {
	Name            string   `json:"name"`
	Harness         string   `json:"harness"`
	DetectedHarness *string  `json:"detected_harness"`
	AgentStatus     *string  `json:"agent_status"`
	WorkspaceID     *string  `json:"workspace_id"`
	TabID           *string  `json:"tab_id"`
	PaneID          *string  `json:"pane_id"`
	Cwd             *string  `json:"cwd"`
	Argv            []string `json:"argv"`
	WorktreePath    *string  `json:"worktree_path"`
	Split           bool     `json:"split"`
	Prompted        bool     `json:"prompted"`
}

func pointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Render writes a successful spawn, or a startup recovery hint after the
// generic failure lines that libagent writes first.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(*Result)
	if !ok {
		return nil
	}
	if o.Error != nil {
		if o.Status != "partial" && o.Status != "unknown" {
			return nil
		}
		switch o.Error.Phase {
		case "agent.wait":
			if o.Error.Code == "agent_blocked" {
				_, err := fmt.Fprintf(w, "Agent is waiting on a startup prompt. Inspect with: herdr agent get %s; herdr agent read %s\n", r.Name, r.Name)
				return err
			}
			fallthrough
		case "agent.start":
			_, err := fmt.Fprintf(w, "Startup was not confirmed; a process may still be running. Inspect with: herdr agent get %s; herdr agent read %s\n", r.Name, r.Name)
			return err
		}
		return nil
	}
	if _, err := fmt.Fprintf(w, "Spawned %s (%s) in %s / %s / %s\n  cwd: %s\n  worktree: %s\n", r.Name, r.Harness, display(r.WorkspaceID), display(r.TabID), display(r.PaneID), display(r.Cwd), display(r.WorktreePath)); err != nil {
		return err
	}
	if r.Prompted {
		_, err := fmt.Fprintf(w, "Message submitted to %s.\n", display(r.PaneID))
		return err
	}
	return nil
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}

// PositionalError explains the native-argument separator requirement.
func PositionalError() error { return libagent.Invalid("native positional arguments must follow --") }
