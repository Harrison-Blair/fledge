// Package complete implements task complete: the owner recording its result.
package complete

import (
	"context"
	"fmt"
	"io"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

type Options struct {
	ID, Summary, File          string
	SummarySet, FileSet, Force bool
}

// Run moves an assigned task to completed with the summary as its result,
// records the worker's usage snapshot, then notifies a distinct registered
// creator. Only the owner, identified by the
// caller's live record, may complete it unless Force is set.
func Run(ctx context.Context, c libagent.Client, o Options, in io.Reader) libagent.Outcome {
	return run(ctx, c, o, in, libagent.NewMessageID())
}

func run(ctx context.Context, c libagent.Client, o Options, in io.Reader, messageID string) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.complete", Status: "success", Effects: []libagent.Effect{}}
	err := task.ValidateID(o.ID)
	var summary string
	if err == nil {
		summary, err = libagent.ReadText(in, libagent.TextInput{Body: o.Summary, BodyFlag: "summary", BodySet: o.SummarySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "summary"})
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	var caller *identity.Record
	var live *herdr.AgentDetails
	if err == nil && s != nil {
		caller, live, err = identity.CallerAgent(ctx, s, c)
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
		notification = &task.CompletionNotification{Recipient: *snapshot.CreatedBy, MessageID: messageID}
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
	out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "task", ID: r.ID})
	out.Result = recordUsage(ctx, c, s, &out, r, caller, live)
	if notification == nil {
		return out
	}
	if notificationErr != nil {
		out.Fail(notificationErr, "identity", false)
		return out
	}
	body := fmt.Sprintf("task completed: %s · title: %s · verify with: fledge task verify --id %s --summary \"...\"\nresult:\n%s", r.ID, r.Title, r.ID, summary)
	if r, ok := task.Deliver(ctx, c, s, &out, o.ID, recipient.PaneID, messageID, body, func(r *task.Record) (*task.Attempt, error) {
		n := r.CompletionNotification
		if n == nil || n.MessageID != messageID || n.Recipient != notification.Recipient {
			return nil, &herdr.Error{Code: "task_state_changed", Message: fmt.Sprintf("task %s's completion notification changed before its delivery could be recorded", r.ID)}
		}
		return &n.Attempt, nil
	}); ok {
		out.Result = r
	}
	return out
}

// recordUsage snapshots the worker's usage over the assignment, after the
// completion is committed and in a separate write, so it never fails the
// completion: a failed write is a warning effect. The worker is the caller
// when it owns the task, else the owner from its persisted record. A worker
// snapshot already stored is kept.
func recordUsage(ctx context.Context, c libagent.Client, s *state.Store, out *libagent.Outcome, r task.Record, caller *identity.Record, live *herdr.AgentDetails) task.Record {
	var notes []string
	from := r.CreatedAt
	if r.AssignedAt != nil {
		from = *r.AssignedAt
	} else {
		notes = append(notes, "task was never assigned; window starts at created_at")
	}
	var worker *identity.Record
	switch {
	case caller != nil && r.Owner != nil && *r.Owner == caller.ID:
		rec := task.Observe(s, observeSession, *caller, live, time.Now(), out)
		worker = &rec
	case r.Owner == nil:
		notes = append(notes, "task has no owner")
	default:
		by := "an unregistered caller"
		if caller != nil {
			by = caller.ID
		}
		notes = append(notes, fmt.Sprintf("completed with --force by %s on the owner's behalf", by))
		var rec identity.Record
		if err := s.Get(identity.Kind, *r.Owner, &rec); err != nil {
			notes = append(notes, "owner record unreadable: "+err.Error())
		} else {
			worker, live = &rec, nil
		}
	}
	snapshot := task.CollectUsage(ctx, readUsage, worker, live, c.Cwd, from, *r.CompletedAt, time.Now(), notes...)
	completedAt := *r.CompletedAt
	stored, err := task.Update(s, r.ID, func(r *task.Record) error {
		if r.CompletedAt == nil || *r.CompletedAt != completedAt {
			return &herdr.Error{Code: "task_state_changed", Message: fmt.Sprintf("task %s changed before its usage could be recorded", r.ID)}
		}
		if r.Usage == nil {
			r.Usage = &task.Usage{}
		}
		if r.Usage.Worker == nil {
			r.Usage.Worker = snapshot
		}
		return nil
	})
	if err != nil {
		out.Effects = append(out.Effects, libagent.Effect{Action: "warning", Kind: "usage", ID: r.ID})
		return r
	}
	return stored
}

func authorize(r *task.Record, caller *identity.Record, force bool) error {
	if err := task.Require(r, "complete", task.Assigned); err != nil {
		return err
	}
	if !force && (caller == nil || r.Owner == nil || *r.Owner != caller.ID) {
		return &herdr.Error{Code: "task_not_owner", Message: fmt.Sprintf("task %s is owned by %s and only its owner may complete it; pass --force to override", r.ID, display(r.Owner))}
	}
	return nil
}

// Render writes completion and notification results.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(task.Record)
	if !ok {
		return nil
	}
	n := r.CompletionNotification
	var text string
	switch {
	case o.Error == nil && n != nil && n.DeliveredAt != nil && n.Pane != nil:
		text = fmt.Sprintf("Completed task %s; notified creator %s in %s as message %s.\n", r.ID, n.Recipient, *n.Pane, n.MessageID)
	case o.Error == nil:
		text = fmt.Sprintf("Completed task %s.\n", r.ID)
	case o.Status == "unknown" && o.Error.Phase == "agent.prompt":
		text = fmt.Sprintf("Task %s is completed; the notification outcome is unknown and will not be retried.\n", r.ID)
	// A creator lookup failure is stored on the notification; its phase names
	// whichever Herdr call failed.
	case o.Error.Phase == "agent.prompt" || n != nil && n.Error != nil:
		text = fmt.Sprintf("Task %s is completed; its creator was not notified and the notification will not be retried.\n", r.ID)
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

// readUsage and observeSession are replaceable so tests can inject a reader
// and fail the session write.
var (
	readUsage      task.Reader   = task.LocalReader
	observeSession task.Observer = identity.ObserveSession
)
