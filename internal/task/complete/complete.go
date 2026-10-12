// Package complete implements task complete: the owner recording its result.
package complete

import (
	"context"
	"fmt"
	"io"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

type Options struct {
	ID, Summary, File          string
	SummarySet, FileSet, Force bool
}

// Run moves an assigned task to completed with the summary as its result,
// stores the owner's live session ref when the owner is the caller, then
// stores for a distinct registered creator a short inbox notice naming the
// commands to read the result and verify it, and rings its doorbell, starting
// the supervisor when the doorbell does not ring. Only the owner, identified by
// the caller's live record, may complete it unless Force is set.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) cli.Outcome {
	out := cli.NewOutcome("task.complete")
	err := task.ValidateID(o.ID)
	var summary string
	if err == nil {
		summary, err = cli.ReadText(in, cli.TextInput{Body: o.Summary, BodyFlag: "summary", BodySet: o.SummarySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "summary"})
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	var caller *identity.Record
	var live *herdr.AgentDetails
	var sender libagent.Sender
	if err == nil && s != nil {
		caller, live, sender, err = identity.CallerAgentWithSender(ctx, s, c)
	}
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	snapshot, err := task.Get(s, o.ID)
	if err == nil {
		err = authorize(&snapshot, caller, o.Force)
	}
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	var recipient herdr.AgentDetails
	var notification *task.CompletionNotification
	var notificationErr error
	if snapshot.CreatedBy != nil && (caller == nil || *snapshot.CreatedBy != caller.ID) {
		notification = &task.CompletionNotification{Recipient: *snapshot.CreatedBy}
		_, recipient, notificationErr = identity.Resolve(ctx, s, c, *snapshot.CreatedBy)
		if notificationErr != nil {
			msg := notificationErr.Error()
			notification.Error = &msg
		} else {
			notification.Pane = &recipient.PaneID
		}
	}
	r, err := task.Update(s, o.ID, func(r *task.Record) error {
		if err := authorize(r, caller, o.Force); err != nil {
			return err
		}
		r.Status, r.Result, r.CompletedAt, r.CompletionNotification = task.Completed, &summary, task.Now(), notification
		return nil
	})
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	out.Effects = append(out.Effects, cli.Effect{Action: "updated", Kind: "task", ID: r.ID})
	if caller != nil && r.Owner != nil && *r.Owner == caller.ID {
		identity.Observe(s, observeSession, *caller, live, time.Now(), &out)
	}
	out.Result = r
	if notification == nil {
		return out
	}
	if notificationErr != nil {
		out.Fail(notificationErr, "identity", false)
		return out
	}
	body := fmt.Sprintf("task completed: %s · title: %s · read result: fledge task get --id %s · verify with: fledge task verify --id %s --summary \"...\"", r.ID, r.Title, r.ID, r.ID)
	var callerID *string
	if caller != nil {
		callerID = &caller.ID
	}
	msg := inbox.Record{Kind: inbox.Message, From: inbox.FromSender(sender, callerID), To: notification.Recipient, Task: &r.ID, Body: body}
	if r, ok := task.Deliver(ctx, c, s, &out, msg, recipient.PaneID, r.Title, func(r *task.Record, messageID string) (*task.Attempt, error) {
		n := r.CompletionNotification
		if n == nil || n.MessageID != "" || n.Recipient != notification.Recipient {
			return nil, &cli.Error{Code: "task_state_changed", Message: fmt.Sprintf("task %s's completion notification changed before its delivery could be recorded", r.ID)}
		}
		n.MessageID = messageID
		return &n.Attempt, nil
	}); ok {
		out.Result = r
	}
	// A notice stored but not rung waits for the supervisor to ring it.
	if out.Error != nil && out.Error.Phase == "agent.prompt" {
		started, err := supervise.EnsureFor(ctx, c.Cwd, startSupervisor)
		supervise.Note(&out, started, err)
	}
	return out
}

func authorize(r *task.Record, caller *identity.Record, force bool) error {
	if err := task.Require(r, "complete", task.Assigned); err != nil {
		return err
	}
	if !force && (caller == nil || r.Owner == nil || *r.Owner != caller.ID) {
		return &cli.Error{Code: "task_not_owner", Message: fmt.Sprintf("task %s is owned by %s and only its owner may complete it; pass --force to override", r.ID, display(r.Owner))}
	}
	return nil
}

// Render writes completion and notification results.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(task.Record)
	if !ok {
		return nil
	}
	n := r.CompletionNotification
	var text string
	switch {
	case o.Error == nil && n != nil && n.DeliveredAt != nil && n.Pane != nil:
		text = fmt.Sprintf("Completed task %s; stored a notice for creator %s as inbox message %s and rang its doorbell in %s.\n", r.ID, n.Recipient, n.MessageID, *n.Pane)
	case o.Error == nil:
		text = fmt.Sprintf("Completed task %s.\n", r.ID)
	case o.Status == "unknown" && o.Error.Phase == "agent.prompt":
		text = fmt.Sprintf("Task %s is completed; the notice is stored as inbox message %s and the doorbell outcome is unknown; the doorbell may be rung again by the supervisor; the body is never typed.\n", r.ID, n.MessageID)
	case o.Error.Phase == "agent.prompt":
		text = fmt.Sprintf("Task %s is completed; the notice is stored as inbox message %s but its creator's doorbell did not ring; the doorbell may be rung again by the supervisor; the body is never typed.\n", r.ID, n.MessageID)
	case o.Error.Phase == "inbox":
		text = fmt.Sprintf("Task %s is completed; its creator was not notified because the notice could not be stored.\n", r.ID)
	// A creator lookup failure is stored on the notification; its phase names
	// whichever Herdr call failed.
	case n != nil && n.Error != nil:
		text = fmt.Sprintf("Task %s is completed; its creator could not be reached, so no notice was stored.\n", r.ID)
	case o.Error.Phase == "task":
		text = fmt.Sprintf("Task %s is completed; the notification outcome could not be recorded.\n", r.ID)
	}
	_, err := io.WriteString(w, text)
	return err
}

func display(s *string) string {
	if s == nil {
		return "no one"
	}
	return *s
}

// startSupervisor launches the supervision daemon; tests replace it.
var startSupervisor supervise.Starter = supervise.Launch

// observeSession is replaceable so tests can fail the session write.
var observeSession identity.Observer = identity.ObserveSession
