package task

import (
	"context"
	"errors"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
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

// Deliver submits body to pane as message messageID with the caller's sender
// header, then records the outcome on task id under the store lock. attempt
// returns the task's attempt for this message, or an error when the task
// changed so the outcome is no longer this call's to record. Deliver appends
// the submitted-message and updated-task effects and fails out at phase
// agent.prompt for a delivery error, otherwise at phase task for a recording
// error. It returns the updated task and whether the outcome was recorded.
func Deliver(ctx context.Context, c libagent.Client, s *state.Store, out *libagent.Outcome, id, pane, messageID, body string, attempt func(*Record) (*Attempt, error)) (Record, bool) {
	sender := libagent.ResolveSender(ctx, c)
	_, deliveryErr := c.Prompt(ctx, pane, libagent.WithHeader(messageID, sender, body))
	if deliveryErr == nil {
		out.Effects = append(out.Effects, libagent.Effect{Action: "submitted", Kind: "message", ID: pane})
	}
	r, err := Update(s, id, func(r *Record) error {
		a, err := attempt(r)
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
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "task", ID: r.ID})
	}
	switch {
	case deliveryErr != nil:
		out.Fail(deliveryErr, "agent.prompt", true)
	case err != nil:
		out.Fail(err, "task", false)
	}
	return r, err == nil
}
