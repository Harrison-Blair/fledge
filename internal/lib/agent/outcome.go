package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
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

// NewAgentRow reports a live pane's agent fields, preserving empty values as null.
func NewAgentRow(p herdr.Pane) AgentRow {
	return AgentRow{Name: p.Name, Harness: p.Agent, AgentStatus: Pointer(p.AgentStatus), WorkspaceID: Pointer(p.WorkspaceID), TabID: Pointer(p.TabID), PaneID: Pointer(p.PaneID), Cwd: p.Cwd}
}

// Pointer returns nil for an empty string, so optional JSON fields encode null.
func Pointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Display renders an optional value for humans, showing "-" when it is absent.
func Display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}

// Fail records err as the outcome's failure. A located transport error
// overrides phase; mutating marks unconfirmed requests as unknown.
func (o *Outcome) Fail(err error, phase string, mutating bool) {
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

// Invalid builds an InputError from a format string.
func Invalid(format string, args ...any) error {
	return &InputError{Message: fmt.Sprintf(format, args...)}
}

// InputError identifies invalid CLI input (exit status 2).
type InputError struct{ Message string }

func (e *InputError) Error() string { return e.Message }

// ResultError carries a rendered operation's exit status without duplicate output.
type ResultError struct{ Outcome Outcome }

func (e *ResultError) Error() string { return e.Outcome.Error.Message }
func (e *ResultError) ExitCode() int { return e.Outcome.ExitCode() }
func (e *ResultError) Rendered()     {}

// InvalidOutcome wraps command syntax errors in the same public envelope.
func InvalidOutcome(operation string, err error) Outcome {
	o := Outcome{Operation: operation, Result: nil, Effects: []Effect{}}
	o.Fail(Invalid("%v", err), "validation", false)
	return o
}

// HumanRenderer writes a leaf operation's human output. It runs for successes
// and, after the generic failure lines, for failures so it may add hints.
type HumanRenderer func(w io.Writer, o Outcome) error

// Write renders one outcome. JSON never calls render; a nil render writes
// only the generic failure lines.
func (o Outcome) Write(w io.Writer, asJSON bool, render HumanRenderer) error {
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
	}
	if render == nil {
		return nil
	}
	return render(w, o)
}

// Finish writes an outcome and returns its exit status to the CLI.
// Error details are never printed twice by callers.
func Finish(o Outcome, w io.Writer, asJSON bool, render HumanRenderer) error {
	if err := o.Write(w, asJSON, render); err != nil {
		return &cli.OutputError{Cause: err}
	}
	if o.Error != nil {
		return &ResultError{Outcome: o}
	}
	return nil
}
