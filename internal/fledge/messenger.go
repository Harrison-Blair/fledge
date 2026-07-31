package fledge

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

// messenger owns durable messaging: the append-only run log, the mailbox
// operations recorded against it, and delivery into live agent panes. It
// reaches the Herdr session only through connect, so the session lifecycle
// stays with the Service that holds it.
type messenger struct {
	project      project.Info
	store        *state.Store
	log          *messaging.Store
	callerPaneID string
	connect      func(context.Context) (*herdr.Client, error)
}

func (m *messenger) send(ctx context.Context, recipient, body string) (MessageResult, error) {
	if err := ValidateMessageBody(body); err != nil {
		return MessageResult{}, err
	}
	active, err := m.activeContext(ctx)
	if err != nil {
		return MessageResult{}, err
	}
	if recipient != userMailbox {
		if err := validateMailboxName(recipient); err != nil {
			return MessageResult{}, err
		}
		if _, ok := active.state.Agents[recipient]; !ok {
			return MessageResult{}, NewError("message_wrong_recipient",
				fmt.Sprintf("recipient %q is not known in the active run", recipient))
		}
	}
	if recipient == active.actor {
		return MessageResult{}, NewError("message_wrong_recipient", "cannot send a message to yourself")
	}
	messageID, err := messaging.NewID("msg_")
	if err != nil {
		return MessageResult{}, Wrap("message_log_unavailable", "generate message ID", err)
	}
	if _, err := m.appendActiveRunEvent(active.runID, messaging.Event{
		Type: messaging.EventMessageCreated, MessageID: messageID,
		Sender: active.actor, Recipient: recipient, Body: body,
	}); err != nil {
		return MessageResult{}, err
	}
	return m.deliverIfLive(ctx, active, messageID, recipient)
}

func (m *messenger) reply(ctx context.Context, messageID, body string) (MessageResult, error) {
	if err := ValidateMessageBody(body); err != nil {
		return MessageResult{}, err
	}
	target, active, err := m.resolveActive(ctx, messageID, "replied to")
	if err != nil {
		return MessageResult{}, err
	}
	if target.Recipient != active.actor {
		return MessageResult{}, NewError("message_wrong_recipient", "only the message recipient can reply")
	}
	if target.Status == messaging.StatusCancelled {
		return MessageResult{}, NewError("message_state_conflict", "cancelled messages cannot be replied to")
	}
	replyID, err := messaging.NewID("msg_")
	if err != nil {
		return MessageResult{}, Wrap("message_log_unavailable", "generate reply ID", err)
	}
	if _, err := m.appendActiveRunEvent(active.runID, messaging.Event{
		Type: messaging.EventMessageReplied, MessageID: replyID,
		Sender: active.actor, Recipient: target.Sender, ReplyTo: target.ID,
		Body: body, Actor: active.actor,
	}); err != nil {
		return MessageResult{}, err
	}
	return m.deliverIfLive(ctx, active, replyID, target.Sender)
}

func (m *messenger) ack(ctx context.Context, messageID string) (MessageResult, error) {
	message, active, err := m.resolveActive(ctx, messageID, "acknowledged")
	if err != nil {
		return MessageResult{}, err
	}
	if message.Recipient != active.actor {
		return MessageResult{}, NewError("message_wrong_recipient", "only the message recipient can acknowledge it")
	}
	if message.Status == messaging.StatusAcknowledged {
		return MessageResult{Message: message}, nil
	}
	if message.Status == messaging.StatusCancelled {
		return MessageResult{}, NewError("message_state_conflict", "cancelled messages cannot be acknowledged")
	}
	if _, err := m.appendActiveRunEvent(active.runID, messaging.Event{
		Type: messaging.EventMessageAcknowledged, MessageID: message.ID, Actor: active.actor,
	}); err != nil {
		return MessageResult{}, err
	}
	current, err := m.messageInRun(active.runID, message.ID)
	return MessageResult{Message: current}, err
}

