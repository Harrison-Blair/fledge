package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/statedir"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

func TestWatchPassesTheActiveSessionAndOptionsToTheRuntime(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	manager, _ := newTestManager(&fakeHerdr{}, &fakeConfirmer{})
	var got watchproc.Options
	manager.watchRunner = func(_ context.Context, options watchproc.Options) error {
		got = options
		return nil
	}

	if err := manager.Watch(context.Background(), root, WatchOptions{Daemon: true}); err != nil {
		t.Fatal(err)
	}
	if got.Root != root || got.Session != testSessionName || !got.Config.Enabled || !got.Daemon || got.Herdr == nil || got.Deliver == nil || got.Output == nil {
		t.Fatalf("watch runtime options = %#v", got)
	}
}

func TestWatchRunsTheConcreteRuntime(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: false}}}
	var output bytes.Buffer
	manager := NewManager(client, &fakeConfirmer{}, nil, &output)

	if err := manager.Watch(context.Background(), root, WatchOptions{}); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(statedir.Session(root, testSessionName), watchproc.LogFilename)
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "watcher exiting") || !strings.Contains(output.String(), "watcher exiting") {
		t.Fatalf("watch log/output = %q / %q, want foreground decision in both", contents, output.String())
	}
}

func TestWatchDeliveryUsesWatcherMessaging(t *testing.T) {
	t.Parallel()

	manager, _, root := newMessagingManager(t)
	var deliveredID string
	manager.watchRunner = func(ctx context.Context, options watchproc.Options) error {
		var err error
		deliveredID, err = options.Deliver(ctx, "automated wake")
		return err
	}
	if err := manager.Watch(context.Background(), root, WatchOptions{}); err != nil {
		t.Fatal(err)
	}
	message, err := messaging.New(root, testSessionName).Get(deliveredID)
	if err != nil {
		t.Fatal(err)
	}
	if message.Sender != "watcher" || message.Recipient != "orchestrator" || message.Body != "automated wake" || message.Status != messaging.StatusDelivered {
		t.Fatalf("watch delivery = %#v", message)
	}
}

func TestWatchDisabledExitsWithoutAnActiveSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	if err := os.WriteFile(filepath.Join(statedir.Root(root), "watch.json"), []byte(`{"enabled":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	manager, _ := newTestManager(&fakeHerdr{}, &fakeConfirmer{})
	manager.watchRunner = func(context.Context, watchproc.Options) error {
		t.Fatal("disabled watcher invoked the runtime")
		return nil
	}
	if err := manager.Watch(context.Background(), root, WatchOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestWatchRequiresAnActiveSessionWhenEnabled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	manager, _ := newTestManager(&fakeHerdr{}, &fakeConfirmer{})
	if err := manager.Watch(context.Background(), root, WatchOptions{}); err == nil || !strings.Contains(err.Error(), "run fledge start") {
		t.Fatalf("Watch() error = %v", err)
	}
}

func TestStopLeavesSessionAndTemporaryStateWhenWatcherWillNotStop(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	marker := filepath.Join(statedir.TempSession(root, testSessionName), "keep")
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
	manager, _ := newTestManager(client, &fakeConfirmer{answer: true})
	want := errors.New("watcher stayed alive")
	manager.watchStopper = func(root, session string) error { return want }

	err := manager.Stop(context.Background(), root)
	if !errors.Is(err, want) {
		t.Fatalf("Stop() error = %v, want %v", err, want)
	}
	if len(client.stopCalls) != 0 || len(client.deleteCalls) != 0 {
		t.Fatalf("Herdr stop/delete calls = %v/%v, want none before watcher teardown", client.stopCalls, client.deleteCalls)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("temporary marker after failed watcher stop = %v, want retained", err)
	}
	if _, found, err := readRecord(root); err != nil || !found {
		t.Fatalf("session record after failed watcher stop = found %v, error %v", found, err)
	}
}
