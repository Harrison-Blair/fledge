package fledge

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/state"
)

const MaxMessageBodyBytes = 256 << 10

type MessageResult struct {
	Message       *messaging.Message `json:"message"`
	DeliveryError string             `json:"delivery_error,omitempty"`
}

type MessageHistoryOptions struct {
	Agent   string
	With    string
	RunIDs  []string
	AllRuns bool
	Status  string
	Limit   int
}

func ValidateAgentName(name string) error {
	if name == "user" {
		return NewError("invalid_agent_name", `"user" is reserved for the project owner mailbox`)
	}
	if len(name) == 0 || len(name) > 64 {
		return NewError("invalid_agent_name", "agent names must contain 1 to 64 characters")
	}
	for i, r := range name {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			(i > 0 && (r == '_' || r == '-')) {
			continue
		}
		return NewError("invalid_agent_name", fmt.Sprintf("invalid agent name %q", name))
	}
	return nil
}

func ValidateMessageBody(body string) error {
	if !utf8.ValidString(body) {
		return NewError("invalid_message", "message body must be valid UTF-8")
	}
	if len(body) > MaxMessageBodyBytes {
		return NewError("invalid_message", fmt.Sprintf("message body exceeds %d bytes", MaxMessageBodyBytes))
	}
	if strings.TrimSpace(body) == "" {
		return NewError("invalid_message", "message body must not be blank")
	}
	return nil
}

