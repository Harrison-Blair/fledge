package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

func TestInferMessageCaller(t *testing.T) {
	t.Parallel()
	snapshot := messagingSnapshot()
	unnamedPane := "p-unnamed"
	reservedPane, reservedName := "p-reserved", userIdentity
	snapshot.Panes = append(snapshot.Panes, herdr.Pane{PaneID: unnamedPane})
	snapshot.Panes = append(snapshot.Panes, herdr.Pane{PaneID: reservedPane})
	snapshot.Agents = append(snapshot.Agents, herdr.Agent{PaneID: unnamedPane}, herdr.Agent{PaneID: reservedPane, Name: &reservedName})

	tests := []struct {
		name, pane, identity, boundPane string
		user                            bool
		wantError                       string
	}{
		{name: "outside Herdr", identity: userIdentity, user: true},
		{name: "control pane", pane: "p-control", identity: userIdentity, user: true},
		{name: "named agent", pane: "p-worker", identity: "worker", boundPane: "p-worker"},
		{name: "unnamed agent", pane: unnamedPane, wantError: "not a recognized named agent"},
		{name: "reserved agent name", pane: reservedPane, wantError: "is reserved"},
		{name: "foreign pane", pane: "foreign", wantError: "does not belong"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller, err := inferMessageCaller(test.pane, snapshot)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("inferMessageCaller() error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if caller.identity != test.identity || caller.paneID != test.boundPane || caller.isUser != test.user {
				t.Errorf("caller = %#v", caller)
			}
		})
	}
}

func TestSendMessageDeliversAuditedEnvelope(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	client.promptHook = func() {
		messages, err := messaging.New(root, testSessionName).List()
		if err != nil || len(messages) != 1 || messages[0].Status != messaging.StatusUncertain {
			t.Fatalf("state at injection = %#v, %v", messages, err)
		}
	}

	message, err := manager.SendMessage(context.Background(), root, "worker", "Review this\ncarefully")
	if err != nil {
		t.Fatal(err)
	}
	if message.Sender != userIdentity || message.Recipient != "worker" || message.RecipientPane != "p-worker" || message.Status != messaging.StatusDelivered {
		t.Errorf("message = %#v", message)
	}
	if len(client.promptCalls) != 1 || client.promptCalls[0].recipient != "worker" {
		t.Fatalf("PromptAgent calls = %#v", client.promptCalls)
	}
	prompt := client.promptCalls[0].prompt
	for _, want := range []string{
		"ID: " + message.ID, "From: user", "To: worker", "Body:\nReview this\ncarefully",
		"Reply: fledge agent message reply " + message.ID + " <text>",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt = %q, want %q", prompt, want)
		}
	}
	if strings.Contains(prompt, "ack") {
		t.Errorf("prompt contains acknowledgement guidance: %q", prompt)
	}
	stored, err := messaging.New(root, testSessionName).Get(message.ID)
	if err != nil || stored.Status != messaging.StatusDelivered {
		t.Fatalf("stored message = %#v, %v", stored, err)
	}
}

func TestSendMessageAuthorizationAndRecipientRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, callerPane, recipient, want string
		mutateSnapshot                    func(*herdr.Snapshot)
	}{
		{name: "self message", callerPane: "p-worker", recipient: "worker", want: "yourself"},
		{name: "user reserved", recipient: userIdentity, want: "reserved for replies"},
		{name: "stopped recipient", recipient: "stopped", want: "was not found"},
		{name: "foreign caller", callerPane: "foreign", recipient: "worker", want: "does not belong"},
		{name: "ambiguous recipient", recipient: "worker", want: "ambiguous", mutateSnapshot: func(snapshot *herdr.Snapshot) {
			name := "worker"
			snapshot.Agents = append(snapshot.Agents, herdr.Agent{Name: &name, PaneID: "p-other"})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, client, root := newMessagingManager(t)
			manager.getenv = func(string) string { return test.callerPane }
			if test.mutateSnapshot != nil {
				test.mutateSnapshot(&client.snapshot)
			}
			_, err := manager.SendMessage(context.Background(), root, test.recipient, "body")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("SendMessage() error = %v, want %q", err, test.want)
			}
			if len(client.promptCalls) != 0 {
				t.Fatalf("PromptAgent calls = %#v", client.promptCalls)
			}
		})
	}

	manager, client, root := newMessagingManager(t)
	message, err := manager.SendMessage(context.Background(), root, "orchestrator", "coordinate")
	if err != nil {
		t.Fatal(err)
	}
	if message.Recipient != "orchestrator" || client.promptCalls[0].recipient != "orchestrator" {
		t.Fatalf("orchestrator delivery = %#v, %#v", message, client.promptCalls)
	}
}

