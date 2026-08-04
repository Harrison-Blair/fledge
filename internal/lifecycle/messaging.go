package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
)

const (
	userIdentity    = "user"
	watcherIdentity = "watcher"
)

type messageCaller struct {
	identity string
	paneID   string
	isUser   bool
}

type activeMessageSession struct {
	root     string
	session  string
	snapshot herdr.Snapshot
	caller   messageCaller
	unlock   func() error
}

// SendMessage sends one audited message to a live named agent.
func (m *Manager) SendMessage(ctx context.Context, dir, recipient, body string) (_ messaging.Message, resultErr error) {
	if err := messaging.ValidateBody(body); err != nil {
		return messaging.Message{}, err
	}
	active, err := m.activeMessageSession(ctx, dir, nil)
	if err != nil {
		return messaging.Message{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, active.unlock()) }()
	if recipient == userIdentity {
		return messaging.Message{}, errors.New("recipient \"user\" is reserved for replies")
	}
	recipientPane, err := liveAgentPane(active.snapshot, recipient)
	if err != nil {
		return messaging.Message{}, err
	}
	if active.caller.identity == recipient {
		return messaging.Message{}, errors.New("cannot send a message to yourself")
	}

	logger, closeLog := m.sessionLogger(active.root, active.session)
	defer closeLog()
	defer func() { logOutcome(logger, "message send", resultErr) }()
	store := messaging.New(active.root, active.session)
	if _, err := store.Ensure(); err != nil {
		return messaging.Message{}, err
	}
	message, err := store.Create(messaging.CreateParams{
		Sender: active.caller.identity, Recipient: recipient, Body: body, RecipientPane: recipientPane,
	})
	if err != nil {
		return messaging.Message{}, err
	}
	logger.Info("message created", "message_id", message.ID, "sender", message.Sender, "recipient", message.Recipient, "body_bytes", len(body))
	return m.deliverMessage(ctx, logger, active.session, store, message)
}

// SendWatcherWake sends one audited automated notification to the orchestrator.
func (m *Manager) SendWatcherWake(ctx context.Context, dir, body string) (_ messaging.Message, resultErr error) {
	if err := messaging.ValidateBody(body); err != nil {
		return messaging.Message{}, err
	}
	active, err := m.activeMessageSession(ctx, dir, &messageCaller{identity: watcherIdentity})
	if err != nil {
		return messaging.Message{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, active.unlock()) }()
	recipientPane, err := liveAgentPane(active.snapshot, "orchestrator")
	if err != nil {
		return messaging.Message{}, err
	}

	logger, closeLog := m.sessionLogger(active.root, active.session)
	defer closeLog()
	defer func() { logOutcome(logger, "watcher wake", resultErr) }()
	store := messaging.New(active.root, active.session)
	if _, err := store.Ensure(); err != nil {
		return messaging.Message{}, err
	}
	message, err := store.Create(messaging.CreateParams{
		Sender: watcherIdentity, Recipient: "orchestrator", Body: body, RecipientPane: recipientPane,
	})
	if err != nil {
		return messaging.Message{}, err
	}
	logger.Info("message created", "message_id", message.ID, "sender", watcherIdentity, "recipient", "orchestrator", "body_bytes", len(body))
	return m.deliverMessage(ctx, logger, active.session, store, message)
}

