package messaging

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

const (
	testSession  = "fledge-test-0a1b2c3d"
	otherSession = "fledge-other-1b2c3d4e"
)

func TestValidateBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "text", body: "hello \u4e16\u754c", want: true},
		{name: "blank", body: " \n\t", want: false},
		{name: "invalid UTF-8", body: string([]byte{0xff}), want: false},
		{name: "at limit", body: strings.Repeat("x", MaxBodyBytes), want: true},
		{name: "over limit", body: strings.Repeat("x", MaxBodyBytes+1), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateBody(test.body)
			if (err == nil) != test.want {
				t.Fatalf("ValidateBody() error = %v, want valid %v", err, test.want)
			}
			if err != nil && !errors.Is(err, ErrInvalidBody) {
				t.Fatalf("ValidateBody() error = %v, want ErrInvalidBody", err)
			}
		})
	}
}

func TestLifecycleInitializeEnsureAndReset(t *testing.T) {
	root := t.TempDir()
	store := New(root, testSession)

	firstSessionID, err := store.Ensure()
	if err != nil {
		t.Fatal(err)
	}
	message := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "first", RecipientPane: "%1"})

	preservedID, err := store.Ensure()
	if err != nil {
		t.Fatal(err)
	}
	if preservedID != firstSessionID {
		t.Fatalf("Ensure replaced session ID: got %q, want %q", preservedID, firstSessionID)
	}
	if _, err := store.Get(message.ID); err != nil {
		t.Fatalf("Ensure did not preserve message: %v", err)
	}

	resetID, err := store.Initialize()
	if err != nil {
		t.Fatal(err)
	}
	if resetID == firstSessionID {
		t.Fatal("Initialize reused session ID")
	}
	if _, err := store.Get(message.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old message after Initialize error = %v, want ErrNotFound", err)
	}
	if _, err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveLockKeepsLogAndRemoveAllDeletesSessionDirectory(t *testing.T) {
	store := initializedStore(t)

	if err := store.RemoveLock(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.logPath()); err != nil {
		t.Fatalf("log after RemoveLock: %v; want preserved", err)
	}
	if _, err := os.Stat(store.lockPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock after RemoveLock error = %v, want not exist", err)
	}

	if err := store.RemoveAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.statePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session directory after RemoveAll error = %v, want not exist", err)
	}
	if _, err := os.Stat(store.tempPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary session directory after RemoveAll error = %v, want not exist", err)
	}
	if _, err := os.Stat(fsutil.Logs(store.root)); err != nil {
		t.Fatalf("logs directory after RemoveAll: %v; want preserved", err)
	}
}

func TestSessionsKeepIndependentLogs(t *testing.T) {
	root := t.TempDir()
	first := New(root, testSession)
	if _, err := first.Initialize(); err != nil {
		t.Fatal(err)
	}
	kept := mustCreate(t, first, CreateParams{Sender: "user", Recipient: "alice", Body: "first", RecipientPane: "%1"})

	second := New(root, otherSession)
	if _, err := second.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Get(kept.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other session Get error = %v, want ErrNotFound", err)
	}
	if err := second.RemoveAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Get(kept.ID); err != nil {
		t.Fatalf("first session message after other session RemoveAll: %v", err)
	}
}

func TestEnsureRejectsLogFromARenamedSession(t *testing.T) {
	root := t.TempDir()
	initialized := New(root, testSession)
	if _, err := initialized.Initialize(); err != nil {
		t.Fatal(err)
	}
	moved := New(root, otherSession)
	if err := os.Rename(initialized.statePath(), moved.statePath()); err != nil {
		t.Fatal(err)
	}
	if _, err := moved.Ensure(); !errors.Is(err, ErrSessionMismatch) {
		t.Fatalf("Ensure error = %v, want ErrSessionMismatch", err)
	}
}

func TestInvalidSessionNameFailsBeforeCreatingDirectories(t *testing.T) {
	for _, session := range []string{"", "   ", ".", "..", "herdr-one", "fledge-demo-0a1b2c3d/child"} {
		t.Run(session, func(t *testing.T) {
			root := t.TempDir()
			if _, err := New(root, session).Initialize(); err == nil {
				t.Fatalf("Initialize() error = nil, want invalid session name error")
			}
			if _, err := os.Stat(fsutil.Root(root)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("state directory error = %v, want not exist", err)
			}
		})
	}
}