func (m *messenger) retry(ctx context.Context, messageID string, force bool) (MessageResult, error) {
	message, active, err := m.resolveActive(ctx, messageID, "retried")
	if err != nil {
		return MessageResult{}, err
	}
	if err := authorizeSenderOrUser(active, message, "retry"); err != nil {
		return MessageResult{}, err
	}
	switch message.Status {
	case messaging.StatusQueued, messaging.StatusFailed:
	case messaging.StatusAwaitingAck:
		if !force {
			return MessageResult{}, NewError("message_state_conflict",
				"message is awaiting acknowledgement; pass --force to inject it again")
		}
	default:
		return MessageResult{}, NewError("message_state_conflict",
			fmt.Sprintf("message in state %s cannot be retried", message.Status))
	}
	return m.deliverIfLive(ctx, active, message.ID, message.Recipient)
}

func (m *messenger) cancel(ctx context.Context, messageID, reason string) (MessageResult, error) {
	message, active, err := m.resolveActive(ctx, messageID, "cancelled")
	if err != nil {
		return MessageResult{}, err
	}
	if err := authorizeSenderOrUser(active, message, "cancel"); err != nil {
		return MessageResult{}, err
	}
	if message.Status == messaging.StatusAcknowledged || message.Status == messaging.StatusCancelled {
		return MessageResult{}, NewError("message_state_conflict",
			fmt.Sprintf("message in state %s cannot be cancelled", message.Status))
	}
	if _, err := m.appendActiveRunEvent(active.runID, messaging.Event{
		Type: messaging.EventMessageCancelled, MessageID: message.ID,
		Actor: active.actor, Reason: reason,
	}); err != nil {
		return MessageResult{}, err
	}
	current, err := m.messageInRun(active.runID, message.ID)
	return MessageResult{Message: current}, err
}

func (m *messenger) show(messageID string) (*messaging.Message, error) {
	message, _, err := m.log.FindMessage(messageID)
	if err != nil {
		return nil, messageLookupError(messageID, err)
	}
	return message, nil
}

func (m *messenger) runs(limit int) (messaging.RunCollection, error) {
	runs, err := m.log.ReadRuns()
	if err != nil {
		return messaging.RunCollection{}, messageStoreError(err)
	}
	total := len(runs)
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return messaging.RunCollection{Total: total, Returned: len(runs), Runs: runs}, nil
}

func (m *messenger) history(opts MessageHistoryOptions) (messaging.Collection, error) {
	runs, err := m.log.ReadRuns()
	if err != nil {
		return messaging.Collection{}, messageStoreError(err)
	}
	selected := selectRuns(runs, opts)
	if len(opts.RunIDs) > 0 {
		found := map[string]bool{}
		for _, run := range selected {
			found[run.ID] = true
		}
		for _, runID := range opts.RunIDs {
			if !found[runID] {
				return messaging.Collection{}, NewError("message_not_found",
					fmt.Sprintf("message run %q was not found", runID))
			}
		}
	}
	out := messaging.Collection{SelectedRuns: []string{}, Messages: []*messaging.Message{}}
	for _, run := range selected {
		out.SelectedRuns = append(out.SelectedRuns, run.ID)
		for _, message := range run.Messages {
			if opts.Agent != "" && message.Sender != opts.Agent && message.Recipient != opts.Agent {
				continue
			}
			if opts.With != "" && !messageBetween(message, opts.Agent, opts.With) {
				continue
			}
			if opts.Status != "" && message.Status != opts.Status {
				continue
			}
			out.Messages = append(out.Messages, message)
		}
	}
	sort.Slice(out.Messages, func(i, j int) bool {
		if out.Messages[i].CreatedAt.Equal(out.Messages[j].CreatedAt) {
			return out.Messages[i].Sequence < out.Messages[j].Sequence
		}
		return out.Messages[i].CreatedAt.Before(out.Messages[j].CreatedAt)
	})
	out.Total = len(out.Messages)
	if opts.Limit > 0 && len(out.Messages) > opts.Limit {
		out.Messages = out.Messages[len(out.Messages)-opts.Limit:]
	}
	out.Returned = len(out.Messages)
	return out, nil
}