// ReplyMessage creates and immediately submits a correlated reply.
func (m *Manager) ReplyMessage(ctx context.Context, dir, originalID, body string) (_ messaging.Message, resultErr error) {
	if err := messaging.ValidateBody(body); err != nil {
		return messaging.Message{}, err
	}
	active, err := m.activeMessageSession(ctx, dir, nil)
	if err != nil {
		return messaging.Message{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, active.unlock()) }()
	logger, closeLog := m.sessionLogger(active.root, active.session)
	defer closeLog()
	defer func() { logOutcome(logger, "message reply", resultErr) }()
	store := messaging.New(active.root, active.session)
	if _, err := store.Ensure(); err != nil {
		return messaging.Message{}, err
	}
	original, err := store.Get(originalID)
	if err != nil {
		return messaging.Message{}, err
	}
	if original.Recipient != active.caller.identity || original.RecipientPane != active.caller.paneID {
		return messaging.Message{}, fmt.Errorf("%w: message %s does not belong to the caller", messaging.ErrUnauthorized, originalID)
	}

	replyRecipientPane := ""
	if original.Sender != userIdentity {
		replyRecipientPane, err = liveAgentPane(active.snapshot, original.Sender)
		if err != nil {
			return messaging.Message{}, fmt.Errorf("cannot reply to %s: %w", original.Sender, err)
		}
	}
	reply, err := store.Reply(originalID, active.caller.identity, active.caller.paneID, body, replyRecipientPane)
	if err != nil {
		return messaging.Message{}, err
	}
	logger.Info("message created", "message_id", reply.ID, "sender", reply.Sender, "recipient", reply.Recipient, "reply_to", originalID, "body_bytes", len(body))
	if reply.Recipient == userIdentity {
		return reply, nil
	}
	return m.deliverMessage(ctx, logger, active.session, store, reply)
}

// MessageInbox returns a complete transcript to direct-user/control-shell
// callers, along with the identity it resolved the transcript for. Callers
// render message direction against that identity, so the default lives here
// only.
func (m *Manager) MessageInbox(ctx context.Context, dir, identity string) (_ []messaging.Message, _ string, resultErr error) {
	active, err := m.activeMessageSession(ctx, dir, nil)
	if err != nil {
		return nil, "", err
	}
	defer func() { resultErr = errors.Join(resultErr, active.unlock()) }()
	if !active.caller.isUser {
		return nil, "", errors.New("managed agents cannot query message transcripts")
	}
	if identity == "" {
		identity = userIdentity
	}

	logger, closeLog := m.sessionLogger(active.root, active.session)
	defer closeLog()
	defer func() { logOutcome(logger, "message inbox", resultErr) }()
	logger.Info("inbox queried", "identity", identity)
	store := messaging.New(active.root, active.session)
	if _, err := store.Ensure(); err != nil {
		return nil, "", err
	}
	messages, err := store.Inbox(identity)
	return messages, identity, err
}

func (m *Manager) deliverMessage(ctx context.Context, logger *slog.Logger, session string, store *messaging.Store, message messaging.Message) (messaging.Message, error) {
	if _, err := store.RecordAttempt(message.ID); err != nil {
		return messaging.Message{}, err
	}
	logger.Debug("delivery attempt", "message_id", message.ID, "recipient", message.Recipient)
	prompt := messageEnvelope(message)
	if err := m.herdr.PromptAgent(ctx, session, message.Recipient, prompt); err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			interruptionErr := errors.Join(ctx.Err(), err)
			logger.Warn("delivery interrupted", "message_id", message.ID, "recipient", message.Recipient, "err", interruptionErr.Error())
			uncertain, getErr := store.Get(message.ID)
			return uncertain, errors.Join(fmt.Errorf("delivery of message %s to %s was interrupted: %w", message.ID, message.Recipient, interruptionErr), getErr)
		}
		logger.Warn("delivery failed", "message_id", message.ID, "recipient", message.Recipient, "err", err.Error())
		failed, recordErr := store.RecordDelivery(message.ID, false, err.Error())
		return failed, errors.Join(fmt.Errorf("deliver message %s to %s: %w", message.ID, message.Recipient, err), recordErr)
	}
	delivered, err := store.RecordDelivery(message.ID, true, "")
	if err != nil {
		return messaging.Message{}, fmt.Errorf("message %s was submitted, but recording its delivery outcome failed: %w", message.ID, err)
	}
	logger.Debug("delivered", "message_id", message.ID, "recipient", message.Recipient)
	return delivered, nil
}

