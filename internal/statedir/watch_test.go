package statedir

import (
	"path/filepath"
	"testing"
)

func TestWatchPathsNestBeneathSessionTemp(t *testing.T) {
	t.Parallel()

	root := filepath.Join("home", "project")
	session := "fledge-demo-0a1b2c3d"
	if got, want := WatchSession(root, session), filepath.Join(root, ".fledge", "tmp", session, "watch"); got != want {
		t.Errorf("WatchSession() = %q, want %q", got, want)
	}
	if got, want := StatusDir(root, session), filepath.Join(root, ".fledge", "tmp", session, "status"); got != want {
		t.Errorf("StatusDir() = %q, want %q", got, want)
	}
	if got, want := StatusFile(root, session, "reviewer"), filepath.Join(root, ".fledge", "tmp", session, "status", "reviewer.status"); got != want {
		t.Errorf("StatusFile() = %q, want %q", got, want)
	}
}