func TestSendMessageReportedFailureIsDurablyFailed(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	client.promptErr = errors.New("submission rejected")

	message, err := manager.SendMessage(context.Background(), root, "worker", "body")
	if err == nil || !strings.Contains(err.Error(), "submission rejected") {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if message.Status != messaging.StatusFailed || !strings.Contains(message.Failure, "submission rejected") {
		t.Fatalf("message = %#v", message)
	}
	stored, getErr := messaging.New(root, testSessionName).Get(message.ID)
	if getErr != nil || stored.Status != messaging.StatusFailed {
		t.Fatalf("stored message = %#v, %v", stored, getErr)
	}
}

func TestInterruptedSendRemainsUncertain(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	client.promptErr = errors.New("signal: killed")
	ctx, cancel := context.WithCancel(context.Background())
	client.promptHook = cancel

	message, err := manager.SendMessage(ctx, root, "worker", "body")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SendMessage() error = %v, want context cancellation", err)
	}
	if message.Status != messaging.StatusUncertain {
		t.Fatalf("message = %#v", message)
	}
	stored, getErr := messaging.New(root, testSessionName).Get(message.ID)
	if getErr != nil || stored.Status != messaging.StatusUncertain {
		t.Fatalf("stored message = %#v, %v", stored, getErr)
	}
}

func TestReplyToUserPreservesOriginalAndEntersUserTranscript(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	original, err := manager.SendMessage(context.Background(), root, "worker", "question")
	if err != nil {
		t.Fatal(err)
	}
	client.promptCalls = nil
	manager.getenv = func(string) string { return "p-worker" }

	reply, err := manager.ReplyMessage(context.Background(), root, original.ID, "answer")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Sender != "worker" || reply.Recipient != userIdentity || reply.ReplyTo != original.ID || reply.Status != messaging.StatusDelivered {
		t.Fatalf("reply = %#v", reply)
	}
	if len(client.promptCalls) != 0 {
		t.Fatalf("local user reply used Herdr: %#v", client.promptCalls)
	}
	storedOriginal, err := messaging.New(root, testSessionName).Get(original.ID)
	if err != nil || storedOriginal != original {
		t.Fatalf("original = %#v, %v", storedOriginal, err)
	}

	manager.getenv = func(string) string { return "" }
	inbox, err := manager.MessageInbox(context.Background(), root, "")
	if err != nil || len(inbox) != 2 || inbox[0].ID != original.ID || inbox[1].ID != reply.ID {
		t.Fatalf("user inbox = %#v, %v", inbox, err)
	}
}

func TestReplyToLiveAgentUsesCurrentBoundPane(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	manager.getenv = func(string) string { return "p-worker" }
	original, err := manager.SendMessage(context.Background(), root, "orchestrator", "question")
	if err != nil {
		t.Fatal(err)
	}
	manager.getenv = func(string) string { return "p-orchestrator" }

	reply, err := manager.ReplyMessage(context.Background(), root, original.ID, "answer")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Recipient != "worker" || reply.RecipientPane != "p-worker" || reply.Status != messaging.StatusDelivered {
		t.Fatalf("reply = %#v", reply)
	}
	if got := client.promptCalls[len(client.promptCalls)-1].recipient; got != "worker" {
		t.Fatalf("reply PromptAgent recipient = %q", got)
	}
}

