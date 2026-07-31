package messaging

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/buildinfo"
)

func testHeader(root string) RunHeader {
	return RunHeader{
		Fledge: buildinfo.Current(), Herdr: "herdr test", Protocol: 17,
		ProjectRoot: root, Session: "test", StartedAt: time.Now().UTC(),
	}
}

func TestStoreSerializesConcurrentWritersAndReconstructs(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	runID, err := store.StartRun(testHeader(root))
	if err != nil {
		t.Fatal(err)
	}
	const count = 48
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := NewID("msg_")
			if err == nil {
				_, err = store.Append(runID, Event{
					Type: EventMessageCreated, MessageID: id,
					Sender: "user", Recipient: "worker", Body: "hello",
				})
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	run, err := store.ReadRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Messages) != count || run.LastSequence != count+1 {
		t.Fatalf("messages=%d sequence=%d", len(run.Messages), run.LastSequence)
	}
	seen := map[string]bool{}
	for _, message := range run.Messages {
		if seen[message.ID] {
			t.Fatalf("duplicate ID %s", message.ID)
		}
		seen[message.ID] = true
	}
	if mode := fileMode(t, store.Dir); mode != 0o700 {
		t.Fatalf("directory mode = %o", mode)
	}
	if mode := fileMode(t, store.runPath(runID)); mode != 0o600 {
		t.Fatalf("log mode = %o", mode)
	}
}

func TestStoreRepairsOnlyUnterminatedCrashTail(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	runID, err := store.StartRun(testHeader(root))
	if err != nil {
		t.Fatal(err)
	}
	path := store.runPath(runID)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"schema_version":1`); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	run, err := store.ReadRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.LastSequence != 1 {
		t.Fatalf("last sequence = %d", run.LastSequence)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if data[len(data)-1] != '\n' {
		t.Fatal("repaired log does not end in newline")
	}
}

func TestStoreRejectsTerminatedMalformedRecord(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	runID, err := store.StartRun(testHeader(root))
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(store.runPath(runID), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString("{bad}\n")
	_ = file.Close()
	if _, err := store.ReadRun(runID); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("error = %v", err)
	}
}

func TestReconstructDeliveryReplyAndClosure(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	runID, err := store.StartRun(testHeader(root))
	if err != nil {
		t.Fatal(err)
	}
	events := []Event{
		{Type: EventMessageCreated, MessageID: "msg_one", Sender: "user", Recipient: "worker", Body: "one"},
		{Type: EventDeliveryAttempted, MessageID: "msg_one", AttemptID: "try_one", ActivationID: "act_one"},
		{Type: EventDeliveryInjected, MessageID: "msg_one", AttemptID: "try_one", ActivationID: "act_one"},
		{Type: EventMessageReplied, MessageID: "msg_two", ReplyTo: "msg_one", Sender: "worker", Recipient: "user", Actor: "worker", Body: "two"},
		{Type: EventRunClosed, Reason: "test"},
	}
	for _, event := range events {
		if _, err := store.Append(runID, event); err != nil {
			t.Fatal(err)
		}
	}
	run, err := store.ReadRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Active || run.Messages[0].Status != StatusAcknowledged ||
		run.Messages[0].Acknowledgement.ViaReply != "msg_two" || run.Messages[1].ActiveRun {
		t.Fatalf("unexpected reconstruction: %#v", run)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}
