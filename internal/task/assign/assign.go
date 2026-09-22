// Package assign implements task assign: recording a registered agent as a
// task's owner, then delivering the brief to it as a headed message.
package assign

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"regexp"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options selects the task by ID and the agent by exactly one of Name, Pane,
// or AgentID.
type Options struct{ ID, Name, Pane, AgentID string }

// Result is the task after assignment with the owner's name when it has one.
type Result struct {
	task.Record
	OwnerName *string `json:"owner_name"`
}

// Run assigns the task and delivers its brief. Assignment and delivery are
// stored separately: a failed delivery leaves the task assigned with the
// failure recorded, and is never retried.
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
	if err == nil && o.AgentID != "" && !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(o.AgentID) {
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
		owner, err = identity.Live(s, a.TerminalID)
		if err != nil {
			out.Fail(err, "state", false)
			return out
		}
		if owner == nil || owner.Pane != a.PaneID {
			out.Fail(&herdr.Error{Code: "agent_unregistered", Message: fmt.Sprintf("the agent in %s has no Fledge record; register it first with: fledge agent adopt --pane %s", a.PaneID, a.PaneID)}, "identity", false)
			return out
		}
	}
	r, err := task.Update(s, o.ID, func(r *task.Record) error {
		if !reflect.DeepEqual(*r, snapshot) {
			return &herdr.Error{Code: "task_state_changed", Message: fmt.Sprintf("task %s changed while it was being assigned; inspect it with fledge task get --id %s", r.ID, r.ID)}
		}
		r.Owner, r.Status, r.AssignedAt = &owner.ID, task.Assigned, task.Now()
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
			r.Delivery.Error = &msg
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
	case o.Error.Phase == "agent.prompt":
		_, err = fmt.Fprintf(w, "Task %s remains assigned to %s; the brief was not delivered and will not be retried.\n", r.ID, owner)
	}
	return err
}
