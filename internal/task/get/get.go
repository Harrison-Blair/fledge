// Package get implements task get: one task's full record.
package get

import (
	"context"
	"fmt"
	"io"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

type Options struct{ ID string }

// Run reads one task from the store without contacting Herdr.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.get", Status: "success", Effects: []libagent.Effect{}}
	if err := task.ValidateID(o.ID); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	r, err := task.Get(s, o.ID)
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	out.Result = r
	return out
}

// Render writes the task as labelled lines, omitting steps not yet reached,
// followed by its indented texts.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(task.Record)
	if o.Error != nil || !ok {
		return nil
	}
	var b strings.Builder
	owner := "-"
	if r.Owner != nil {
		owner = *r.Owner
	}
	creator := "an unregistered caller"
	if r.CreatedBy != nil {
		creator = *r.CreatedBy
	}
	fmt.Fprintf(&b, "id: %s\ntitle: %s\nstatus: %s\nowner: %s\ncreated: %s by %s\n", r.ID, r.Title, r.Status, owner, r.CreatedAt, creator)
	if r.AssignedAt != nil {
		fmt.Fprintf(&b, "assigned: %s\n", *r.AssignedAt)
	}
	if d := r.Delivery; d != nil {
		state := "outcome unknown"
		switch {
		case d.DeliveredAt != nil:
			state = "delivered " + *d.DeliveredAt
		case d.Error != nil && d.Uncertain:
			state = "outcome unknown: " + *d.Error
		case d.Error != nil:
			state = "failed: " + *d.Error
		}
		fmt.Fprintf(&b, "delivery: message %s to %s, %s\n", d.MessageID, d.Pane, state)
	}
	if r.CompletedAt != nil {
		fmt.Fprintf(&b, "completed: %s\n", *r.CompletedAt)
	}
	if n := r.CompletionNotification; n != nil {
		state := "outcome unknown"
		switch {
		case n.DeliveredAt != nil:
			state = "delivered " + *n.DeliveredAt
		case n.Error != nil && n.Uncertain:
			state = "outcome unknown: " + *n.Error
		case n.Error != nil:
			state = "failed: " + *n.Error
		}
		target := n.Recipient
		if n.Pane != nil {
			target += " in " + *n.Pane
		}
		fmt.Fprintf(&b, "completion notification: message %s to %s, %s\n", n.MessageID, target, state)
	}
	if r.VerifiedAt != nil {
		verifier, forced := "an unregistered caller", ""
		if r.Verifier != nil {
			verifier = *r.Verifier
		}
		if r.Forced {
			forced = " (forced)"
		}
		fmt.Fprintf(&b, "verified: %s by %s%s\n", *r.VerifiedAt, verifier, forced)
	}
	if r.CancelledAt != nil {
		reason := ""
		if r.CancelReason != nil {
			reason = " (" + *r.CancelReason + ")"
		}
		fmt.Fprintf(&b, "cancelled: %s%s\n", *r.CancelledAt, reason)
	}
	text(&b, "brief", &r.Brief)
	text(&b, "result", r.Result)
	text(&b, "verification note", r.VerificationNote)
	_, err := io.WriteString(w, b.String())
	return err
}

func text(b *strings.Builder, label string, s *string) {
	if s == nil || *s == "" {
		return
	}
	fmt.Fprintf(b, "%s:\n", label)
	for _, line := range strings.Split(strings.TrimRight(*s, "\n"), "\n") {
		fmt.Fprintf(b, "  %s\n", line)
	}
}
