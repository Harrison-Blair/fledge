package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// A stopped agent's pane must lose its authority outright. Downgrading it to
// the direct user — the only other way to read an unrecognized pane — would let
// a departed agent's leftover process delegate work, cancel any task in the
// session, and read every transcript.
func TestStoppedPaneLosesSessionAuthority(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	store := messaging.New(root, testSessionName)
	if err := store.StopAgent("worker", "p-worker"); err != nil {
		t.Fatal(err)
	}
	manager.getenv = func(name string) string {
		if name == "HERDR_PANE_ID" {
			return "p-worker"
		}
		return ""
	}
	ctx := context.Background()
	cases := map[string]func() error{
		"send": func() error {
			_, err := manager.SendMessage(ctx, root, "orchestrator", "still here")
			return err
		},
		"inbox": func() error {
			_, _, err := manager.MessageInbox(ctx, root, "")
			return err
		},
		"assign": func() error {
			_, err := manager.TaskAssign(ctx, root, "orchestrator", "", "do my bidding")
			return err
		},
		"list": func() error {
			_, err := manager.TaskList(ctx, root)
			return err
		},
		"spawn": func() error {
			return manager.Spawn(ctx, root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "usurper", Harness: "codex"})
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			err := run()
			if err == nil {
				t.Fatal("stopped pane was granted authority")
			}
			if !strings.Contains(err.Error(), "no longer holds session authority") {
				t.Fatalf("error = %v, want an authority revocation", err)
			}
		})
	}
}

func TestPaneAuthorityPreventsPaneIDSpoofingAndSurvivesClearedPaneID(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	store := messaging.New(root, testSessionName)
	const token = "bound-pane-secret"
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "bound", PaneID: "p-bound", Harness: "codex", AuthorityHash: paneAuthorityHash(token), Caller: orchestratorIdentity,
	}); err != nil {
		t.Fatal(err)
	}

	paneID, authority := "", token
	manager.getenv = func(name string) string {
		switch name {
		case "HERDR_PANE_ID":
			return paneID
		case paneAuthorityEnvironment:
			return authority
		default:
			return ""
		}
	}
	message, err := manager.SendMessage(context.Background(), root, orchestratorIdentity, "pane id was cleared")
	if err != nil || message.Sender != "bound" {
		t.Fatalf("SendMessage() = %#v, %v, want bound identity recovered from authority", message, err)
	}

	paneID = "p-worker"
	if _, err := manager.SendMessage(context.Background(), root, orchestratorIdentity, "spoofed pane"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("spoofed pane error = %v, want unauthorized", err)
	}

	paneID, authority = "p-bound", ""
	if _, err := manager.SendMessage(context.Background(), root, orchestratorIdentity, "missing authority"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("missing authority error = %v, want unauthorized", err)
	}
}

// A process that deliberately clears both inherited identity variables is
// indistinguishable from the user's control shell without querying Herdr or
// adding OS-specific process attestation. This is the irreducible local
// protocol boundary; clearing HERDR_PANE_ID alone is covered above.
func TestClearingBothPaneIdentityVariablesIsIndistinguishableFromControlShell(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	manager.getenv = func(string) string { return "" }
	_, identity, err := manager.MessageInbox(context.Background(), root, "")
	if err != nil || identity != userIdentity {
		t.Fatalf("MessageInbox() = %q, %v, want direct-user identity", identity, err)
	}
}

func TestCoordinationRejectsUnboundAndMismatchedSessionRecordsWithoutHerdr(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		sessionID string
		want      error
		contains  string
	}{
		{name: "unbound", contains: "not bound to durable session state"},
		{name: "mismatch", sessionID: "different-session-id", want: messaging.ErrSessionMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, client, root := newMessagingManager(t)
			value, found, err := readRecord(root)
			if err != nil || !found {
				t.Fatal(err)
			}
			value.MessagingSessionID = test.sessionID
			if err := rewriteRecord(root, value); err != nil {
				t.Fatal(err)
			}
			_, err = manager.TaskList(context.Background(), root)
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("TaskList() error = %v, want %v", err, test.want)
			}
			if test.contains != "" && (err == nil || !strings.Contains(err.Error(), test.contains)) {
				t.Fatalf("TaskList() error = %v, want containing %q", err, test.contains)
			}
			if client.listCalls != 0 {
				t.Fatalf("coordination validation made %d Herdr list calls", client.listCalls)
			}
		})
	}
}

// The user's own control shell runs inside a Herdr pane that Fledge never
// registered, so an unknown pane keeps direct-user authority.
func TestUnregisteredPaneKeepsDirectUserAuthority(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	manager.getenv = func(name string) string {
		if name == "HERDR_PANE_ID" {
			return "p-control-shell"
		}
		return ""
	}
	messages, identity, err := manager.MessageInbox(context.Background(), root, "")
	if err != nil {
		t.Fatalf("MessageInbox() = %v", err)
	}
	if identity != userIdentity || len(messages) != 0 {
		t.Fatalf("inbox = %#v for %q", messages, identity)
	}
}

// A live registered pane speaks as its own agent, never as the user.
func TestRegisteredPaneSpeaksAsItsAgent(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	manager.getenv = func(name string) string {
		if name == "HERDR_PANE_ID" {
			return "p-worker"
		}
		return ""
	}
	message, err := manager.SendMessage(context.Background(), root, "orchestrator", "progress")
	if err != nil {
		t.Fatal(err)
	}
	if message.Sender != "worker" {
		t.Fatalf("sender = %q, want worker", message.Sender)
	}
	if _, _, err := manager.MessageInbox(context.Background(), root, ""); err == nil {
		t.Fatal("a managed agent read the transcript")
	}
}