func TestMessageAuthorizationAndRespawnBinding(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	message, err := manager.SendMessage(context.Background(), root, "worker", "body")
	if err != nil {
		t.Fatal(err)
	}

	manager.getenv = func(string) string { return "p-orchestrator" }
	if _, err := manager.ReplyMessage(context.Background(), root, message.ID, "intrude"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("foreign agent reply error = %v", err)
	}
	if _, err := manager.MessageInbox(context.Background(), root, "worker"); err == nil || !strings.Contains(err.Error(), "cannot query") {
		t.Fatalf("managed agent inbox error = %v", err)
	}

	for index := range client.snapshot.Agents {
		if client.snapshot.Agents[index].Name != nil && *client.snapshot.Agents[index].Name == "worker" {
			client.snapshot.Agents[index].PaneID = "p-respawned"
		}
	}
	client.snapshot.Panes = append(client.snapshot.Panes, herdr.Pane{PaneID: "p-respawned"})
	manager.getenv = func(string) string { return "" }
	inbox, err := manager.MessageInbox(context.Background(), root, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0].ID != message.ID {
		t.Fatalf("worker history after respawn = %#v", inbox)
	}
	unknown, err := manager.MessageInbox(context.Background(), root, "unknown")
	if err != nil || len(unknown) != 0 {
		t.Fatalf("unknown transcript = %#v, %v", unknown, err)
	}
	manager.getenv = func(string) string { return "p-respawned" }
	if _, err := manager.ReplyMessage(context.Background(), root, message.ID, "late reply"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("respawn reply error = %v", err)
	}
}

func TestMessageCommandsRequireRunningSession(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	client.sessions[0].Running = false
	if _, err := manager.SendMessage(context.Background(), root, "worker", "body"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, stateDirectory, "logs", testSessionName, "messages.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("message log created for stopped session: %v", err)
	}
}

func TestMessagingUpdatesLegacyIgnoreWithoutOverwritingEntries(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	ignorePath := filepath.Join(root, stateDirectory, ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("session.json\nkeep-local/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SendMessage(context.Background(), root, "worker", "body"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "session.json\nkeep-local/\npreferences.json\nlogs/\ntmp/\nprofiles/generated/\n" {
		t.Fatalf(".gitignore = %q", contents)
	}
}

func TestMessageDeliveryHoldsSessionLockAgainstTeardown(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	acquired := make(chan func() error, 1)
	lockErrors := make(chan error, 1)
	client.promptHook = func() {
		go func() {
			unlock, err := lockSessionRecord(root)
			if err != nil {
				lockErrors <- err
				return
			}
			acquired <- unlock
		}()
		select {
		case unlock := <-acquired:
			_ = unlock()
			t.Error("session lock was released before prompt delivery completed")
		case err := <-lockErrors:
			t.Errorf("second session lock failed: %v", err)
		case <-time.After(25 * time.Millisecond):
		}
	}

	if _, err := manager.SendMessage(context.Background(), root, "worker", "body"); err != nil {
		t.Fatal(err)
	}
	select {
	case unlock := <-acquired:
		if err := unlock(); err != nil {
			t.Fatal(err)
		}
	case err := <-lockErrors:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("session lock was not released after message delivery")
	}
}

func newMessagingManager(t *testing.T) (*Manager, *fakeHerdr, string) {
	t.Helper()
	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{
		sessions: []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot: messagingSnapshot(),
	}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.getenv = func(string) string { return "" }
	return manager, client, root
}

func messagingSnapshot() herdr.Snapshot {
	orchestrator, worker := "orchestrator", "worker"
	kind := "codex"
	return herdr.Snapshot{
		Panes: []herdr.Pane{{PaneID: "p-control"}, {PaneID: "p-orchestrator"}, {PaneID: "p-worker"}},
		Agents: []herdr.Agent{
			{Name: &orchestrator, Agent: &kind, PaneID: "p-orchestrator"},
			{Name: &worker, Agent: &kind, PaneID: "p-worker"},
		},
	}
}
