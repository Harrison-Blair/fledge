package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

// Outcome is the stable JSON envelope for every agent operation.
type Outcome struct {
	Operation string   `json:"operation"`
	Status    string   `json:"status"`
	Result    any      `json:"result"`
	Effects   []Effect `json:"effects"`
	Error     *Failure `json:"error"`
	input     bool
}
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Phase   string `json:"phase"`
}
type Effect struct {
	Action string `json:"action"`
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Path   string `json:"path,omitempty"`
}
type AgentRow struct {
	Name        *string `json:"name"`
	Harness     *string `json:"harness"`
	AgentStatus *string `json:"agent_status"`
	WorkspaceID *string `json:"workspace_id"`
	TabID       *string `json:"tab_id"`
	PaneID      *string `json:"pane_id"`
	Cwd         *string `json:"cwd"`
}
type SpawnResult struct {
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
}
type ListResult struct {
	Agents []AgentRow `json:"agents"`
}

// GetResult exposes inspection details, preserving unavailable values as null.
type GetResult struct {
	AgentRow
	ForegroundCwd    *string          `json:"foreground_cwd"`
	InteractiveReady *bool            `json:"interactive_ready"`
	LaunchPending    *bool            `json:"launch_pending"`
	Focused          *bool            `json:"focused"`
	Title            *string          `json:"title"`
	AgentSession     *SessionIdentity `json:"agent_session"`
}
type SessionIdentity struct {
	Source  *string `json:"source"`
	Harness *string `json:"harness"`
	Kind    *string `json:"kind"`
	Value   *string `json:"value"`
}
type MessageResult struct {
	AgentRow
	Submitted bool `json:"submitted"`
}
type StopResult struct {
	AgentRow
	Stopped bool `json:"stopped"`
}
type ModelRow struct {
	Harness string  `json:"harness"`
	Model   string  `json:"model"`
	Name    *string `json:"name"`
}
type ModelsResult struct {
	Models []ModelRow `json:"models"`
}

func pointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func row(p herdr.Pane) AgentRow {
	return AgentRow{Name: p.Name, Harness: p.Agent, AgentStatus: pointer(p.AgentStatus), WorkspaceID: pointer(p.WorkspaceID), TabID: pointer(p.TabID), PaneID: pointer(p.PaneID), Cwd: p.Cwd}
}
func (o *Outcome) fail(err error, phase string, mutating bool) {
	var located *phaseError
	if errors.As(err, &located) {
		phase = located.phase
	}
	o.Status = "rejected"
	for _, e := range o.Effects {
		if e.Action != "reused" {
			o.Status = "partial"
			break
		}
	}
	code := "operation_failed"
	var input *InputError
	var remote *herdr.Error
	if errors.As(err, &input) {
		code = "invalid_input"
		o.input = true
	}
	if errors.As(err, &remote) {
		code = remote.Code
		if mutating && remote.Uncertain {
			o.Status = "unknown"
		} else if phase == "agent.start" && (code == "timeout" || code == "agent_not_ready") {
			o.Status = "partial"
		}
	}
	o.Error = &Failure{Code: code, Message: err.Error(), Phase: phase}
}
func (o Outcome) ExitCode() int {
	if o.Error == nil {
		return 0
	}
	if o.input {
		return 2
	}
	return 1
}

// ResultError carries a rendered operation's exit status without duplicate output.
type ResultError struct{ Outcome Outcome }

func (e *ResultError) Error() string { return e.Outcome.Error.Message }
func (e *ResultError) ExitCode() int { return e.Outcome.ExitCode() }

// InvalidOutcome wraps command syntax errors in the same public envelope.
func InvalidOutcome(operation string, err error) Outcome {
	o := Outcome{Operation: operation, Result: nil, Effects: []Effect{}}
	o.fail(invalid("%v", err), "validation", false)
	return o
}

