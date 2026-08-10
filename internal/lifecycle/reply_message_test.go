package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// paneEnv builds a getenv stub that reports the given HERDR_PANE_ID and nothing
// else, so paneCaller resolves the caller to the agent bound to that pane.
func paneEnv(paneID string) func(string) string {
	return func(key string) string {
		if key == "HERDR_PANE_ID" {
			return paneID
		}
		return ""
	}
}

// deliverStoredMessage appends a message directly to the store and drives its
// wake to a delivered outcome, leaving the message in a state a reply is allowed
// to correlate against. It returns the reconstructed, delivered message.
func deliverStoredMessage(t *testing.T, store *messaging.Store, sender, recipient, recipientPane, body string) messaging.Message {
	t.Helper()
	message, err := store.Create(messaging.CreateParams{
		Sender: sender, Recipient: recipient, Body: body, RecipientPane: recipientPane,
	})
	if err != nil {
		t.Fatalf("create %s->%s message: %v", sender, recipient, err)
	}
	wakeID := "w-" + message.ID
	if _, err := store.RecordWakeAttempt(wakeID); err != nil {
		t.Fatalf("record wake attempt: %v", err)
	}
	if _, err := store.RecordWakeOutcome(wakeID, true, ""); err != nil {
		t.Fatalf("record wake outcome: %v", err)
	}
	delivered, err := store.Get(message.ID)
	if err != nil {
		t.Fatalf("reload delivered message: %v", err)
	}
	if delivered.Status != messaging.StatusDelivered {
		t.Fatalf("message status = %s, want delivered", delivered.Status)
	}
	return delivered
}

// TestReplyMessageRoutesToSenderAndWakes covers the happy path: a worker replying
// to an orchestrator message correlates the reply, routes it back to the
// orchestrator's pane, queues exactly one wake, and launches the dispatcher.
func TestReplyMessageRoutesToSenderAndWakes(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	manager.getenv = paneEnv("p-worker")
	launches := 0
	manager.watchLauncher = func(string) error { launches++; return nil }

	store := messaging.New(root, testSessionName)
	original := deliverStoredMessage(t, store, "orchestrator", "worker", "p-worker", "please review")

	reply, err := manager.ReplyMessage(context.Background(), root, original.ID, "done reviewing")
	if err != nil {
		t.Fatalf("ReplyMessage: %v", err)
	}
	if reply.ReplyTo != original.ID {
		t.Fatalf("reply.ReplyTo = %q, want %q", reply.ReplyTo, original.ID)
	}
	if reply.Sender != "worker" {
		t.Fatalf("reply.Sender = %q, want worker", reply.Sender)
	}
	if reply.Recipient != "orchestrator" || reply.RecipientPane != "p-orchestrator" {
		t.Fatalf("reply routed to %q pane %q, want orchestrator pane p-orchestrator", reply.Recipient, reply.RecipientPane)
	}
	if launches != 1 {
		t.Fatalf("dispatcher launches = %d, want 1", launches)
	}
	if len(client.calls) != 0 {
		t.Fatalf("Herdr calls = %v, want none", client.calls)
	}
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(wakes) != 1 || wakes[0].Recipient != "orchestrator" || wakes[0].ReferenceID != reply.ID {
		t.Fatalf("pending wakes = %#v, want one addressed to orchestrator for the reply", wakes)
	}
}

// TestReplyMessageToUserOriginatedDoesNotLaunch locks the local-delivery branch:
// a reply to a user-originated message is addressed straight to the user and
// launches no dispatcher because there is nothing to wake.
func TestReplyMessageToUserOriginatedDoesNotLaunch(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	manager.getenv = paneEnv("p-worker")
	launches := 0
	manager.watchLauncher = func(string) error { launches++; return nil }

	store := messaging.New(root, testSessionName)
	original := deliverStoredMessage(t, store, "user", "worker", "p-worker", "status?")

	reply, err := manager.ReplyMessage(context.Background(), root, original.ID, "all green")
	if err != nil {
		t.Fatalf("ReplyMessage: %v", err)
	}
	if reply.Recipient != userIdentity || reply.RecipientPane != "" {
		t.Fatalf("reply = %#v, want recipient user with no pane", reply)
	}
	if launches != 0 {
		t.Fatalf("dispatcher launches = %d, want 0 for a user reply", launches)
	}
	if len(client.calls) != 0 {
		t.Fatalf("Herdr calls = %v, want none", client.calls)
	}
	wakes, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(wakes) != 0 {
		t.Fatalf("pending wakes = %#v, want none for a user reply", wakes)
	}
}

// TestReplyMessageUnauthorized rejects a caller replying to a message that does
// not name them as its recipient, before any reply is appended.
func TestReplyMessageUnauthorized(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	// Reply as the orchestrator to a message whose recipient is the worker.
	manager.getenv = paneEnv("p-orchestrator")
	launches := 0
	manager.watchLauncher = func(string) error { launches++; return nil }

	store := messaging.New(root, testSessionName)
	// The message is addressed to the worker, so the orchestrator caller has no
	// claim on it.
	foreign := deliverStoredMessage(t, store, "orchestrator", "worker", "p-worker", "for the worker")

	_, err := manager.ReplyMessage(context.Background(), root, foreign.ID, "not mine")
	if !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("ReplyMessage error = %v, want ErrUnauthorized", err)
	}
	if launches != 0 {
		t.Fatalf("dispatcher launches = %d, want 0 on an unauthorized reply", launches)
	}
}

// TestReplyMessageSenderStopped surfaces a lookup error when the original sender's
// agent has been stopped, so its pane can no longer receive the reply.
func TestReplyMessageSenderStopped(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	manager.getenv = paneEnv("p-worker")
	launches := 0
	manager.watchLauncher = func(string) error { launches++; return nil }

	store := messaging.New(root, testSessionName)
	original := deliverStoredMessage(t, store, "orchestrator", "worker", "p-worker", "please review")
	if err := store.StopAgent("orchestrator", "p-orchestrator"); err != nil {
		t.Fatalf("stop orchestrator: %v", err)
	}

	_, err := manager.ReplyMessage(context.Background(), root, original.ID, "done")
	if err == nil || !strings.Contains(err.Error(), "cannot reply to orchestrator") {
		t.Fatalf("ReplyMessage error = %v, want a 'cannot reply to orchestrator' lookup error", err)
	}
	if !errors.Is(err, messaging.ErrAgentNotFound) {
		t.Fatalf("ReplyMessage error = %v, want wrapped ErrAgentNotFound", err)
	}
	if launches != 0 {
		t.Fatalf("dispatcher launches = %d, want 0 when the sender is gone", launches)
	}
}