func TestEnsureInitializesEmptyLog(t *testing.T) {
	root := t.TempDir()
	store := New(root, testSession)
	if err := os.MkdirAll(store.statePath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.logPath(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	if messages, err := store.Inbox("user"); err != nil || len(messages) != 0 {
		t.Fatalf("Inbox() = %v, %v; want empty", messages, err)
	}
}

func TestStatusReconstruction(t *testing.T) {
	store := initializedStore(t)

	pending := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "pending", RecipientPane: "%1"})
	if pending.Status != StatusPending {
		t.Fatalf("Create status = %s, want pending", pending.Status)
	}
	// Driving the message's wake to uncertain then delivered projects those
	// statuses onto the message it carries.
	if _, err := store.RecordWakeAttempt("w-" + pending.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(pending.ID); err != nil || got.Status != StatusUncertain {
		t.Fatalf("after wake attempt = %#v, %v", got, err)
	}
	if _, err := store.RecordWakeOutcome("w-"+pending.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(pending.ID); err != nil || got.Status != StatusDelivered {
		t.Fatalf("after wake success = %#v, %v", got, err)
	}

	failed := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "bob", Body: "failed", RecipientPane: "%2"})
	if _, err := store.RecordWakeAttempt("w-" + failed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordWakeOutcome("w-"+failed.ID, false, "Herdr refused"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(failed.ID); err != nil || got.Status != StatusFailed {
		t.Fatalf("after wake failure = %#v, %v", got, err)
	}

	got, err := store.Inbox("user")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Status != StatusDelivered || got[1].Status != StatusFailed {
		t.Fatalf("reconstructed messages = %#v", got)
	}
}

func TestInterruptedAttemptReconstructsUncertainAndAppearsInTranscript(t *testing.T) {
	store := initializedStore(t)
	message := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "hello", RecipientPane: "%1"})
	if _, err := store.RecordWakeAttempt("w-" + message.ID); err != nil {
		t.Fatal(err)
	}

	reopened := New(store.root, store.session)
	got, err := reopened.Get(message.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusUncertain {
		t.Fatalf("status = %s, want uncertain", got.Status)
	}
	inbox, err := reopened.Inbox("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 || inbox[0].ID != message.ID {
		t.Fatalf("Inbox() = %#v", inbox)
	}
}

func TestReplyPreservesOriginalAndUserReplyIsDelivered(t *testing.T) {
	store := initializedStore(t)
	original := deliveredMessage(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "question", RecipientPane: "%1"})
	before := lineCount(t, store.logPath())

	reply, err := store.Reply(original.ID, "alice", "%1", "answer", "")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Sender != "alice" || reply.Recipient != "user" || reply.ReplyTo != original.ID || reply.Status != StatusDelivered {
		t.Fatalf("reply = %#v", reply)
	}
	gotOriginal, err := store.Get(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotOriginal != original {
		t.Fatalf("reply changed original:\n%#v\n%#v", original, gotOriginal)
	}
	if got := lineCount(t, store.logPath()); got != before+1 {
		t.Fatalf("reply transaction appended %d events, want 1", got-before)
	}
	inbox, err := store.Inbox("user")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 2 || inbox[0].ID != original.ID || inbox[1].ID != reply.ID {
		t.Fatalf("user Inbox() = %#v", inbox)
	}
	contents, err := os.ReadFile(store.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), `"type":"acknowledged"`) {
		t.Fatal("Reply wrote an acknowledgement event")
	}
}