func (s *Service) SendMessage(ctx context.Context, recipient, body string) (MessageResult, error) {
	if err := ValidateMessageBody(body); err != nil {
		return MessageResult{}, err
	}
	active, err := s.activeMessaging(ctx)
	if err != nil {
		return MessageResult{}, err
	}
	if recipient != "user" {
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
	if _, err := s.appendActiveRunEvent(active.runID, messaging.Event{
		Type: messaging.EventMessageCreated, MessageID: messageID,
		Sender: active.actor, Recipient: recipient, Body: body,
	}); err != nil {
		return MessageResult{}, err
	}
	return s.deliverIfLive(ctx, active, messageID, recipient)
}

func (s *Service) ReplyMessage(ctx context.Context, messageID, body string) (MessageResult, error) {
	if err := ValidateMessageBody(body); err != nil {
		return MessageResult{}, err
	}
	target, run, err := s.messageStore().FindMessage(messageID)
	if err != nil {
		return MessageResult{}, messageLookupError(messageID, err)
	}
	if !run.Active {
		return MessageResult{}, NewError("message_state_conflict", "archived messages cannot be replied to")
	}
	active, err := s.activeMessaging(ctx)
	if err != nil {
		return MessageResult{}, err
	}
	if run.ID != active.runID {
		return MessageResult{}, NewError("message_state_conflict", "message is not part of the active run")
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
	if _, err := s.appendActiveRunEvent(active.runID, messaging.Event{
		Type: messaging.EventMessageReplied, MessageID: replyID,
		Sender: active.actor, Recipient: target.Sender, ReplyTo: target.ID,
		Body: body, Actor: active.actor,
	}); err != nil {
		return MessageResult{}, err
	}
	return s.deliverIfLive(ctx, active, replyID, target.Sender)
}

func (s *Service) AckMessage(ctx context.Context, messageID string) (MessageResult, error) {
	message, run, err := s.messageStore().FindMessage(messageID)
	if err != nil {
		return MessageResult{}, messageLookupError(messageID, err)
	}
	if !run.Active {
		return MessageResult{}, NewError("message_state_conflict", "archived messages cannot be acknowledged")
	}
	active, err := s.activeMessaging(ctx)
	if err != nil {
		return MessageResult{}, err
	}
	if run.ID != active.runID {
		return MessageResult{}, NewError("message_state_conflict", "message is not part of the active run")
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
	if _, err := s.appendActiveRunEvent(active.runID, messaging.Event{
		Type: messaging.EventMessageAcknowledged, MessageID: message.ID, Actor: active.actor,
	}); err != nil {
		return MessageResult{}, err
	}
	current, err := s.messageInRun(active.runID, message.ID)
	return MessageResult{Message: current}, err
}

func (s *Service) RetryMessage(ctx context.Context, messageID string, force bool) (MessageResult, error) {
	message, run, err := s.messageStore().FindMessage(messageID)
	if err != nil {
		return MessageResult{}, messageLookupError(messageID, err)
	}
	if !run.Active {
		return MessageResult{}, NewError("message_state_conflict", "archived messages cannot be retried")
	}
	active, err := s.activeMessaging(ctx)
	if err != nil {
		return MessageResult{}, err
	}
	if run.ID != active.runID {
		return MessageResult{}, NewError("message_state_conflict", "message is not part of the active run")
	}
	if active.actor != "user" && active.actor != message.Sender {
		return MessageResult{}, NewError("message_forbidden", "only the sender or user can retry this message")
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
	return s.deliverIfLive(ctx, active, message.ID, message.Recipient)
}

func (s *Service) CancelMessage(ctx context.Context, messageID, reason string) (MessageResult, error) {
	message, run, err := s.messageStore().FindMessage(messageID)
	if err != nil {
		return MessageResult{}, messageLookupError(messageID, err)
	}
	if !run.Active {
		return MessageResult{}, NewError("message_state_conflict", "archived messages cannot be cancelled")
	}
	active, err := s.activeMessaging(ctx)
	if err != nil {
		return MessageResult{}, err
	}
	if run.ID != active.runID {
		return MessageResult{}, NewError("message_state_conflict", "message is not part of the active run")
	}
	if active.actor != "user" && active.actor != message.Sender {
		return MessageResult{}, NewError("message_forbidden", "only the sender or user can cancel this message")
	}
	if message.Status == messaging.StatusAcknowledged || message.Status == messaging.StatusCancelled {
		return MessageResult{}, NewError("message_state_conflict",
			fmt.Sprintf("message in state %s cannot be cancelled", message.Status))
	}
	if _, err := s.appendActiveRunEvent(active.runID, messaging.Event{
		Type: messaging.EventMessageCancelled, MessageID: message.ID,
		Actor: active.actor, Reason: reason,
	}); err != nil {
		return MessageResult{}, err
	}
	current, err := s.messageInRun(active.runID, message.ID)
	return MessageResult{Message: current}, err
}

func (s *Service) ShowMessage(messageID string) (*messaging.Message, error) {
	message, _, err := s.messageStore().FindMessage(messageID)
	if err != nil {
		return nil, messageLookupError(messageID, err)
	}
	return message, nil
}

func (s *Service) MessageRuns(limit int) (messaging.RunCollection, error) {
	runs, err := s.messageStore().ReadRuns()
	if err != nil {
		return messaging.RunCollection{}, messageStoreError(err)
	}
	total := len(runs)
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return messaging.RunCollection{Total: total, Returned: len(runs), Runs: runs}, nil
}

func (s *Service) MessageHistory(opts MessageHistoryOptions) (messaging.Collection, error) {
	runs, err := s.messageStore().ReadRuns()
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

func (s *Service) MessageInbox(ctx context.Context, identity string, limit int) (messaging.Collection, error) {
	active, err := s.activeMessaging(ctx)
	if err != nil {
		return messaging.Collection{}, err
	}
	if identity == "" {
		identity = active.actor
	} else if identity != active.actor && active.actor != "user" {
		return messaging.Collection{}, NewError("message_forbidden", "only user can inspect another mailbox")
	}
	if identity != "user" {
		if err := validateMailboxName(identity); err != nil {
			return messaging.Collection{}, err
		}
		if _, known := active.state.Agents[identity]; !known {
			return messaging.Collection{}, NewError("message_wrong_recipient",
				fmt.Sprintf("mailbox %q is not known in the active run", identity))
		}
	}
	run, err := s.messageStore().ReadRun(active.runID)
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

func (s *Service) appendActiveRunEvent(runID string, event messaging.Event) (messaging.Event, error) {
	var appended messaging.Event
	err := s.messageStore().WithLifecycleLock(runID, func() error {
		run, err := s.messageStore().ReadRun(runID)
		if err != nil {
			return messageStoreError(err)
		}
		if !run.Active {
			return NewError("message_state_conflict", "the message run is closed")
		}
		appended, err = s.messageStore().Append(runID, event)
		if err != nil {
			return messageStoreError(err)
		}
		return nil
	})
	if err != nil {
		var serviceErr *Error
		if errors.As(err, &serviceErr) {
			return messaging.Event{}, err
		}
		return messaging.Event{}, messageStoreError(err)
	}
	return appended, nil
}

func (s *Service) activeMessaging(ctx context.Context) (activeMessagingContext, error) {
	_, _, client, err := s.running(ctx)
	if err != nil {
		return activeMessagingContext{}, err
	}
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return activeMessagingContext{}, err
	}
	st, err := s.Store.Read(s.Project.Session, s.Project.Root)
	if err != nil {
		return activeMessagingContext{}, Wrap("state_unavailable", err.Error(), err)
	}
	if st.ActiveRunID == "" {
		return activeMessagingContext{}, NewError("message_run_unavailable",
			"this session predates durable messaging; run `fledge stop` followed by `fledge start`")
	}
	run, err := s.messageStore().ReadRun(st.ActiveRunID)
	if err != nil {
		return activeMessagingContext{}, messageStoreError(err)
	}
	if !run.Active {
		return activeMessagingContext{}, NewError("message_run_unavailable", "the active message run is already closed")
	}
	return activeMessagingContext{
		runID: st.ActiveRunID, actor: inferActor(s.CallerPaneID, st, snapshot),
		state: st, snapshot: snapshot, client: client,
	}, nil
}

func inferActor(paneID string, st state.Session, snapshot herdr.Snapshot) string {
	if paneID == "" {
		return "user"
	}
	live := agentsByPane(snapshot)
	for name, managed := range st.Agents {
		agent, ok := live[paneID]
		if managed.PaneID == paneID && ok && agent.Agent != nil && agent.PaneID == paneID {
			return name
		}
	}
	return "user"
}

func (s *Service) deliverIfLive(
	ctx context.Context,
	active activeMessagingContext,
	messageID, recipient string,
) (MessageResult, error) {
	managed, known := active.state.Agents[recipient]
	live := agentsByPane(active.snapshot)
	agent, running := live[managed.PaneID]
	if recipient == "user" || !known || !running || agent.Agent == nil {
		message, err := s.messageInRun(active.runID, messageID)
		return MessageResult{Message: message}, err
	}
	message, deliveryErr, err := s.deliver(ctx, active.runID, messageID, recipient,
		managed.PaneID, managed.ActivationID, active.client)
	if err != nil {
		return MessageResult{}, err
	}
	return MessageResult{Message: message, DeliveryError: deliveryErr}, nil
}

func (s *Service) deliver(
	ctx context.Context,
	runID, messageID, recipient, paneID, activationID string,
	client *herdr.Client,
) (*messaging.Message, string, error) {
	var message *messaging.Message
	var deliveryError string
	err := s.messageStore().WithLifecycleLock(runID, func() error {
		run, err := s.messageStore().ReadRun(runID)
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
		message, deliveryError, innerErr = s.deliverLocked(
			ctx, runID, message, recipient, paneID, activationID, client,
		)
		return innerErr
	})
	if err != nil {
		var serviceErr *Error
		if errors.As(err, &serviceErr) {
			return nil, "", err
		}
		return nil, "", messageStoreError(err)
	}
	return message, deliveryError, nil
}

func (s *Service) deliverLocked(
	ctx context.Context,
	runID string,
	message *messaging.Message,
	recipient, paneID, activationID string,
	client *herdr.Client,
) (*messaging.Message, string, error) {
	messageID := message.ID
	attemptID, err := messaging.NewID("try_")
	if err != nil {
		return message, fmt.Sprintf("delivery was not attempted: generate attempt ID: %v", err), nil
	}
	if _, err := s.messageStore().Append(runID, messaging.Event{
		Type: messaging.EventDeliveryAttempted, MessageID: messageID,
		AttemptID: attemptID, Agent: recipient, ActivationID: activationID, PaneID: paneID,
	}); err != nil {
		return message, fmt.Sprintf("delivery was not attempted: %v", err), nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	callErr := client.Call(callCtx, "agent.prompt", map[string]any{
		"target": paneID, "text": messageEnvelope(message),
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
	if _, err := s.messageStore().Append(runID, messaging.Event{
		Type: outcome, MessageID: messageID, AttemptID: attemptID,
		Agent: recipient, ActivationID: activationID, PaneID: paneID,
		Error: errorText, ErrorKind: errorKind,
	}); err != nil {
		current, readErr := s.messageInRun(runID, messageID)
		if readErr == nil {
			return current, fmt.Sprintf("delivery outcome could not be committed: %v", err), nil
		}
		copy := *message
		copy.Status = messaging.StatusUncertain
		return &copy, fmt.Sprintf("delivery outcome could not be committed: %v", err), nil
	}
	current, err := s.messageInRun(runID, messageID)
	if err != nil {
		return message, fmt.Sprintf("delivery succeeded but its result could not be re-read: %v", err), nil
	}
	return current, errorText, nil
}

func (s *Service) messageInRun(runID, messageID string) (*messaging.Message, error) {
	run, err := s.messageStore().ReadRun(runID)
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

func (s *Service) activateMessagingAgent(
	ctx context.Context, client *herdr.Client, name, paneID string,
) error {
	runID, activationID, err := s.prepareMessagingActivation(name, paneID)
	if err != nil || runID == "" {
		return err
	}
	return s.drainAgentMessages(ctx, client, runID, name, paneID, activationID)
}

func (s *Service) prepareMessagingActivation(name, paneID string) (string, string, error) {
	if st, found, err := s.Store.ReadExisting(s.Project.Session, s.Project.Root); err != nil {
		return "", "", err
	} else if found {
		if managed, ok := st.Agents[name]; ok && managed.ActivationID != "" {
			if err := s.deactivateMessagingAgent(name, "activation superseded"); err != nil {
				return "", "", err
			}
		}
	}
	var runID, activationID string
	err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		runID = st.ActiveRunID
		if runID == "" {
			return nil
		}
		var err error
		activationID, err = messaging.NewID("act_")
		if err != nil {
			return err
		}
		managed := st.Agents[name]
		managed.ActivationID = activationID
		st.Agents[name] = managed
		return nil
	})
	if err != nil {
		return "", "", err
	}
	if runID == "" {
		return "", "", nil
	}
	if _, err := s.appendActiveRunEvent(runID, messaging.Event{
		Type: messaging.EventAgentActivated, Agent: name,
		ActivationID: activationID, PaneID: paneID,
	}); err != nil {
		_ = s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
			managed := st.Agents[name]
			if managed.ActivationID == activationID {
				managed.ActivationID = ""
				st.Agents[name] = managed
			}
			return nil
		})
		return "", "", err
	}
	return runID, activationID, nil
}

// DeliverActivation is used by the bounded hidden helper launched immediately
// before an in-pane exec. It exits quietly if the mapping is superseded.
func (s *Service) DeliverActivation(ctx context.Context, name, activationID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, _, client, err := s.running(ctx)
		if err == nil {
			st, readErr := s.Store.Read(s.Project.Session, s.Project.Root)
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
					return s.drainAgentMessages(ctx, client, st.ActiveRunID, name,
						managed.PaneID, activationID)
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

func (s *Service) drainAgentMessages(
	ctx context.Context, client *herdr.Client, runID, name, paneID, activationID string,
) error {
	run, err := s.messageStore().ReadRun(runID)
	if err != nil {
		return messageStoreError(err)
	}
	for _, message := range run.Messages {
		if message.Recipient != name ||
			(message.Status != messaging.StatusQueued && message.Status != messaging.StatusFailed) ||
			attemptedInActivation(message, activationID) {
			continue
		}
		_, deliveryErr, err := s.deliver(ctx, runID, message.ID, name, paneID, activationID, client)
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

func (s *Service) deactivateMessagingAgent(name, reason string) error {
	var runID, activationID, paneID string
	err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
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
	store := s.messageStore()
	err = store.WithLifecycleLock(runID, func() error {
		run, err := store.ReadRun(runID)
		if err != nil {
			return messageStoreError(err)
		}
		if !run.Active {
			return nil
		}
		if _, err := store.Append(runID, messaging.Event{
			Type: messaging.EventAgentDeactivated, Agent: name,
			ActivationID: activationID, PaneID: paneID, Reason: reason,
		}); err != nil {
			return messageStoreError(err)
		}
		run, err = store.ReadRun(runID)
		if err != nil {
			return messageStoreError(err)
		}
		for _, message := range run.Messages {
			if message.Recipient == name && message.Status == messaging.StatusAwaitingAck &&
				attemptedInActivation(message, activationID) {
				if _, err := store.Append(runID, messaging.Event{
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
		var serviceErr *Error
		if errors.As(err, &serviceErr) {
			return err
		}
		return messageStoreError(err)
	}
	return nil
}

func (s *Service) closeMessageRun(runID, reason string) error {
	if runID == "" {
		return nil
	}
	store := s.messageStore()
	err := store.WithLifecycleLock(runID, func() error {
		run, err := store.ReadRun(runID)
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
				if _, err := store.Append(runID, messaging.Event{
					Type: messaging.EventMessageFailed, MessageID: message.ID, Reason: reason,
				}); err != nil {
					return messageStoreError(err)
				}
			}
		}
		_, err = store.Append(runID, messaging.Event{Type: messaging.EventRunClosed, Reason: reason})
		return messageStoreErrorIf(err)
	})
	if err == nil {
		return nil
	}
	var serviceErr *Error
	if errors.As(err, &serviceErr) {
		return err
	}
	return messageStoreError(err)
}

func (s *Service) closeActiveMessageRun(reason string) error {
	st, found, err := s.Store.ReadExisting(s.Project.Session, s.Project.Root)
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
		if err := s.deactivateMessagingAgent(name, reason); err != nil {
			return err
		}
	}
	return s.closeMessageRun(st.ActiveRunID, reason)
}

func (s *Service) pendingMessageCounts(runID string) (map[string]int, error) {
	counts := map[string]int{}
	if runID == "" {
		return counts, nil
	}
	run, err := s.messageStore().ReadRun(runID)
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
	if name == "user" {
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
