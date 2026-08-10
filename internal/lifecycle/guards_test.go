package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

// TestTaskProgressWakesNoOne locks the contract that recording progress appends a
// durable event but neither launches the dispatcher nor touches Herdr, and adds
// no wake — progress is a record, not a signal.
func TestTaskProgressWakesNoOne(t *testing.T) {
	t.Parallel()
	manager, client, root := newMessagingManager(t)
	launches := 0
	manager.watchLauncher = func(string) error { launches++; return nil }
	ctx := context.Background()

	task, err := manager.TaskAssign(ctx, root, "worker", "", "do the thing")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}

	store := messaging.New(root, testSessionName)
	before, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	launches = 0
	client.calls = nil

	progressed, err := manager.TaskProgress(ctx, root, task.ID, "halfway there")
	if err != nil {
		t.Fatalf("TaskProgress: %v", err)
	}
	if progressed.Detail != "halfway there" {
		t.Fatalf("progressed.Detail = %q, want the recorded detail", progressed.Detail)
	}
	if launches != 0 {
		t.Fatalf("dispatcher launches = %d, want 0 for progress", launches)
	}
	if len(client.calls) != 0 {
		t.Fatalf("Herdr calls = %v, want none for progress", client.calls)
	}
	after, err := store.PendingWakes()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("pending wakes changed from %d to %d; progress must wake no one", len(before), len(after))
	}
}

// TestSendMessageGuards covers the two pre-append rejections: the reserved "user"
// recipient and a self-directed send. Neither may append a message or launch the
// dispatcher.
func TestSendMessageGuards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cases := []struct {
		name      string
		pane      string
		recipient string
		wantErr   string
	}{
		{name: "user recipient is reserved", pane: "", recipient: "user", wantErr: "reserved for replies"},
		{name: "self send", pane: "p-worker", recipient: "worker", wantErr: "cannot send a message to yourself"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			manager, client, root := newMessagingManager(t)
			manager.getenv = paneEnv(tc.pane)
			launches := 0
			manager.watchLauncher = func(string) error { launches++; return nil }

			_, err := manager.SendMessage(ctx, root, tc.recipient, "hello there")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("SendMessage error = %v, want %q", err, tc.wantErr)
			}
			if launches != 0 {
				t.Fatalf("dispatcher launches = %d, want 0", launches)
			}
			if len(client.calls) != 0 {
				t.Fatalf("Herdr calls = %v, want none", client.calls)
			}
			store := messaging.New(root, testSessionName)
			wakes, err := store.PendingWakes()
			if err != nil {
				t.Fatal(err)
			}
			if len(wakes) != 0 {
				t.Fatalf("pending wakes = %#v, want none; a rejected send appends nothing", wakes)
			}
		})
	}
}

// TestWatchFailsBeforeRunning proves Watch refuses to hand off to the watcher when
// its preconditions fail: a missing session record, or an unhealthy Herdr.
func TestWatchFailsBeforeRunning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("no session record", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initTestProject(t, root)
		manager, _ := newTestManager(&fakeHerdr{}, &fakeConfirmer{})
		ran := false
		manager.watchRunner = func(context.Context, watchproc.Options) error { ran = true; return nil }

		err := manager.Watch(ctx, root, WatchOptions{})
		if err == nil || !strings.Contains(err.Error(), "run fledge start first") {
			t.Fatalf("Watch error = %v, want 'run fledge start first'", err)
		}
		if ran {
			t.Fatal("watchRunner ran despite the missing session record")
		}
	})

	t.Run("herdr check fails", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeTestRecord(t, root)
		checkErr := errors.New("herdr unreachable")
		manager, _ := newTestManager(&fakeHerdr{checkErr: checkErr}, &fakeConfirmer{})
		ran := false
		manager.watchRunner = func(context.Context, watchproc.Options) error { ran = true; return nil }

		err := manager.Watch(ctx, root, WatchOptions{})
		if !errors.Is(err, checkErr) {
			t.Fatalf("Watch error = %v, want the Herdr check error", err)
		}
		if ran {
			t.Fatal("watchRunner ran despite the failed Herdr check")
		}
	})
}

// TestTargetWorkspace exercises the workspace selection fallback: the focused
// workspace wins, an unfocused session falls back to the first workspace, and a
// session with no workspace at all is an error.
func TestTargetWorkspace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		snapshot herdr.Snapshot
		want     string
		wantErr  string
	}{
		{
			name:     "focused workspace is used",
			snapshot: herdr.Snapshot{FocusedWorkspaceID: "ws-focused", Workspaces: []herdr.Workspace{{WorkspaceID: "ws-first"}}},
			want:     "ws-focused",
		},
		{
			name:     "falls back to the first workspace",
			snapshot: herdr.Snapshot{Workspaces: []herdr.Workspace{{WorkspaceID: "ws-first"}, {WorkspaceID: "ws-second"}}},
			want:     "ws-first",
		},
		{
			name:     "no workspace is an error",
			snapshot: herdr.Snapshot{},
			wantErr:  "no workspace for the new agent tab",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := targetWorkspace(tc.snapshot)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("targetWorkspace error = %v, want %q", err, tc.wantErr)
				}
				if got != "" {
					t.Fatalf("targetWorkspace = %q, want empty on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("targetWorkspace: %v", err)
			}
			if got != tc.want {
				t.Fatalf("targetWorkspace = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMessageInboxScopesToIdentity confirms MessageInbox returns the transcript
// for the requested identity — only messages that identity sent or received — and
// echoes back the identity it resolved, defaulting to the user when unset.
func TestMessageInboxScopesToIdentity(t *testing.T) {
	t.Parallel()
	manager, _, root := newMessagingManager(t)
	ctx := context.Background()

	store := messaging.New(root, testSessionName)
	// One message involves the worker; one does not.
	workerMessage := deliverStoredMessage(t, store, "orchestrator", "worker", "p-worker", "for the worker")
	otherMessage := deliverStoredMessage(t, store, "user", "orchestrator", "p-orchestrator", "for the orchestrator")

	messages, identity, err := manager.MessageInbox(ctx, root, "worker")
	if err != nil {
		t.Fatalf("MessageInbox: %v", err)
	}
	if identity != "worker" {
		t.Fatalf("resolved identity = %q, want worker", identity)
	}
	if len(messages) != 1 || messages[0].ID != workerMessage.ID {
		t.Fatalf("worker inbox = %#v, want only %s (never %s)", messages, workerMessage.ID, otherMessage.ID)
	}

	// An empty identity defaults to the user's own transcript.
	_, defaulted, err := manager.MessageInbox(ctx, root, "")
	if err != nil {
		t.Fatalf("MessageInbox default: %v", err)
	}
	if defaulted != userIdentity {
		t.Fatalf("default identity = %q, want user", defaulted)
	}
}