// A reply to an uncertain original is allowed and, unlike the retired
// acknowledgement path, leaves the original's projected status untouched.
func TestReplyToUncertainOriginalLeavesItUncertain(t *testing.T) {
	store := initializedStore(t)
	original := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "question", RecipientPane: "%1"})
	if _, err := store.RecordWakeAttempt("w-" + original.ID); err != nil {
		t.Fatal(err)
	}
	before := lineCount(t, store.logPath())

	if _, err := store.Reply(original.ID, "alice", "%1", "answer", ""); err != nil {
		t.Fatal(err)
	}
	got, err := New(store.root, store.session).Get(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusUncertain {
		t.Fatalf("original after reply = %#v, want uncertain", got)
	}
	// A reply to the local user appends only the reply_created event.
	if got := lineCount(t, store.logPath()); got != before+1 {
		t.Fatalf("reply transaction appended %d events, want 1", got-before)
	}
}

func TestCrashedReplyRecordIsDiscardedWithoutChangingOriginal(t *testing.T) {
	store := initializedStore(t)
	original := deliveredMessage(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "question", RecipientPane: "%1"})
	reply, err := store.Reply(original.ID, "alice", "%1", "answer", "")
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a crash after the reply JSON was written but before its newline
	// record terminator became durable. Repair must discard the whole semantic
	// event, leaving the reply invisible.
	info, err := os.Stat(store.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(store.logPath(), info.Size()-1); err != nil {
		t.Fatal(err)
	}

	reopened := New(store.root, store.session)
	gotOriginal, err := reopened.Get(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotOriginal != original {
		t.Fatalf("original changed after reply-tail repair:\n%#v\n%#v", original, gotOriginal)
	}
	if _, err := reopened.Get(reply.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reply after tail repair error = %v, want ErrNotFound", err)
	}
}

func TestAgentReplyRemainsPendingForHerdrDelivery(t *testing.T) {
	store := initializedStore(t)
	original := deliveredMessage(t, store, CreateParams{Sender: "alice", Recipient: "bob", Body: "question", RecipientPane: "%2"})
	reply, err := store.Reply(original.ID, "bob", "%2", "answer", "%1")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Recipient != "alice" || reply.RecipientPane != "%1" || reply.Status != StatusPending {
		t.Fatalf("reply = %#v", reply)
	}
}

func TestReplyAuthorizationFailureAppendsNothing(t *testing.T) {
	store := initializedStore(t)
	original := deliveredMessage(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "question", RecipientPane: "%1"})
	before := lineCount(t, store.logPath())
	if _, err := store.Reply(original.ID, "alice", "%old", "answer", ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Reply error = %v, want ErrUnauthorized", err)
	}
	if got := lineCount(t, store.logPath()); got != before {
		t.Fatalf("failed reply appended %d events", got-before)
	}
}

func TestInboxReturnsCompleteChronologicalTranscriptIndependentOfPane(t *testing.T) {
	store := initializedStore(t)
	first := deliveredMessage(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "question", RecipientPane: "%old"})
	reply, err := store.Reply(first.ID, "alice", "%old", "answer", "")
	if err != nil {
		t.Fatal(err)
	}
	failed := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "bob", Body: "failed", RecipientPane: "%2"})
	if _, err := store.RecordWakeAttempt("w-" + failed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordWakeOutcome("w-"+failed.ID, false, "rejected"); err != nil {
		t.Fatal(err)
	}
	uncertain := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "uncertain", RecipientPane: "%new"})
	if _, err := store.RecordWakeAttempt("w-" + uncertain.ID); err != nil {
		t.Fatal(err)
	}

	userTranscript, err := store.Inbox("user")
	if err != nil {
		t.Fatal(err)
	}
	if len(userTranscript) != 4 || userTranscript[0].ID != first.ID || userTranscript[1].ID != reply.ID || userTranscript[2].ID != failed.ID || userTranscript[3].ID != uncertain.ID {
		t.Fatalf("user transcript = %#v", userTranscript)
	}
	aliceTranscript, err := store.Inbox("alice")
	if err != nil || len(aliceTranscript) != 3 || aliceTranscript[0].ID != first.ID || aliceTranscript[1].ID != reply.ID || aliceTranscript[2].ID != uncertain.ID {
		t.Fatalf("alice transcript = %#v, %v", aliceTranscript, err)
	}
	unknown, err := store.Inbox("unknown")
	if err != nil || len(unknown) != 0 {
		t.Fatalf("unknown transcript = %#v, %v", unknown, err)
	}
}