func (m *messenger) inbox(ctx context.Context, identity string, limit int) (messaging.Collection, error) {
	active, err := m.activeContext(ctx)
	if err != nil {
		return messaging.Collection{}, err
	}
	if identity == "" {
		identity = active.actor
	} else if identity != active.actor && active.actor != userMailbox {
		return messaging.Collection{}, NewError("message_forbidden", "only user can inspect another mailbox")
	}
	if identity != userMailbox {
		if err := validateMailboxName(identity); err != nil {
			return messaging.Collection{}, err
		}
		if _, known := active.state.Agents[identity]; !known {
			return messaging.Collection{}, NewError("message_wrong_recipient",
				fmt.Sprintf("mailbox %q is not known in the active run", identity))
		}
	}
	run, err := m.log.ReadRun(active.runID)
	if err != nil {
		return messaging.Collection{}, messageStoreError(err)
	}
	out := messaging.Collection{SelectedRuns: []string{run.ID}, Messages: []*messaging.Message{}}
	for _, message := range run.Messages {
		if message.Recipient == identity && messaging.IsUnresolved(message.Status) {
			out.Messages = append(out.Messages, message)
		}
	}
	out.Total = len(out.Messages)
	if limit > 0 && len(out.Messages) > limit {
		out.Messages = out.Messages[len(out.Messages)-limit:]
	}
	out.Returned = len(out.Messages)
	return out, nil
}

type activeMessagingContext struct {
	runID    string
	actor    string
	state    state.Session
	snapshot herdr.Snapshot
	client   *herdr.Client
}

// resolveActive looks up a message and the messaging context it belongs to,
// rejecting messages that are archived or outside the active run.
// archivedVerb completes "archived messages cannot be %s".
func (m *messenger) resolveActive(
	ctx context.Context, messageID, archivedVerb string,
) (*messaging.Message, activeMessagingContext, error) {
	message, run, err := m.log.FindMessage(messageID)
	if err != nil {
		return nil, activeMessagingContext{}, messageLookupError(messageID, err)
	}
	if !run.Active {
		return nil, activeMessagingContext{}, NewError("message_state_conflict",
			fmt.Sprintf("archived messages cannot be %s", archivedVerb))
	}
	active, err := m.activeContext(ctx)
	if err != nil {
		return nil, activeMessagingContext{}, err
	}
	if run.ID != active.runID {
		return nil, activeMessagingContext{}, NewError("message_state_conflict",
			"message is not part of the active run")
	}
	return message, active, nil
}

// authorizeSenderOrUser allows only the message sender or the project owner.
// action completes "only the sender or user can %s this message".
func authorizeSenderOrUser(active activeMessagingContext, message *messaging.Message, action string) error {
	if active.actor != userMailbox && active.actor != message.Sender {
		return NewError("message_forbidden",
			fmt.Sprintf("only the sender or user can %s this message", action))
	}
	return nil
}

func (m *messenger) appendActiveRunEvent(runID string, event messaging.Event) (messaging.Event, error) {
	var appended messaging.Event
	err := m.log.WithLifecycleLock(runID, func() error {
		run, err := m.log.ReadRun(runID)
		if err != nil {
			return messageStoreError(err)
		}
		if !run.Active {
			return NewError("message_state_conflict", "the message run is closed")
		}
		appended, err = m.log.Append(runID, event)
		if err != nil {
			return messageStoreError(err)
		}
		return nil
	})
	if err != nil {
		return messaging.Event{}, asServiceOrStoreError(err)
	}
	return appended, nil
}