func messageEnvelope(message messaging.Message) string {
	var envelope strings.Builder
	fmt.Fprintf(&envelope, "[Fledge message]\nID: %s\nFrom: %s\nTo: %s\n", message.ID, message.Sender, message.Recipient)
	if message.ReplyTo != "" {
		fmt.Fprintf(&envelope, "Reply-To: %s\n", message.ReplyTo)
	}
	fmt.Fprintf(&envelope, "Body:\n%s\n\nReply: fledge agent message reply %s <text>", message.Body, message.ID)
	return envelope.String()
}

func (m *Manager) activeMessageSession(ctx context.Context, dir string, forcedCaller *messageCaller) (activeMessageSession, error) {
	root, err := project.Find(dir)
	if err != nil {
		return activeMessageSession{}, err
	}
	if err := m.herdr.Check(); err != nil {
		return activeMessageSession{}, err
	}
	value, found, err := readRecord(root)
	if err != nil {
		return activeMessageSession{}, err
	}
	if !found {
		return activeMessageSession{}, errors.New("project has no Fledge session; run fledge start first")
	}
	unlock, err := lockSessionRecord(root)
	if err != nil {
		return activeMessageSession{}, err
	}
	fail := func(operationErr error) (activeMessageSession, error) {
		return activeMessageSession{}, errors.Join(operationErr, unlock())
	}
	lockedValue, stillPresent, err := readRecord(root)
	if err != nil {
		return fail(err)
	}
	if !stillPresent || lockedValue.SessionName != value.SessionName {
		return fail(errors.New("Fledge session record changed while preparing messaging; retry the command"))
	}
	sessions, err := m.herdr.List(ctx)
	if err != nil {
		return fail(err)
	}
	session, exists := sessionByName(sessions, lockedValue.SessionName)
	if !exists || !session.Running {
		return fail(errors.New("project's Fledge session is not running; run fledge start first"))
	}
	snapshot, err := m.herdr.Snapshot(ctx, lockedValue.SessionName)
	if err != nil {
		return fail(err)
	}
	caller := messageCaller{}
	if forcedCaller != nil {
		caller = *forcedCaller
	} else {
		caller, err = inferMessageCaller(m.getenv("HERDR_PANE_ID"), snapshot)
		if err != nil {
			return fail(err)
		}
	}
	if err := project.EnsureRuntimeIgnore(root); err != nil {
		return fail(err)
	}
	return activeMessageSession{root: root, session: lockedValue.SessionName, snapshot: snapshot, caller: caller, unlock: unlock}, nil
}

func inferMessageCaller(paneID string, snapshot herdr.Snapshot) (messageCaller, error) {
	if paneID == "" {
		return messageCaller{identity: userIdentity, isUser: true}, nil
	}
	for _, agent := range snapshot.Agents {
		if agent.PaneID != paneID {
			continue
		}
		if agent.Name == nil || strings.TrimSpace(*agent.Name) == "" {
			return messageCaller{}, fmt.Errorf("Herdr pane %q is not a recognized named agent pane", paneID)
		}
		if *agent.Name == userIdentity {
			return messageCaller{}, errors.New("agent name \"user\" is reserved for the direct user")
		}
		return messageCaller{identity: *agent.Name, paneID: paneID}, nil
	}
	for _, pane := range snapshot.Panes {
		if pane.PaneID == paneID {
			return messageCaller{identity: userIdentity, isUser: true}, nil
		}
	}
	return messageCaller{}, fmt.Errorf("HERDR_PANE_ID %q does not belong to the active project session", paneID)
}

func liveAgentPane(snapshot herdr.Snapshot, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("recipient must not be blank")
	}
	match := ""
	for _, agent := range snapshot.Agents {
		if agent.Name == nil || *agent.Name != name || agent.PaneID == "" {
			continue
		}
		if match != "" && match != agent.PaneID {
			return "", fmt.Errorf("live agent name %q is ambiguous", name)
		}
		match = agent.PaneID
	}
	if match == "" {
		return "", fmt.Errorf("live agent %q was not found in the project's Fledge session", name)
	}
	return match, nil
}
