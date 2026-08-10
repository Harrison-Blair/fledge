package watchproc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// A foreground Run comes up, writes the readiness marker and PID file, prints the
// readiness line to its captured output, and removes both marker files once it
// returns on context cancellation.
func TestRunForegroundAnnouncesReadyAndCleansUp(t *testing.T) {
	root := t.TempDir()
	store := messaging.New(root, testSession)
	if _, err := store.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{
		Name: "worker", PaneID: "p1", Harness: "codex", Caller: messaging.UserIdentity}); err != nil {
		t.Fatal(err)
	}

	client := &fakeHerdr{protocol: RequiredHerdrProtocol}
	client.setSnapshotPanes("p1")
	files := &fakeFiles{events: make(chan struct{}, 1), errs: make(chan error, 1)}
	var output bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			Root: root, Session: testSession, Herdr: client, Output: &output,
			WatchFile: func(string) (FileWatcher, error) { return files, nil },
			Subscribe: func(streamCtx context.Context, _ []string, onReady func(), _ func(herdr.Event)) error {
				onReady()
				<-streamCtx.Done()
				return streamCtx.Err()
			},
		})
	}()

	statePath := fsutil.TempSession(root, testSession)
	readyPath := filepath.Join(statePath, readyFilename)
	pidPath := filepath.Join(statePath, pidFilename)

	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("Run exited before writing the readiness marker: %v", err)
		case <-deadline:
			t.Fatal("readiness marker never appeared")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// The PID file is present while the foreground watcher runs.
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("PID file missing while running: %v", err)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	if !strings.Contains(output.String(), "dispatcher ready") {
		t.Fatalf("output = %q, want it to contain 'dispatcher ready'", output.String())
	}
	if _, err := os.Stat(readyPath); !os.IsNotExist(err) {
		t.Fatalf("readiness marker still present after return: err = %v", err)
	}
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("PID file still present after return: err = %v", err)
	}
}

// inspectDirectory rejects a symlink and a non-directory, and surfaces a stat
// failure for a missing path.
func TestInspectDirectoryRejectsNonDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, symlink); err != nil {
		t.Fatal(err)
	}
	regularFile := filepath.Join(dir, "file")
	if err := os.WriteFile(regularFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "absent")

	cases := []struct {
		name     string
		path     string
		wantFrag string
	}{
		{"symlink", symlink, "must not be a symlink"},
		{"regular file", regularFile, "is not a directory"},
		{"missing path", missing, "inspect watch directory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := inspectDirectory(tc.path)
			if err == nil || !strings.Contains(err.Error(), tc.wantFrag) {
				t.Fatalf("inspectDirectory(%s) = %v, want fragment %q", tc.name, err, tc.wantFrag)
			}
		})
	}
}

// ensureDirectories propagates inspectDirectory's symlink rejection: a symlinked
// leaf must not be accepted as a managed state directory even though MkdirAll
// follows it to a real directory.
func TestEnsureDirectoriesRejectsASymlinkedLeaf(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, symlink); err != nil {
		t.Fatal(err)
	}

	err := ensureDirectories(symlink)
	if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("ensureDirectories(symlink) = %v, want a 'must not be a symlink' error", err)
	}
}
