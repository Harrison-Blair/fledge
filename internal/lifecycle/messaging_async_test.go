package lifecycle

import (
	"context"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

func TestSendMessageQueuesDurableAlwaysWakeWithoutHerdrCalls(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	message, err := manager.SendMessage(context.Background(), root, "worker", "Review this")
	if err != nil {
		t.Fatal(err)
	}
	if message.Status != messaging.StatusPending || message.RecipientPane != "p-worker" {
		t.Fatalf("message = %#v", message)
	}
	if len(client.promptCalls) != 0 || client.listCalls != 0 {
		t.Fatalf("message command made Herdr calls: %#v", client.calls)
	}
	wakes, err := messaging.New(root, testSessionName).PendingWakes()
	if err != nil || len(wakes) != 1 || wakes[0].Kind != "message" || wakes[0].Recipient != "worker" {
		t.Fatalf("pending wakes = %#v, %v", wakes, err)
	}
}

func newMessagingManager(t *testing.T) (*Manager, *fakeHerdr, string) {
	t.Helper()
	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.getenv = func(string) string { return "" }
	store := messaging.New(root, testSessionName)
	sessionID, err := store.Initialize()
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceRecordSessionBinding(root, testSessionName, sessionID); err != nil {
		t.Fatal(err)
	}
	for _, params := range []messaging.RegisterParams{
		{Name: "orchestrator", PaneID: "p-orchestrator", Harness: "codex", Caller: "user", CanDelegate: true},
		{Name: "worker", PaneID: "p-worker", Harness: "codex", Caller: "orchestrator"},
	} {
		if _, _, err := store.RegisterAgent(params); err != nil {
			t.Fatal(err)
		}
	}
	return manager, client, root
}
