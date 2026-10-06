// Package assign implements task assign: recording a registered agent as the
// owner of a task whose prerequisites are satisfied, then delivering the brief
// to it as a headed message.
package assign

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options selects the task by ID and the agent by exactly one of Agent's
// Name, Pane, or ID. Force assigns a task whose prerequisites are not all satisfied.
type Options struct {
	ID    string
	Agent identity.Target
	Force bool
}

// Result is the task after assignment with the owner's name when it has one.
type Result struct {
	task.Record
	OwnerName *string `json:"owner_name"`
}

// Run assigns the task and delivers its brief. Assignment and delivery are
// stored separately: a failed delivery leaves the task assigned with the
// failure recorded, and is never retried. A task with unmet prerequisites is
// refused before the agent is resolved unless Force is set; a forced
// assignment records the prerequisites it bypassed as UnmetAtAssign. The
// owner's live session ref is stored on its record, best effort, once the
// assignment is committed.
func Run(ctx context.Context, c libagent.Client, o Options) cli.Outcome {
	return run(ctx, c, o, libagent.NewMessageID())
}

func run(ctx context.Context, c libagent.Client, o Options, messageID string) cli.Outcome {
	out := cli.NewOutcome("task.assign")
	err := task.ValidateID(o.ID)
	if err == nil {
		err = o.Agent.Validate()
	}
	if err == nil && o.Agent.ID != "" {
		err = libagent.ValidateID("agent-id", "agent", o.Agent.ID)
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	// The snapshot lets the locked update refuse if anyone changed the task
	// while the agent was being resolved.
	snapshot, err := task.Get(s, o.ID)
	if err == nil {
		err = task.Require(&snapshot, "assign", task.Created, task.Assigned)
	}
	// Listing every task also refuses a malformed record, even one the task
	// does not wait on; the locked check reads only the prerequisites.
	var rs []task.Record
	if err == nil {
		rs, err = task.List(s)
	}
	if err == nil {
		_, err = waiting(snapshot, task.Index(rs), o.Force)
	}
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	a, _, owner, err := o.Agent.Get(ctx, c)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	if owner == nil {
		owner, err = identity.LiveEndingMismatched(s, a)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
		if owner == nil {
			out.Fail(&cli.Error{Code: "agent_unregistered", Message: fmt.Sprintf("the agent in %s has no Fledge record; register it first with: fledge agent adopt --pane %s", a.PaneID, a.PaneID)}, "identity", false)
			return out
		}
		// The terminal is the identity; the record follows it to a new pane.
		moved, err := identity.Relocate(s, *owner, a)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
		owner = &moved
	}
	r, err := task.Update(s, o.ID, func(r *task.Record) error {
		if !reflect.DeepEqual(*r, snapshot) {
			return &cli.Error{Code: "task_state_changed", Message: fmt.Sprintf("task %s changed while it was being assigned; inspect it with fledge task get --id %s", r.ID, r.ID)}
		}
		byID, err := prerequisites(s, *r)
		if err != nil {
			return err
		}
		unmet, err := waiting(*r, byID, o.Force)
		if err != nil {
			return err
		}
		r.Owner, r.Status, r.AssignedAt, r.UnmetAtAssign = &owner.ID, task.Assigned, task.Now(), unmet
		r.Delivery = &task.Delivery{MessageID: messageID, Pane: a.PaneID}
		return nil
	})
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	out.Effects = append(out.Effects, cli.Effect{Action: "updated", Kind: "task", ID: r.ID})
	out.Result = Result{Record: r, OwnerName: owner.Name}
	identity.Observe(s, observeSession, *owner, &a, time.Now(), &out)
	body := fmt.Sprintf("task: %s · title: %s · complete with: fledge task complete --id %s --summary \"...\"\n%s", r.ID, r.Title, r.ID, r.Brief)
	assignedAt := r.AssignedAt
	if r, ok := task.Deliver(ctx, c, s, &out, libagent.ResolveSender(ctx, c), o.ID, a.PaneID, messageID, body, func(r *task.Record) (*task.Attempt, error) {
		// Record the outcome only for this assignment; the owner may already
		// have completed the task, so the status is not checked.
		if r.AssignedAt == nil || *r.AssignedAt != *assignedAt || r.Owner == nil || *r.Owner != owner.ID || r.Delivery == nil || r.Delivery.MessageID != messageID {
			return nil, &cli.Error{Code: "task_state_changed", Message: fmt.Sprintf("task %s was reassigned before its delivery could be recorded", r.ID)}
		}
		return &r.Delivery.Attempt, nil
	}); ok {
		out.Result = Result{Record: r, OwnerName: owner.Name}
	}
	return out
}

// prerequisites reads r's prerequisites by id. A missing one is left out, so
// it counts as unmet; any other read error is returned.
func prerequisites(s *state.Store, r task.Record) (map[string]task.Record, error) {
	byID := make(map[string]task.Record, len(r.After))
	for _, id := range r.After {
		dep, err := task.Get(s, id)
		if code, _ := cli.Coded(err); code == "task_not_found" {
			continue
		}
		if err != nil {
			return nil, err
		}
		byID[id] = dep
	}
	return byID, nil
}

// waiting returns r's prerequisites that are unmet in byID, nil when there are
// none, and refuses them unless force is set.
func waiting(r task.Record, byID map[string]task.Record, force bool) ([]string, error) {
	unmet := task.Unmet(r, byID)
	switch {
	case len(unmet) == 0:
		return nil, nil
	case !force:
		return nil, &cli.Error{Code: "task_dependencies_unmet", Message: fmt.Sprintf("task %s waits on prerequisites that are not verified or cancelled: %s; assign it once they are, or pass --force", r.ID, strings.Join(unmet, ", "))}
	}
	return unmet, nil
}

// Render writes an assignment, or after a failed delivery, what state the
// task was left in.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if !ok {
		return nil
	}
	owner := *r.Owner
	if r.OwnerName != nil {
		owner = fmt.Sprintf("%s (%s)", *r.OwnerName, *r.Owner)
	}
	var err error
	switch {
	case o.Error == nil:
		_, err = fmt.Fprintf(w, "Assigned task %s to %s; brief delivered to %s as message %s.\n", r.ID, owner, r.Delivery.Pane, r.Delivery.MessageID)
		if err == nil && r.UnmetAtAssign != nil {
			_, err = fmt.Fprintf(w, "Assigned with --force before prerequisites %s were satisfied.\n", strings.Join(r.UnmetAtAssign, ", "))
		}
	case o.Error.Phase == "agent.prompt" && o.Status == "unknown":
		_, err = fmt.Fprintf(w, "Task %s remains assigned to %s; the delivery outcome is unknown and will not be retried.\n", r.ID, owner)
	case o.Error.Phase == "agent.prompt":
		_, err = fmt.Fprintf(w, "Task %s remains assigned to %s; the brief was not delivered and will not be retried.\n", r.ID, owner)
	}
	return err
}

// observeSession is replaceable so tests can fail its write.
var observeSession identity.Observer = identity.ObserveSession
