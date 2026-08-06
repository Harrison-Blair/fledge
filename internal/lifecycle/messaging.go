package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
)

const (
	userIdentity         = messaging.UserIdentity
	orchestratorIdentity = messaging.OrchestratorIdentity
)

type messageCaller struct {
	identity string
	paneID   string
	isUser   bool
}

type activeMessageSession struct {
	root    string
	session string
	caller  messageCaller
	unlock  func() error
}

// SendMessage sends one audited message to a live named agent.
func (m *Manager) SendMessage(ctx context.Context, dir, recipient, body string) (_ messaging.Message, resultErr error) {
	if err := messaging.ValidateBody(body); err != nil {
		return messaging.Message{}, err
	}
	active, err := m.activeMessageSession(dir)
	if err != nil {
		return messaging.Message{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, active.unlock()) }()
	if recipient == userIdentity {
		return messaging.Message{}, errors.New("recipient \"user\" is reserved for replies")
	}
	store := messaging.New(active.root, active.session)
	recipientAgent, err := store.Agent(recipient)
	if err != nil {
		return messaging.Message{}, fmt.Errorf("live agent %q was not found in the project's Fledge session: %w", recipient, err)
	}
	recipientPane := recipientAgent.PaneID
	if active.caller.identity == recipient {
		return messaging.Message{}, errors.New("cannot send a message to yourself")
	}

	logger, closeLog := m.sessionLogger(active.root, active.session)
	defer closeLog()
	defer func() { logOutcome(logger, "message send", resultErr) }()
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
	logger.Info("message queued", "message_id", message.ID, "recipient", message.Recipient)
	m.launchWatcherWarn(active.root)
	return message, nil
}

// ReplyMessage creates and immediately submits a correlated reply.
func (m *Manager) ReplyMessage(ctx context.Context, dir, originalID, body string) (_ messaging.Message, resultErr error) {
	if err := messaging.ValidateBody(body); err != nil {
		return messaging.Message{}, err
	}
	active, err := m.activeMessageSession(dir)
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
		recipientAgent, lookupErr := store.Agent(original.Sender)
		if lookupErr != nil {
			return messaging.Message{}, fmt.Errorf("cannot reply to %s: %w", original.Sender, lookupErr)
		}
		replyRecipientPane = recipientAgent.PaneID
	}
	reply, err := store.Reply(originalID, active.caller.identity, active.caller.paneID, body, replyRecipientPane)
	if err != nil {
		return messaging.Message{}, err
	}
	logger.Info("message created", "message_id", reply.ID, "sender", reply.Sender, "recipient", reply.Recipient, "reply_to", originalID, "body_bytes", len(body))
	if reply.Recipient == userIdentity {
		return reply, nil
	}
	logger.Info("message queued", "message_id", reply.ID, "recipient", reply.Recipient)
	m.launchWatcherWarn(active.root)
	return reply, nil
}

// MessageInbox returns a complete transcript to direct-user/control-shell
// callers, along with the identity it resolved the transcript for. Callers
// render message direction against that identity, so the default lives here
// only.
func (m *Manager) MessageInbox(ctx context.Context, dir, identity string) (_ []messaging.Message, _ string, resultErr error) {
	active, err := m.activeMessageSession(dir)
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

func (m *Manager) activeMessageSession(dir string) (activeMessageSession, error) {
	root, err := project.Find(dir)
	if err != nil {
		return activeMessageSession{}, err
	}
	value, found, err := readRecord(root)
	if err != nil {
		return activeMessageSession{}, err
	}
	if !found {
		return activeMessageSession{}, errors.New("project has no Fledge session; run fledge start first")
	}
	store := messaging.New(root, value.SessionName)
	if err := validateStoreBinding(store, value); err != nil {
		return activeMessageSession{}, err
	}
	caller, err := m.paneCaller(store)
	if err != nil {
		return activeMessageSession{}, err
	}
	if err := project.EnsureRuntimeIgnore(root); err != nil {
		return activeMessageSession{}, err
	}
	return activeMessageSession{root: root, session: value.SessionName, caller: caller, unlock: func() error { return nil }}, nil
}

// paneCaller maps HERDR_PANE_ID onto the durable registry without querying
// Herdr. A pane the registry has never seen is the user's own control shell. A
// pane whose agent has been stopped is emphatically not: downgrading it to the
// direct user would hand a departed agent's leftover process authority to
// delegate, retarget, and cancel every task in the session, and to read every
// transcript. New panes also carry a random bearer token whose hash is in the
// registry. That second binding prevents changing or clearing HERDR_PANE_ID
// alone from changing identity, while keeping coordination commands free of
// Herdr calls.
func (m *Manager) paneCaller(store *messaging.Store) (messageCaller, error) {
	paneID := m.getenv("HERDR_PANE_ID")
	authority := m.getenv(paneAuthorityEnvironment)
	if authority != "" {
		bound, err := store.AgentByAuthorityHashAny(paneAuthorityHash(authority))
		if err != nil {
			return messageCaller{}, fmt.Errorf("%w: pane authority is not registered to this session", messaging.ErrUnauthorized)
		}
		if !bound.Active {
			return messageCaller{}, fmt.Errorf("%w: agent %q has been stopped, so its pane authority is revoked", messaging.ErrUnauthorized, bound.Name)
		}
		if paneID != "" && paneID != bound.PaneID {
			return messageCaller{}, fmt.Errorf("%w: pane identity does not match its authority binding", messaging.ErrUnauthorized)
		}
		return messageCaller{identity: bound.Name, paneID: bound.PaneID}, nil
	}
	if paneID == "" {
		return messageCaller{identity: userIdentity, isUser: true}, nil
	}
	agent, err := store.AgentByPaneAny(paneID)
	if errors.Is(err, messaging.ErrAgentNotFound) {
		return messageCaller{identity: userIdentity, paneID: paneID, isUser: true}, nil
	}
	if err != nil {
		return messageCaller{}, err
	}
	if !agent.Active {
		return messageCaller{}, fmt.Errorf("%w: agent %q has been stopped, so pane %s no longer holds session authority",
			messaging.ErrUnauthorized, agent.Name, paneID)
	}
	if agent.AuthorityHash != "" {
		return messageCaller{}, fmt.Errorf("%w: agent %q is missing its pane authority", messaging.ErrUnauthorized, agent.Name)
	}
	return messageCaller{identity: agent.Name, paneID: paneID}, nil
}
