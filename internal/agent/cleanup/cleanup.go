package cleanup

import (
	"context"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/agent/stop"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/worktree/remove"
)

// Options controls one cleanup. DryRun only reports the plan.
// ResultsCollected asserts that the caller has read the results of its
// workers that own no task, making them eligible.
type Options struct {
	DryRun, ResultsCollected bool
}

// Result lists the caller's workers with live records and the checkouts its
// workers recorded, each with its outcome: planned, done, skipped, or failed.
type Result struct {
	DryRun    bool       `json:"dry_run"`
	Caller    string     `json:"caller"`
	Workers   []Worker   `json:"workers"`
	Checkouts []Checkout `json:"checkouts"`
}

// Worker is one direct worker the caller spawned; stopping it is the action.
type Worker struct {
	ID          string  `json:"id"`
	Name        *string `json:"name"`
	PaneID      *string `json:"pane_id"`
	AgentStatus *string `json:"agent_status"`
	Outcome     string  `json:"outcome"`
	Reason      *string `json:"reason"`
}

// Checkout is one checkout a worker recorded; removing it is the action.
// Base is the ref its spawn created it from.
type Checkout struct {
	Path    string  `json:"path"`
	Branch  *string `json:"branch"`
	Base    *string `json:"base"`
	Worker  string  `json:"worker"`
	Outcome string  `json:"outcome"`
	Reason  *string `json:"reason"`
	// marker and markedBranch identify the owned checkout planned for removal.
	marker, markedBranch string
}

// Run plans the cleanup of the caller's finished workers and their checkouts
// and, unless o.DryRun, carries it out: it stops each planned worker, then
// removes each planned checkout whose worker is gone, never forcing either.
// Guards are refreshed before each action. Safety skips still succeed;
// failed actions fail the outcome after every other action has run.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.cleanup", Status: "success", Effects: []libagent.Effect{}}
	p, err := plan(ctx, c, o)
	if err != nil {
		out.Fail(err, "identity", false)
		return out
	}
	out.Result = p.Result
	if o.DryRun {
		return out
	}
	var failures []libagent.Outcome
	record := func(sub libagent.Outcome) (string, *string) {
		out.Effects = append(out.Effects, sub.Effects...)
		switch {
		case sub.Error == nil:
			return "done", nil
		case sub.Error.Code == "invalid_input" || sub.Error.Code == "agent_identity_stale" || sub.Error.Code == "agent_record_not_found":
			return "skipped", &sub.Error.Message
		}
		failures = append(failures, sub)
		return "failed", &sub.Error.Message
	}
	stopped := map[string]bool{}
	for i := range p.Workers {
		w := &p.Workers[i]
		if w.Outcome != "planned" {
			continue
		}
		stopped[w.ID] = false
		// Recheck its workers and tasks: either may have appeared since planning.
		live, err := identity.LiveByTerminal(p.store)
		var tasks []task.Record
		if err == nil {
			tasks, err = task.List(p.store)
		}
		if err != nil {
			w.Outcome, w.Reason = record(libagent.Outcome{Error: &libagent.Failure{Code: "operation_failed", Message: err.Error(), Phase: "state"}})
			continue
		}
		if r := hold(live, tasks, w.ID, o.ResultsCollected); r != "" {
			w.Outcome, w.Reason = "skipped", &r
			continue
		}
		w.Outcome, w.Reason = record(stop.Run(ctx, c, stop.Options{Selection: selector.Selection{IDs: []string{w.ID}}}))
		stopped[w.ID] = w.Outcome == "done"
	}
	for i := range p.Checkouts {
		ch := &p.Checkouts[i]
		if ch.Outcome != "planned" {
			continue
		}
		if done, ok := stopped[ch.Worker]; ok && !done {
			ch.Outcome, ch.Reason = "skipped", libagent.Pointer("its worker "+ch.Worker+" was not stopped")
			continue
		}
		ch.Outcome, ch.Reason = record(remove.Run(ctx, c, remove.Options{Path: ch.Path, Cwd: p.root, Base: *ch.Base, Marker: ch.marker, MarkedBranch: ch.markedBranch, PlannedPath: ch.Path}))
	}
	out.Result = p.Result
	if len(failures) > 0 {
		first := failures[0].Error
		out.Fail(fmt.Errorf("%d cleanup action(s) failed; first: %s", len(failures), first.Message), first.Phase, false)
		for _, f := range failures {
			if f.Status == "unknown" {
				out.Status = "unknown"
			}
		}
	}
	return out
}