// Write renders one outcome. Error details are never printed twice by callers.
func (o Outcome) Write(w io.Writer, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(o)
	}
	if o.Error != nil {
		if _, err := fmt.Fprintf(w, "%s: %s (%s)\n", o.Status, o.Error.Message, o.Error.Phase); err != nil {
			return err
		}
		for _, effect := range o.Effects {
			value := effect.ID
			if value == "" {
				value = effect.Path
			}
			if _, err := fmt.Fprintf(w, "  %s %s %s\n", effect.Action, effect.Kind, value); err != nil {
				return err
			}
		}
		if r, ok := o.Result.(*SpawnResult); ok && (o.Status == "partial" || o.Status == "unknown") {
			_, err := fmt.Fprintf(w, "Startup was not confirmed; a process may still be running. Inspect with: herdr agent get %s; herdr agent read %s\n", r.Name, r.Name)
			return err
		}
		return nil
	}
	switch r := o.Result.(type) {
	case *SpawnResult:
		_, err := fmt.Fprintf(w, "Spawned %s (%s) in %s / %s / %s\n  cwd: %s\n  worktree: %s\n", r.Name, r.Harness, display(r.WorkspaceID), display(r.TabID), display(r.PaneID), display(r.Cwd), display(r.WorktreePath))
		return err
	case ListResult:
		if len(r.Agents) == 0 {
			_, err := fmt.Fprintln(w, "No live agents.")
			return err
		}
		table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "NAME\tHARNESS\tSTATUS\tWORKSPACE\tTAB\tPANE\tCWD")
		for _, a := range r.Agents {
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", display(a.Name), display(a.Harness), display(a.AgentStatus), display(a.WorkspaceID), display(a.TabID), display(a.PaneID), display(a.Cwd))
		}
		return table.Flush()
	case GetResult:
		return writeGetResult(w, r)
	case MessageResult:
		_, err := fmt.Fprintf(w, "Message submitted to %s.\n", display(r.PaneID))
		return err
	case StopResult:
		_, err := fmt.Fprintf(w, "Stopped %s (%s) in %s.\n", display(r.Name), display(r.Harness), display(r.PaneID))
		return err
	case ModelsResult:
		if len(r.Models) == 0 {
			_, err := fmt.Fprintln(w, "No models discovered.")
			return err
		}
		table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "HARNESS\tMODEL\tNAME")
		for _, m := range r.Models {
			fmt.Fprintf(table, "%s\t%s\t%s\n", m.Harness, m.Model, display(m.Name))
		}
		return table.Flush()
	}
	return nil
}
func writeGetResult(w io.Writer, r GetResult) error {
	session := r.AgentSession
	if session == nil {
		session = &SessionIdentity{}
	}
	for _, f := range []struct{ label, value string }{
		{"Name", display(r.Name)}, {"Harness", display(r.Harness)}, {"Status", display(r.AgentStatus)},
		{"Workspace ID", display(r.WorkspaceID)}, {"Tab ID", display(r.TabID)}, {"Pane ID", display(r.PaneID)},
		{"Working directory", display(r.Cwd)}, {"Foreground working directory", display(r.ForegroundCwd)},
		{"Interactive ready", displayBool(r.InteractiveReady)}, {"Launch pending", displayBool(r.LaunchPending)},
		{"Focused", displayBool(r.Focused)}, {"Title", display(r.Title)},
		{"Session source", display(session.Source)}, {"Session harness", display(session.Harness)},
		{"Session reference kind", display(session.Kind)}, {"Session reference value", display(session.Value)},
	} {
		if _, err := fmt.Fprintf(w, "%s: %s\n", f.label, f.value); err != nil {
			return err
		}
	}
	return nil
}
func displayBool(b *bool) string {
	if b == nil {
		return "-"
	}
	return strconv.FormatBool(*b)
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}

// Finish writes an outcome and returns its exit status to the CLI.
func Finish(o Outcome, w io.Writer, asJSON bool) error {
	if err := o.Write(w, asJSON); err != nil {
		return &OutputError{Cause: err}
	}
	if o.Error != nil {
		return &ResultError{Outcome: o}
	}
	return nil
}

// PositionalError explains the native-argument separator requirement.
func PositionalError() error { return invalid("native positional arguments must follow --") }

// OutputError reports failure to render an outcome without attempting a second write.
type OutputError struct{ Cause error }

func (e *OutputError) Error() string { return e.Cause.Error() }
func (e *OutputError) Unwrap() error { return e.Cause }
func (e *OutputError) ExitCode() int { return 1 }