func (m *messenger) activeContext(ctx context.Context) (activeMessagingContext, error) {
	client, err := m.connect(ctx)
	if err != nil {
		return activeMessagingContext{}, err
	}
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return activeMessagingContext{}, err
	}
	st, err := m.store.Read(m.project.Session, m.project.Root)
	if err != nil {
		return activeMessagingContext{}, Wrap("state_unavailable", err.Error(), err)
	}
	if st.ActiveRunID == "" {
		return activeMessagingContext{}, NewError("message_run_unavailable",
			"this session predates durable messaging; run `fledge stop` followed by `fledge start`")
	}
	run, err := m.log.ReadRun(st.ActiveRunID)
	if err != nil {
		return activeMessagingContext{}, messageStoreError(err)
	}
	if !run.Active {
		return activeMessagingContext{}, NewError("message_run_unavailable", "the active message run is already closed")
	}
	return activeMessagingContext{
		runID: st.ActiveRunID, actor: inferActor(m.callerPaneID, st, snapshot),
		state: st, snapshot: snapshot, client: client,
	}, nil
}

func inferActor(paneID string, st state.Session, snapshot herdr.Snapshot) string {
	if paneID == "" {
		return userMailbox
	}
	live := agentsByPane(snapshot)
	for name, managed := range st.Agents {
		agent, ok := live[paneID]
		if managed.PaneID == paneID && ok && agent.Agent != nil && agent.PaneID == paneID {
			return name
		}
	}
	return userMailbox
}

// deliveryTarget identifies the agent activation a message is delivered to.
type deliveryTarget struct {
	runID        string
	agent        string
	paneID       string
	activationID string
}

func (m *messenger) deliverIfLive(
	ctx context.Context,
	active activeMessagingContext,
	messageID, recipient string,
) (MessageResult, error) {
	managed, known := active.state.Agents[recipient]
	live := agentsByPane(active.snapshot)
	agent, running := live[managed.PaneID]
	if recipient == userMailbox || !known || !running || agent.Agent == nil {
		message, err := m.messageInRun(active.runID, messageID)
		return MessageResult{Message: message}, err
	}
	target := deliveryTarget{
		runID: active.runID, agent: recipient,
		paneID: managed.PaneID, activationID: managed.ActivationID,
	}
	message, deliveryErr, err := m.deliver(ctx, target, messageID, active.client)
	if err != nil {
		return MessageResult{}, err
	}
	return MessageResult{Message: message, DeliveryError: deliveryErr}, nil
}

func (m *messenger) deliver(
	ctx context.Context,
	target deliveryTarget,
	messageID string,
	client *herdr.Client,
) (*messaging.Message, string, error) {
	var message *messaging.Message
	var deliveryError string
	err := m.log.WithLifecycleLock(target.runID, func() error {
		run, err := m.log.ReadRun(target.runID)
		if err != nil {
			return messageStoreError(err)
		}
		for _, candidate := range run.Messages {
			if candidate.ID == messageID {
				message = candidate
				break
			}
		}
		if message == nil {
			return NewError("message_not_found", fmt.Sprintf("message %q was not found", messageID))
		}
		if !run.Active {
			deliveryError = "delivery was not attempted because the message run is closed"
			return nil
		}
		var innerErr error
		message, deliveryError, innerErr = m.deliverLocked(ctx, target, message, client)
		return innerErr
	})
	if err != nil {
		return nil, "", asServiceOrStoreError(err)
	}
	return message, deliveryError, nil
}

