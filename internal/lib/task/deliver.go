package task

import (
	"context"
	"errors"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// Attempt is the recorded outcome of submitting one message. Exactly one of
// DeliveredAt and Error is set once the attempt has finished; Uncertain marks
// an Error after which the message may still have arrived.
type Attempt struct {
	DeliveredAt *string `json:"delivered_at"`
	Error       *string `json:"error"`
	Uncertain   bool    `json:"uncertain"`
}

// State describes the attempt for human output.
func (a Attempt) State() string {
	switch {
	case a.DeliveredAt != nil:
		return "delivered " + *a.DeliveredAt
	case a.Error != nil && a.Uncertain:
		return "outcome unknown: " + *a.Error
	case a.Error != nil:
		return "failed: " + *a.Error
	}
	return "outcome unknown"
}

// Deliver stores msg, which names its task, as an inbox record, rings its doorbell in pane (with
// the task title for an assignment), then records the outcome on task id
// under the store lock. The body is never typed; a doorbell that did not ring
// may be rung again by the supervisor. attempt returns the task's attempt for
// this message after storing messageID, the inbox id, on it, or an error when
// the task changed so the outcome is no longer this call's to record. Deliver
// appends the created-inbox, submitted-message, and updated-task effects and
// fails out at phase inbox when the record cannot be stored, at phase
// agent.prompt for a doorbell error, otherwise at phase task for a recording
// error. It returns the updated task and whether the outcome was recorded.
func Deliver(ctx context.Context, c libagent.Client, s *state.Store, out *cli.Outcome, msg inbox.Record, pane, title string, attempt func(r *Record, messageID string) (*Attempt, error)) (Record, bool) {
	m, deliveryErr := inbox.Create(s, msg)
	phase := "inbox"
	if deliveryErr == nil {
		out.Effects = append(out.Effects, cli.Effect{Action: "created", Kind: "inbox", ID: m.ID})
		phase = "agent.prompt"
		_, deliveryErr = c.Prompt(ctx, pane, inbox.Doorbell(m, title))
		if deliveryErr == nil {
			out.Effects = append(out.Effects, cli.Effect{Action: "submitted", Kind: "message", ID: pane})
		}
		if _, err := inbox.AddRing(s, m.ID, pane, deliveryErr); err != nil {
			out.Effects = append(out.Effects, cli.Effect{Action: "warning", Kind: "inbox", ID: m.ID})
		}
	}
	r, err := Update(s, *msg.Task, func(r *Record) error {
		a, err := attempt(r, m.ID)
		if err != nil {
			return err
		}
		if deliveryErr != nil {
			msg := deliveryErr.Error()
			var remote *herdr.Error
			a.Error, a.Uncertain = &msg, errors.As(deliveryErr, &remote) && remote.Uncertain
		} else {
			a.DeliveredAt = Now()
		}
		return nil
	})
	if err == nil {
		out.Effects = append(out.Effects, cli.Effect{Action: "updated", Kind: "task", ID: r.ID})
	}
	switch {
	case deliveryErr != nil:
		out.Fail(deliveryErr, phase, phase == "agent.prompt")
	case err != nil:
		out.Fail(err, "task", false)
	}
	return r, err == nil
}
