// Package wait implements agent wait: blocking until live agents reach a state.
// A settled state means the agent's turn ended, not that its work succeeded.
package wait

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// Options selects targets by name and pane, or one agent by record ID, the
// states to match, and a timeout (zero waits indefinitely). Two or more
// targets need exactly one of All or Any.
type Options struct {
	Names, Panes, Until []string
	ID                  string
	Timeout             time.Duration
	All, Any            bool
}

// maxTimeout is the largest finite timeout whose transport margin, added by
// libagent.WaitFromEnvironment, cannot overflow a Duration.
const maxTimeout = time.Duration(math.MaxInt64) - libagent.TransportMargin

// FanOut reports a multi-target wait: one row per target, in target order.
// Winner names the first --any match.
type FanOut struct {
	Mode    string  `json:"mode"`
	Winner  *string `json:"winner"`
	Targets []Row   `json:"targets"`
}

// Row reports one target: matched with its agent, errored, or cancelled.
type Row struct {
	Target  string             `json:"target"`
	Outcome string             `json:"outcome"`
	Agent   *libagent.AgentRow `json:"agent"`
	Error   *libagent.Failure  `json:"error"`
}

// Run waits for one target, or fans out one agent.wait call per target. A
// single target's result is its agent row.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.wait", Status: "success", Effects: []libagent.Effect{}}
	targets, err := validate(o)
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	if len(targets) == 1 {
		var a herdr.AgentDetails
		if o.ID != "" {
			a, err = waitID(ctx, c, o.ID, o.Until, o.Timeout)
		} else {
			a, err = c.Wait(ctx, targets[0], o.Until, o.Timeout)
		}
		if err != nil && ctx.Err() != nil {
			err = cancelled()
		}
		if err != nil {
			out.Fail(err, "agent.wait", false)
			return out
		}
		out.Result = libagent.NewAgentRow(a.Pane)
		return out
	}
	result, err := fanOut(ctx, c, targets, o)
	out.Result = result
	if err != nil {
		out.Fail(err, "agent.wait", false)
	}
	return out
}

// waitID resolves record id to its verified pane, waits there, and fails
// closed if a different terminal answers the wait.
func waitID(ctx context.Context, c libagent.Client, id string, until []string, timeout time.Duration) (herdr.AgentDetails, error) {
	_, target, rec, err := identity.Target{ID: id}.Get(ctx, c)
	if err != nil {
		return herdr.AgentDetails{}, err
	}
	a, err := c.Wait(ctx, target, until, timeout)
	if err == nil {
		err = identity.Verify(*rec, a)
	}
	return a, err
}

func validate(o Options) ([]string, error) {
	targets := append(slices.Clone(o.Names), o.Panes...)
	if o.ID != "" {
		if len(targets) > 0 || o.All || o.Any {
			return nil, libagent.Invalid("--id waits on a single target and excludes --name, --pane, --all, and --any")
		}
		targets = []string{o.ID}
	}
	switch {
	case len(targets) == 0:
		return nil, libagent.Invalid("at least one --name, --pane, or --id is required")
	case o.All && o.Any:
		return nil, libagent.Invalid("at most one of --all or --any is allowed")
	case len(targets) > 1 && !o.All && !o.Any:
		return nil, libagent.Invalid("waiting on several targets requires --all or --any")
	case o.Timeout < 0 || (o.Timeout > 0 && o.Timeout < time.Millisecond):
		return nil, libagent.Invalid("--timeout must be zero (indefinite) or at least 1ms")
	case o.Timeout > maxTimeout:
		return nil, libagent.Invalid("--timeout must be at most %s", maxTimeout)
	}
	for i, t := range targets {
		if strings.TrimSpace(t) == "" {
			return nil, libagent.Invalid("targets must be nonempty")
		}
		if slices.Contains(targets[:i], t) {
			return nil, libagent.Invalid("duplicate target %q", t)
		}
	}
	for _, s := range o.Until {
		if !slices.Contains([]string{"idle", "working", "blocked", "done", "unknown"}, s) {
			return nil, libagent.Invalid("--until must be idle, working, blocked, done, or unknown")
		}
	}
	return targets, nil
}