func TestConcurrentAppendsAreSerialized(t *testing.T) {
	root := t.TempDir()
	if _, err := New(root, testSession).Initialize(); err != nil {
		t.Fatal(err)
	}
	const writers = 64
	var wait sync.WaitGroup
	errorsChannel := make(chan error, writers)
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := New(root, testSession).Create(CreateParams{
				Sender: "user", Recipient: fmt.Sprintf("agent-%d", index),
				Body: fmt.Sprintf("message-%d", index), RecipientPane: fmt.Sprintf("%%%d", index),
			})
			errorsChannel <- err
		}(index)
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	messages, err := New(root, testSession).Inbox("user")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != writers {
		t.Fatalf("Inbox returned %d messages, want %d", len(messages), writers)
	}
	seen := make(map[string]bool, writers)
	for _, message := range messages {
		if seen[message.ID] {
			t.Fatalf("duplicate message ID %q", message.ID)
		}
		seen[message.ID] = true
	}
}

func TestRemoveAllCannotDestroyAnAppendCommittedUnderTheLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot delete an open lock file, so RemoveAll must release it first")
	}
	for iteration := 0; iteration < 16; iteration++ {
		root := t.TempDir()
		store := New(root, testSession)
		if _, err := store.Initialize(); err != nil {
			t.Fatal(err)
		}
		state, err := store.loadState()
		if err != nil {
			t.Fatal(err)
		}
		// Both racers queue behind a lock this test holds, so RemoveAll can win it
		// first and hand the waiting appender any window it leaves open.
		gate, err := store.acquireLock(store.lockPath())
		if err != nil {
			t.Fatal(err)
		}

		removed := make(chan struct{})
		var removeErr error
		go func() {
			removeErr = New(root, testSession).RemoveAll()
			close(removed)
		}()

		appended := make(chan struct{})
		var failure string
		go func() {
			defer close(appended)
			appender := New(root, testSession)
			unlock, err := appender.acquireLock(appender.lockPath())
			if err != nil {
				return // RemoveAll already deleted the folder holding the lock.
			}
			defer func() { _ = unlock() }()
			if _, err := os.Stat(appender.logPath()); err != nil {
				return // RemoveAll already deleted the log.
			}
			e := event{
				Version: eventVersion, Type: eventMessageCreated, At: time.Now().UTC(), SessionID: state.sessionID,
				MessageID: "committed-under-the-lock", Sender: "user", Recipient: "alice", Body: "committed", RecipientPane: "%1",
			}
			if err := appender.appendEvents([]event{e}); err != nil {
				failure = fmt.Sprintf("log vanished mid-append while the lock was held: %v", err)
				return
			}
			// The append is committed and this appender still holds the lock, so
			// nothing may delete the log before it releases.
			select {
			case <-removed:
				if _, err := os.Stat(appender.logPath()); err != nil {
					failure = fmt.Sprintf("RemoveAll destroyed an append committed under the lock: %v", err)
				}
			case <-time.After(100 * time.Millisecond):
			}
		}()

		if err := gate(); err != nil {
			t.Fatal(err)
		}
		<-appended
		<-removed
		if failure != "" {
			t.Fatalf("iteration %d: %s", iteration, failure)
		}
		if removeErr != nil {
			t.Fatalf("iteration %d: %v", iteration, removeErr)
		}
	}
}

func TestPermissionsAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are not available on Windows")
	}
	store := initializedStore(t)
	// fsutil.Root is deliberately absent: it is the user-facing .fledge folder,
	// owned by project.Init. See TestMessagingPreservesTheUserFacingStateFolder.
	for _, path := range []string{
		fsutil.Logs(store.root),
		store.statePath(), fsutil.Temp(store.root), store.tempPath(),
		store.logPath(), store.lockPath(),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s permission = %04o, want %04o", path, got, want)
		}
	}
}

func TestMessagingPreservesTheUserFacingStateFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are not available on Windows")
	}
	root := t.TempDir()
	// project.Init creates .fledge for the user at 0755; messaging shares the
	// folder and must not narrow it to its own private 0700.
	if err := os.MkdirAll(fsutil.Root(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fsutil.Root(root), 0o755); err != nil {
		t.Fatal(err)
	}

	store := New(root, testSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "hello", RecipientPane: "%1"})

	info, err := os.Stat(fsutil.Root(root))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("%s permission = %04o, want 0755", fsutil.Root(root), got)
	}
}

func TestRepairsOnlyUnterminatedTail(t *testing.T) {
	store := initializedStore(t)
	message := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "complete", RecipientPane: "%1"})
	partial := `{"version":1,"type":"message_created"`
	appendRaw(t, store.logPath(), partial)

	messages, err := store.Inbox("user")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != message.ID {
		t.Fatalf("Inbox after tail repair = %#v", messages)
	}
	contents, err := os.ReadFile(store.logPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) == 0 || contents[len(contents)-1] != '\n' || strings.HasSuffix(string(contents), partial) {
		t.Fatalf("tail was not repaired: %q", contents)
	}
}

func TestRejectsCompletedMalformedOrInvalidEvents(t *testing.T) {
	tests := []string{
		"not-json\n",
		`{"version":1,"type":"invented","at":"2026-01-01T00:00:00Z","session_id":"x"}` + "\n",
		`{"version":1,"type":"session_start","at":"2026-01-01T00:00:00Z","session_id":"second","session":"s"}` + "\n",
		`{"version":1,"type":"wake_outcome","at":"2026-01-01T00:00:00Z","session_id":"x","wake_id":"w-1"}` + "\n",
	}
	for index, invalid := range tests {
		t.Run(fmt.Sprintf("case-%d", index), func(t *testing.T) {
			store := initializedStore(t)
			appendRaw(t, store.logPath(), invalid)
			if _, err := store.Inbox("user"); !errors.Is(err, ErrCorruptLog) {
				t.Fatalf("Inbox error = %v, want ErrCorruptLog", err)
			}
			contents, err := os.ReadFile(store.logPath())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(string(contents), invalid) {
				t.Fatal("completed corrupt record was repaired or removed")
			}
		})
	}
}

func TestClockAndIDOptions(t *testing.T) {
	root := t.TempDir()
	ids := []string{"session-id", "message-id"}
	index := 0
	instant := time.Date(2026, 8, 3, 12, 0, 0, 0, time.FixedZone("local", -4*60*60))
	store := New(root, testSession,
		WithClock(func() time.Time { return instant }),
		WithIDGenerator(func() (string, error) {
			id := ids[index]
			index++
			return id, nil
		}),
	)
	sessionID, err := store.Initialize()
	if err != nil {
		t.Fatal(err)
	}
	message := mustCreate(t, store, CreateParams{Sender: "user", Recipient: "alice", Body: "hello", RecipientPane: "%1"})
	if sessionID != "session-id" || message.ID != "message-id" {
		t.Fatalf("IDs = %q, %q", sessionID, message.ID)
	}
	if message.CreatedAt.Location() != time.UTC || !message.CreatedAt.Equal(instant) {
		t.Fatalf("CreatedAt = %v, want UTC %v", message.CreatedAt, instant)
	}
}

func initializedStore(t *testing.T) *Store {
	t.Helper()
	store := New(t.TempDir(), testSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	return store
}

func mustCreate(t *testing.T, store *Store, params CreateParams) Message {
	t.Helper()
	message, err := store.Create(params)
	if err != nil {
		t.Fatal(err)
	}
	return message
}

// deliveredMessage creates a message to an agent and drives its wake to a
// delivered outcome, which projects the delivered status onto the message.
func deliveredMessage(t *testing.T, store *Store, params CreateParams) Message {
	t.Helper()
	message := mustCreate(t, store, params)
	wakeID := "w-" + message.ID
	if _, err := store.RecordWakeAttempt(wakeID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordWakeOutcome(wakeID, true, ""); err != nil {
		t.Fatal(err)
	}
	delivered, err := store.Get(message.ID)
	if err != nil {
		t.Fatal(err)
	}
	return delivered
}

func appendRaw(t *testing.T, path, contents string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(contents); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func lineCount(t *testing.T, path string) int {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(contents), "\n")
}