func (m *messenger) deliverLocked(
	ctx context.Context,
	target deliveryTarget,
	message *messaging.Message,
	client *herdr.Client,
) (*messaging.Message, string, error) {
	runID := target.runID
	messageID := message.ID
	attemptID, err := messaging.NewID("try_")
	if err != nil {
		return message, fmt.Sprintf("delivery was not attempted: generate attempt ID: %v", err), nil
	}
	if _, err := m.log.Append(runID, messaging.Event{
		Type: messaging.EventDeliveryAttempted, MessageID: messageID,
		AttemptID: attemptID, Agent: target.agent, ActivationID: target.activationID, PaneID: target.paneID,
	}); err != nil {
		return message, fmt.Sprintf("delivery was not attempted: %v", err), nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	callErr := client.Call(callCtx, "agent.prompt", map[string]any{
		"target": target.paneID, "text": messageEnvelope(message),
	}, nil)
	outcome := messaging.EventDeliveryInjected
	errorKind, errorText := "", ""
	if callErr != nil {
		errorText = callErr.Error()
		if deliveryIsUncertain(callErr) {
			outcome, errorKind = messaging.EventDeliveryUncertain, "ambiguous_transport"
		} else {
			outcome, errorKind = messaging.EventDeliveryFailed, "definite_non_injection"
		}
	}
	if _, err := m.log.Append(runID, messaging.Event{
		Type: outcome, MessageID: messageID, AttemptID: attemptID,
		Agent: target.agent, ActivationID: target.activationID, PaneID: target.paneID,
		Error: errorText, ErrorKind: errorKind,
	}); err != nil {
		current, readErr := m.messageInRun(runID, messageID)
		if readErr == nil {
			return current, fmt.Sprintf("delivery outcome could not be committed: %v", err), nil
		}
		copy := *message
		copy.Status = messaging.StatusUncertain
		return &copy, fmt.Sprintf("delivery outcome could not be committed: %v", err), nil
	}
	current, err := m.messageInRun(runID, messageID)
	if err != nil {
		return message, fmt.Sprintf("delivery succeeded but its result could not be re-read: %v", err), nil
	}
	return current, errorText, nil
}

func (m *messenger) messageInRun(runID, messageID string) (*messaging.Message, error) {
	run, err := m.log.ReadRun(runID)
	if err != nil {
		return nil, messageStoreError(err)
	}
	for _, message := range run.Messages {
		if message.ID == messageID {
			return message, nil
		}
	}
	return nil, NewError("message_not_found", fmt.Sprintf("message %q was not found", messageID))
}

func deliveryIsUncertain(err error) bool {
	var transport *herdr.TransportError
	return errors.As(err, &transport) && transport.Operation != "connect"
}

func messageEnvelope(message *messaging.Message) string {
	return fmt.Sprintf(
		"[Fledge message]\nID: %s\nFrom: %s\nTo: %s\nTimestamp: %s\n\n%s\n\n"+
			"Acknowledge exactly with:\nfledge agent message ack %s\n"+
			"Reply exactly with:\nfledge agent message reply %s <text>",
		message.ID, message.Sender, message.Recipient, message.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		message.Body, message.ID, message.ID,
	)
}

// beginRun opens the durable run this server lifecycle records against and
// publishes it as the session's active run.
func (m *messenger) beginRun(header messaging.RunHeader) (string, error) {
	if err := project.EnsureLogsIgnored(m.project.Root); err != nil {
		return "", Wrap("message_log_unavailable", err.Error(), err)
	}
	runID, err := m.log.StartRun(header)
	if err != nil {
		return "", messageStoreError(err)
	}
	if err := m.store.WithLocked(m.project.Session, m.project.Root, func(st *state.Session) error {
		st.ActiveRunID = runID
		return nil
	}); err != nil {
		_ = m.closeRun(runID, "state_persist_failed")
		return "", Wrap("state_persist_failed", fmt.Sprintf("persist active message run: %v", err), err)
	}
	return runID, nil
}

func (m *messenger) activateAgent(ctx context.Context, client *herdr.Client, name, paneID string) error {
	target, err := m.prepareActivation(name, paneID)
	if err != nil || target.runID == "" {
		return err
	}
	return m.drainAgentMessages(ctx, client, target)
}

// prepareActivation returns the zero deliveryTarget when the session has no
// active message run.
func (m *messenger) prepareActivation(name, paneID string) (deliveryTarget, error) {
	if st, found, err := m.store.ReadExisting(m.project.Session, m.project.Root); err != nil {
		return deliveryTarget{}, err
	} else if found {
		if managed, ok := st.Agents[name]; ok && managed.ActivationID != "" {
			if err := m.deactivateAgent(name, "activation superseded"); err != nil {
				return deliveryTarget{}, err
			}
		}
	}
	var target deliveryTarget
	err := m.store.WithLocked(m.project.Session, m.project.Root, func(st *state.Session) error {
		target.runID = st.ActiveRunID
		if target.runID == "" {
			return nil
		}
		var err error
		target.activationID, err = messaging.NewID("act_")
		if err != nil {
			return err
		}
		managed := st.Agents[name]
		managed.ActivationID = target.activationID
		st.Agents[name] = managed
		return nil
	})
	if err != nil {
		return deliveryTarget{}, err
	}
	if target.runID == "" {
		return deliveryTarget{}, nil
	}
	target.agent, target.paneID = name, paneID
	if _, err := m.appendActiveRunEvent(target.runID, messaging.Event{
		Type: messaging.EventAgentActivated, Agent: target.agent,
		ActivationID: target.activationID, PaneID: target.paneID,
	}); err != nil {
		_ = m.store.WithLocked(m.project.Session, m.project.Root, func(st *state.Session) error {
			managed := st.Agents[name]
			if managed.ActivationID == target.activationID {
				managed.ActivationID = ""
				st.Agents[name] = managed
			}
			return nil
		})
		return deliveryTarget{}, err
	}
	return target, nil
}

// deliverActivation is used by the bounded hidden helper launched immediately
// before an in-pane exec. It exits quietly if the mapping is superseded.
func (m *messenger) deliverActivation(ctx context.Context, name, activationID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		client, err := m.connect(ctx)
		if err == nil {
			st, readErr := m.store.Read(m.project.Session, m.project.Root)
			if readErr != nil {
				return readErr
			}
			managed, exists := st.Agents[name]
			if !exists || managed.ActivationID != activationID || st.ActiveRunID == "" {
				return nil
			}
			snapshot, snapshotErr := client.Snapshot(ctx)
			if snapshotErr == nil {
				if live, ok := agentsByPane(snapshot)[managed.PaneID]; ok && live.Agent != nil &&
					live.InteractiveReady {
					return m.drainAgentMessages(ctx, client, deliveryTarget{
						runID: st.ActiveRunID, agent: name,
						paneID: managed.PaneID, activationID: activationID,
					})
				}
			}
		}
		if !time.Now().Before(deadline) {
			return nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (m *messenger) drainAgentMessages(
	ctx context.Context, client *herdr.Client, target deliveryTarget,
) error {
	run, err := m.log.ReadRun(target.runID)
	if err != nil {
		return messageStoreError(err)
	}
	for _, message := range run.Messages {
		if message.Recipient != target.agent ||
			(message.Status != messaging.StatusQueued && message.Status != messaging.StatusFailed) ||
			attemptedInActivation(message, target.activationID) {
			continue
		}
		_, deliveryErr, err := m.deliver(ctx, target, message.ID, client)
		if err != nil {
			return err
		}
		if deliveryErr != "" {
			break
		}
	}
	return nil
}

func attemptedInActivation(message *messaging.Message, activationID string) bool {
	for _, attempt := range message.DeliveryAttempts {
		if attempt.ActivationID == activationID {
			return true
		}
	}
	return false
}

func (m *messenger) deactivateAgent(name, reason string) error {
	var runID, activationID, paneID string
	err := m.store.WithLocked(m.project.Session, m.project.Root, func(st *state.Session) error {
		runID = st.ActiveRunID
		managed := st.Agents[name]
		activationID, paneID = managed.ActivationID, managed.PaneID
		managed.ActivationID = ""
		st.Agents[name] = managed
		return nil
	})
	if err != nil || runID == "" || activationID == "" {
		return err
	}
	err = m.log.WithLifecycleLock(runID, func() error {
		run, err := m.log.ReadRun(runID)
		if err != nil {
			return messageStoreError(err)
		}
		if !run.Active {
			return nil
		}
		if _, err := m.log.Append(runID, messaging.Event{
			Type: messaging.EventAgentDeactivated, Agent: name,
			ActivationID: activationID, PaneID: paneID, Reason: reason,
		}); err != nil {
			return messageStoreError(err)
		}
		run, err = m.log.ReadRun(runID)
		if err != nil {
			return messageStoreError(err)
		}
		for _, message := range run.Messages {
			if message.Recipient == name && message.Status == messaging.StatusAwaitingAck &&
				attemptedInActivation(message, activationID) {
				if _, err := m.log.Append(runID, messaging.Event{
					Type: messaging.EventMessageFailed, MessageID: message.ID,
					ActivationID: activationID, Reason: "recipient activation ended without acknowledgement",
				}); err != nil {
					return messageStoreError(err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return asServiceOrStoreError(err)
	}
	return nil
}

func (m *messenger) closeRun(runID, reason string) error {
	if runID == "" {
		return nil
	}
	err := m.log.WithLifecycleLock(runID, func() error {
		run, err := m.log.ReadRun(runID)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return messageStoreError(err)
		}
		if !run.Active {
			return nil
		}
		for _, message := range run.Messages {
			if messaging.IsUnresolved(message.Status) {
				if _, err := m.log.Append(runID, messaging.Event{
					Type: messaging.EventMessageFailed, MessageID: message.ID, Reason: reason,
				}); err != nil {
					return messageStoreError(err)
				}
			}
		}
		_, err = m.log.Append(runID, messaging.Event{Type: messaging.EventRunClosed, Reason: reason})
		return messageStoreErrorIf(err)
	})
	if err == nil {
		return nil
	}
	return asServiceOrStoreError(err)
}

func (m *messenger) closeActiveRun(reason string) error {
	st, found, err := m.store.ReadExisting(m.project.Session, m.project.Root)
	if err != nil || !found {
		return err
	}
	names := make([]string, 0)
	for name, managed := range st.Agents {
		if managed.ActivationID != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := m.deactivateAgent(name, reason); err != nil {
			return err
		}
	}
	return m.closeRun(st.ActiveRunID, reason)
}

func (m *messenger) pendingCounts(runID string) (map[string]int, error) {
	counts := map[string]int{}
	if runID == "" {
		return counts, nil
	}
	run, err := m.log.ReadRun(runID)
	if err != nil {
		return nil, messageStoreError(err)
	}
	for _, message := range run.Messages {
		if messaging.IsUnresolved(message.Status) {
			counts[message.Recipient]++
		}
	}
	return counts, nil
}

func selectRuns(runs []*messaging.Run, opts MessageHistoryOptions) []*messaging.Run {
	if opts.AllRuns {
		return runs
	}
	if len(opts.RunIDs) > 0 {
		wanted := map[string]bool{}
		for _, id := range opts.RunIDs {
			wanted[id] = true
		}
		selected := make([]*messaging.Run, 0, len(opts.RunIDs))
		for _, run := range runs {
			if wanted[run.ID] {
				selected = append(selected, run)
			}
		}
		return selected
	}
	if len(runs) == 0 {
		return nil
	}
	for _, run := range runs {
		if run.Active {
			return []*messaging.Run{run}
		}
	}
	return []*messaging.Run{runs[0]}
}

func messageBetween(message *messaging.Message, first, second string) bool {
	if first == "" {
		return message.Sender == second || message.Recipient == second
	}
	return (message.Sender == first && message.Recipient == second) ||
		(message.Sender == second && message.Recipient == first)
}

func validateMailboxName(name string) error {
	if name == userMailbox {
		return nil
	}
	if err := ValidateAgentName(name); err != nil {
		return NewError("message_wrong_recipient", err.Error())
	}
	return nil
}

func messageStoreError(err error) error {
	if errors.Is(err, messaging.ErrCorrupt) {
		return Wrap("message_log_corrupt", err.Error(), err)
	}
	return Wrap("message_log_unavailable", err.Error(), err)
}

// asServiceOrStoreError passes a service error through unchanged and wraps
// anything else as a message store failure.
func asServiceOrStoreError(err error) error {
	var serviceErr *Error
	if errors.As(err, &serviceErr) {
		return err
	}
	return messageStoreError(err)
}

func messageStoreErrorIf(err error) error {
	if err == nil {
		return nil
	}
	return messageStoreError(err)
}

func messageLookupError(id string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return NewError("message_not_found", fmt.Sprintf("message %q was not found", id))
	}
	return messageStoreError(err)
}