// fanOut runs one wait per target. --any cancels the rest on its first match;
// errors that end a call after that cancellation, or after ctx ends, are
// reported as cancelled rather than as target failures.
func fanOut(ctx context.Context, c libagent.Client, targets []string, o Options) (FanOut, error) {
	waits, cancel := context.WithCancel(ctx)
	defer cancel()
	type reply struct {
		index int
		agent herdr.AgentDetails
		err   error
	}
	replies := make(chan reply, len(targets))
	for i, target := range targets {
		go func() {
			a, err := c.Wait(waits, target, o.Until, o.Timeout)
			replies <- reply{i, a, err}
		}()
	}
	result := FanOut{Mode: "all", Targets: make([]Row, len(targets))}
	if o.Any {
		result.Mode = "any"
	}
	for i, target := range targets {
		result.Targets[i].Target = target
	}
	for range targets {
		r := <-replies
		row := &result.Targets[r.index]
		switch {
		case r.err == nil:
			agent := libagent.NewAgentRow(r.agent.Pane)
			row.Outcome, row.Agent = "matched", &agent
			if o.Any && result.Winner == nil {
				result.Winner = &row.Target
				cancel()
			}
		case waits.Err() != nil && !serverError(r.err):
			row.Outcome = "cancelled"
		default:
			var failed libagent.Outcome
			failed.Fail(r.err, "agent.wait", false)
			row.Outcome, row.Error = "errored", failed.Error
		}
	}
	if result.Winner != nil {
		return result, nil
	}
	if ctx.Err() != nil {
		return result, cancelled()
	}
	var failures, codes []string
	for _, row := range result.Targets {
		if row.Error != nil {
			failures = append(failures, fmt.Sprintf("%s (%s)", row.Target, row.Error.Code))
			if !slices.Contains(codes, row.Error.Code) {
				codes = append(codes, row.Error.Code)
			}
		}
	}
	if len(failures) == 0 {
		return result, nil
	}
	code := "operation_failed"
	if len(codes) == 1 {
		code = codes[0]
	}
	return result, &herdr.Error{Code: code, Message: fmt.Sprintf("%d of %d targets failed: %s", len(failures), len(targets), strings.Join(failures, ", "))}
}

// serverError reports a definite Herdr answer, as opposed to a local transport
// failure such as the connection closing on cancellation.
func serverError(err error) bool {
	var remote *herdr.Error
	return errors.As(err, &remote) && !remote.Uncertain && remote.Code != "connection_error"
}
func cancelled() error { return &herdr.Error{Code: "cancelled", Message: "wait cancelled"} }

// Render writes a single target's settled state, or one line per fan-out
// target. Fan-out rows are also written after a failure.
func Render(w io.Writer, o libagent.Outcome) error {
	switch r := o.Result.(type) {
	case libagent.AgentRow:
		if o.Error != nil {
			return nil
		}
		who := r.PaneID
		if r.Name != nil && *r.Name != "" {
			who = r.Name
		}
		_, err := fmt.Fprintf(w, "%s is %s.\n", display(who), display(r.AgentStatus))
		return err
	case FanOut:
		for _, row := range r.Targets {
			var line string
			switch row.Outcome {
			case "matched":
				line = fmt.Sprintf("%s is %s", row.Target, display(row.Agent.AgentStatus))
				if r.Winner != nil && *r.Winner == row.Target {
					line += " (first match)"
				}
			case "errored":
				line = fmt.Sprintf("%s failed: %s", row.Target, row.Error.Message)
			default:
				line = row.Target + " was cancelled"
			}
			if _, err := fmt.Fprintln(w, line+"."); err != nil {
				return err
			}
		}
	}
	return nil
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
