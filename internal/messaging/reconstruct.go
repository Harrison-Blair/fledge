package messaging

import (
	"fmt"
	"sort"
)

func Reconstruct(runID string, events []Event) (*Run, error) {
	run := &Run{
		ID: runID, Active: true, Messages: []*Message{},
		Activations: map[string]*Activation{},
	}
	byID := map[string]*Message{}
	for _, event := range events {
		run.LastSequence = event.Sequence
		switch event.Type {
		case EventRunStarted:
			if event.Sequence != 1 || event.Header == nil {
				return nil, fmt.Errorf("%w: missing run header", ErrCorrupt)
			}
			run.Header = *event.Header
			run.StartedAt = event.Header.StartedAt
		case EventRunClosed:
			t := event.Timestamp
			run.ClosedAt, run.Active = &t, false
		case EventAgentActivated:
			run.Activations[event.ActivationID] = &Activation{
				ID: event.ActivationID, Agent: event.Agent, PaneID: event.PaneID,
				ActivatedAt: event.Timestamp,
			}
		case EventAgentDeactivated:
			if activation := run.Activations[event.ActivationID]; activation != nil {
				t := event.Timestamp
				activation.DeactivatedAt = &t
			}
		case EventMessageCreated, EventMessageReplied:
			if _, exists := byID[event.MessageID]; exists {
				return nil, fmt.Errorf("%w: duplicate message %s", ErrCorrupt, event.MessageID)
			}
			message := &Message{
				ID: event.MessageID, RunID: runID, Sequence: event.Sequence,
				CreatedAt: event.Timestamp, UpdatedAt: event.Timestamp,
				Sender: event.Sender, Recipient: event.Recipient, ReplyTo: event.ReplyTo,
				Body: event.Body, Status: StatusQueued, ActiveRun: true,
				DeliveryAttempts: []DeliveryAttempt{},
			}
			byID[message.ID] = message
			run.Messages = append(run.Messages, message)
			if event.Type == EventMessageReplied {
				acknowledge(byID[event.ReplyTo], event, event.MessageID)
			}
		case EventDeliveryAttempted:
			message := byID[event.MessageID]
			if message == nil {
				return nil, fmt.Errorf("%w: delivery for unknown message", ErrCorrupt)
			}
			message.DeliveryAttempts = append(message.DeliveryAttempts, DeliveryAttempt{
				ID: event.AttemptID, Sequence: event.Sequence, Timestamp: event.Timestamp,
				ActivationID: event.ActivationID, Outcome: "attempted",
			})
			if message.Status != StatusAcknowledged && message.Status != StatusCancelled {
				// The attempt is synced before the external write. If the
				// process ends before recording an outcome, reconstruction
				// cannot prove whether the write occurred.
				message.Status = StatusUncertain
			}
			message.UpdatedAt = event.Timestamp
		case EventDeliveryInjected, EventDeliveryFailed, EventDeliveryUncertain:
			message := byID[event.MessageID]
			if message == nil {
				return nil, fmt.Errorf("%w: delivery outcome for unknown message", ErrCorrupt)
			}
			var attempt *DeliveryAttempt
			for i := range message.DeliveryAttempts {
				if message.DeliveryAttempts[i].ID == event.AttemptID {
					attempt = &message.DeliveryAttempts[i]
					break
				}
			}
			if attempt == nil {
				return nil, fmt.Errorf("%w: outcome for unknown attempt", ErrCorrupt)
			}
			t := event.Timestamp
			attempt.CompletedAt, attempt.Error = &t, event.Error
			switch event.Type {
			case EventDeliveryInjected:
				attempt.Outcome = "injected"
				if message.Status != StatusAcknowledged && message.Status != StatusCancelled {
					message.Status = StatusAwaitingAck
				}
			case EventDeliveryFailed:
				attempt.Outcome = "failed"
				if message.Status != StatusAcknowledged && message.Status != StatusCancelled {
					message.Status = StatusQueued
				}
			case EventDeliveryUncertain:
				attempt.Outcome = "uncertain"
				if message.Status != StatusAcknowledged && message.Status != StatusCancelled {
					message.Status = StatusUncertain
				}
			}
			message.UpdatedAt = event.Timestamp
		case EventMessageAcknowledged:
			message := byID[event.MessageID]
			if message == nil {
				return nil, fmt.Errorf("%w: acknowledgement for unknown message", ErrCorrupt)
			}
			acknowledge(message, event, "")
		case EventMessageCancelled:
			message := byID[event.MessageID]
			if message == nil {
				return nil, fmt.Errorf("%w: cancellation for unknown message", ErrCorrupt)
			}
			message.Status, message.UpdatedAt = StatusCancelled, event.Timestamp
			message.Cancellation = &Cancellation{
				By: event.Actor, Reason: event.Reason, Timestamp: event.Timestamp, Sequence: event.Sequence,
			}
		case EventMessageFailed:
			message := byID[event.MessageID]
			if message == nil {
				return nil, fmt.Errorf("%w: failure for unknown message", ErrCorrupt)
			}
			message.Status, message.UpdatedAt = StatusFailed, event.Timestamp
			message.Failure = &Failure{
				Reason: event.Reason, ActivationID: event.ActivationID,
				Timestamp: event.Timestamp, Sequence: event.Sequence,
			}
		default:
			return nil, fmt.Errorf("%w: unknown event type %q", ErrCorrupt, event.Type)
		}
	}
	if len(events) == 0 || events[0].Type != EventRunStarted {
		return nil, fmt.Errorf("%w: run has no header", ErrCorrupt)
	}
	for _, message := range run.Messages {
		message.ActiveRun = run.Active
	}
	sort.Slice(run.Messages, func(i, j int) bool { return run.Messages[i].Sequence < run.Messages[j].Sequence })
	return run, nil
}

func acknowledge(message *Message, event Event, viaReply string) {
	if message == nil || message.Status == StatusCancelled {
		return
	}
	message.Status, message.UpdatedAt = StatusAcknowledged, event.Timestamp
	message.Acknowledgement = &Acknowledgement{
		By: event.Actor, Timestamp: event.Timestamp, Sequence: event.Sequence, ViaReply: viaReply,
	}
}

func IsUnresolved(status string) bool {
	return status == StatusQueued || status == StatusAwaitingAck ||
		status == StatusFailed || status == StatusUncertain
}
