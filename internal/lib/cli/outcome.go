package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Outcome is the stable JSON envelope for every command operation.
type Outcome struct {
	Operation string   `json:"operation"`
	Status    string   `json:"status"`
	Result    any      `json:"result"`
	Effects   []Effect `json:"effects"`
	Error     *Failure `json:"error"`
	input     bool
}

// NewOutcome starts a successful operation with no effects yet. Effects is
// non-nil so the JSON envelope always encodes "effects":[].
func NewOutcome(operation string) Outcome {
	return Outcome{Operation: operation, Status: "success", Effects: []Effect{}}
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Phase   string `json:"phase"`
	// coded records that Fail removed the code prefix from Message.
	coded bool
}

// Text is the failure for humans: Message with the "code: " prefix that Fail
// removed from it, so human text keeps the code that JSON keeps in Code.
func (f Failure) Text() string {
	if f.coded {
		return f.Code + ": " + f.Message
	}
	return f.Message
}

type Effect struct {
	Action string `json:"action"`
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Path   string `json:"path,omitempty"`
}

// Fail records err as the outcome's failure. An error located by AtPhase
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
	if errors.As(err, &input) {
		code = "invalid_input"
		o.input = true
	}
	message, coded := err.Error(), false
	if c, ok := Coded(err); ok {
		code = c
		message, coded = strings.CutPrefix(message, code+": ")
		var remote *herdr.Error
		if mutating && errors.As(err, &remote) && remote.Uncertain {
			o.Status = "unknown"
		} else if phase == "agent.start" && (code == "timeout" || code == "agent_not_ready") {
			o.Status = "partial"
		}
	}
	o.Error = &Failure{Code: code, Message: message, Phase: phase, coded: coded}
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

func (e *ResultError) Error() string { return e.Outcome.Error.Text() }
func (e *ResultError) ExitCode() int { return e.Outcome.ExitCode() }
func (e *ResultError) Rendered()     {}

// InvalidOutcome wraps command syntax errors in the same public envelope.
func InvalidOutcome(operation string, err error) Outcome {
	o := NewOutcome(operation)
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
		if _, err := fmt.Fprintf(w, "%s: %s (%s)\n", o.Status, o.Error.Text(), o.Error.Phase); err != nil {
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
		return &OutputError{Cause: err}
	}
	if o.Error != nil {
		return &ResultError{Outcome: o}
	}
	return nil
}
