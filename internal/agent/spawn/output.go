package spawn

import (
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

// Result reports the placement, launch, registration, and first-prompt state
// of a spawn. RegistrationError explains why a started agent has no record.
// PromptRequested reports a first prompt was given, so Prompted=false
// distinguishes "none requested" from "not submitted".
type Result struct {
	Name              string           `json:"name"`
	Harness           string           `json:"harness"`
	DetectedHarness   *string          `json:"detected_harness"`
	AgentStatus       *string          `json:"agent_status"`
	WorkspaceID       *string          `json:"workspace_id"`
	TabID             *string          `json:"tab_id"`
	PaneID            *string          `json:"pane_id"`
	Cwd               *string          `json:"cwd"`
	Argv              []string         `json:"argv"`
	WorktreePath      *string          `json:"worktree_path"`
	Split             bool             `json:"split"`
	ID                *string          `json:"id"`
	Registered        bool             `json:"registered"`
	RegistrationError *string          `json:"registration_error"`
	Prompted          bool             `json:"prompted"`
	PromptRequested   bool             `json:"prompt_requested"`
	MessageID         *string          `json:"message_id"`
	Sender            *libagent.Sender `json:"sender"`
	Profile           *ProfileRef      `json:"profile"`
}

// ProfileRef names the profile a spawn used and where it came from: source
// is "builtin" or "repo", path the repository file, and base the built-in it
// inherits from.
type ProfileRef struct {
	Name   string  `json:"name"`
	Source string  `json:"source"`
	Path   *string `json:"path"`
	Base   *string `json:"base"`
}

// Render writes a successful spawn, or startup recovery hints by pane after
// the generic failure lines that libagent writes first.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(*Result)
	if !ok {
		return nil
	}
	if o.Error != nil {
		if o.Status != "partial" && o.Status != "unknown" {
			return nil
		}
		pane := libagent.Display(r.PaneID)
		switch o.Error.Phase {
		case "agent.wait":
			if o.Error.Code == "agent_blocked" {
				if _, err := fmt.Fprintf(w, "Agent is waiting on a startup prompt. Inspect with: fledge agent read --pane %s\nAfter inspecting, answer with: fledge agent send --pane %s --key <key>\n", pane, pane); err != nil {
					return err
				}
				if !r.PromptRequested {
					return nil
				}
				_, err := fmt.Fprintf(w, "The first prompt was not submitted; after resolving the dialog, resend it with: fledge agent message --pane %s --file <brief> (or --body <text>)\n", pane)
				return err
			}
			fallthrough
		case "agent.start":
			if _, err := fmt.Fprintf(w, "Startup was not confirmed; a process may still be running. Inspect with: fledge agent get --pane %s; fledge agent read --pane %s\n", pane, pane); err != nil {
				return err
			}
			if r.PromptRequested {
				_, err := fmt.Fprintln(w, "The first prompt was not submitted.")
				return err
			}
		case "agent.prompt":
			if o.Status == "unknown" {
				_, err := fmt.Fprintf(w, "The first prompt may have been submitted. Inspect with: fledge agent read --pane %s before resending it.\n", pane)
				return err
			}
			if o.Error.Code == "timeout" {
				_, err := fmt.Fprintf(w, "The first prompt was not submitted; resend it with: fledge agent message --pane %s --file <brief> (or --body <text>)\n", pane)
				return err
			}
		}
		return nil
	}
	id := libagent.Display(r.ID)
	if r.RegistrationError != nil {
		id += " (not registered: " + *r.RegistrationError + ")"
	}
	if _, err := fmt.Fprintf(w, "Spawned %s (%s) in %s / %s / %s\n  cwd: %s\n  worktree: %s\n  id: %s\n", r.Name, r.Harness, libagent.Display(r.WorkspaceID), libagent.Display(r.TabID), libagent.Display(r.PaneID), libagent.Display(r.Cwd), libagent.Display(r.WorktreePath), id); err != nil {
		return err
	}
	if p := r.Profile; p != nil {
		source := "built-in"
		if p.Path != nil {
			source = *p.Path
		}
		if p.Base != nil {
			source += ", extends " + *p.Base
		}
		if _, err := fmt.Fprintf(w, "  profile: %s (%s)\n", p.Name, source); err != nil {
			return err
		}
	}
	if r.Prompted {
		_, err := fmt.Fprintf(w, "Message submitted to %s.\n", libagent.Display(r.PaneID))
		return err
	}
	return nil
}

// PositionalError explains the native-argument separator requirement.
func PositionalError() error { return libagent.Invalid("native positional arguments must follow --") }
