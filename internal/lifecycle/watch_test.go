package lifecycle

import (
	"context"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

func TestWatchPassesTheOptionsToTheWatcher(t *testing.T) {
	t.Parallel()

	client := &fakeHerdr{}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	root := t.TempDir()
	writeTestRecord(t, root)
	var seen watchproc.Options
	manager.watchRunner = func(_ context.Context, options watchproc.Options) error {
		seen = options
		return nil
	}
	if err := manager.Watch(context.Background(), root, WatchOptions{Daemon: true}); err != nil {
		t.Fatal(err)
	}
	if !seen.Daemon {
		t.Fatalf("watcher options = %#v, want Daemon set", seen)
	}
}
