// Package assign implements task assign: recording a registered agent as the
// owner of a task whose prerequisites are satisfied, then delivering the brief
// to it as a headed message.
package assign

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options selects the task by ID and the agent by exactly one of Name, Pane,
// or AgentID. Force assigns a task whose prerequisites are not all satisfied.
type Options struct {
	ID, Name, Pane, AgentID string
	Force                   bool
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
// assignment records the prerequisites it bypassed as UnmetAtAssign.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	return run(ctx, c, o, libagent.NewMessageID())
}

func run(ctx context.Context, c libagent.Client, o Options, messageID string) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.assign", Status: "success", Effects: []libagent.Effect{}}
	target := identity.Target{Name: o.Name, Pane: o.Pane, ID: o.AgentID}
	err := task.ValidateID(o.ID)
	if err == nil {
		err = target.Validate()
	}
	if err == nil && o.AgentID != "" && !state.ValidID(o.AgentID) {
		err = libagent.Invalid("--agent-id must be 8 lowercase hexadecimal characters")
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
	if err == nil {
		_, err = waiting(s, snapshot, o.Force)
	}
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	a, _, owner, err := target.Get(ctx, c)
	if err != nil {
		out.Fail(err, "agent.get", false)
		return out
	}
	if owner == nil {
		owner, err = identity.Match(s, a)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
		if owner == nil {
			out.Fail(&herdr.Error{Code: "agent_unregistered", Message: fmt.Sprintf("the agent in %s has no Fledge record; register it first with: fledge agent adopt --pane %s", a.PaneID, a.PaneID)}, "identity", false)
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
			return &herdr.Error{Code: "task_state_changed", Message: fmt.Sprintf("task %s changed while it was being assigned; inspect it with fledge task get --id %s", r.ID, r.ID)}
		}
		unmet, err := waiting(s, *r, o.Force)
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
	out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "task", ID: r.ID})
	out.Result = Result{Record: r, OwnerName: owner.Name}
	sender := libagent.ResolveSender(ctx, c)
	body := fmt.Sprintf("task: %s · title: %s · complete with: fledge task complete --id %s --summary \"...\"\n%s", r.ID, r.Title, r.ID, r.Brief)
	_, deliveryErr := c.Prompt(ctx, a.PaneID, libagent.WithHeader(messageID, sender, body))
	if deliveryErr == nil {
		out.Effects = append(out.Effects, libagent.Effect{Action: "submitted", Kind: "message", ID: a.PaneID})
	}
	assignedAt := r.AssignedAt
	r, err = task.Update(s, o.ID, func(r *task.Record) error {
		// Record the outcome only for this assignment; the owner may already
		// have completed the task, so the status is not checked.
		if r.AssignedAt == nil || *r.AssignedAt != *assignedAt || r.Owner == nil || *r.Owner != owner.ID || r.Delivery == nil || r.Delivery.MessageID != messageID {
			return &herdr.Error{Code: "task_state_changed", Message: fmt.Sprintf("task %s was reassigned before its delivery could be recorded", r.ID)}
		}
		if deliveryErr != nil {
			msg := deliveryErr.Error()
			var remote *herdr.Error
			r.Delivery.Error, r.Delivery.Uncertain = &msg, errors.As(deliveryErr, &remote) && remote.Uncertain
		} else {
			r.Delivery.DeliveredAt = task.Now()
		}
		return nil
	})
	if err == nil {
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "task", ID: r.ID})
		out.Result = Result{Record: r, OwnerName: owner.Name}
	}
	switch {
	case deliveryErr != nil:
		out.Fail(deliveryErr, "agent.prompt", true)
	case err != nil:
		out.Fail(err, "task", false)
	}
	return out
}

// waiting returns r's unmet prerequisites, nil when there are none, and
// refuses them unless force is set.
func waiting(s *state.Store, r task.Record, force bool) ([]string, error) {
	rs, err := task.List(s)
	if err != nil {
		return nil, err
	}
	unmet := task.Unmet(r, task.Index(rs))
	switch {
	case len(unmet) == 0:
		return nil, nil
	case !force:
		return nil, &herdr.Error{Code: "task_dependencies_unmet", Message: fmt.Sprintf("task %s waits on prerequisites that are not verified or cancelled: %s; assign it once they are, or pass --force", r.ID, strings.Join(unmet, ", "))}
	}
	return unmet, nil
}

// Render writes an assignment, or after a failed delivery, what state the
// task was left in.
func Render(w io.Writer, o libagent.Outcome) error {
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
