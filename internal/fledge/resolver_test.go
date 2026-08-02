package fledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrtest"
	"github.com/Harrison-Blair/fledge/internal/project"
)

func TestMatchingWorkspaceUsesOnlyCanonicalWorkspaceMetadata(t *testing.T) {
	root := t.TempDir()
	paneCWD := root
	snapshot := herdr.Snapshot{
		Workspaces: []herdr.WorkspaceInfo{
			{WorkspaceID: "wrong", Number: 0, Worktree: &herdr.WorkspaceWorktreeInfo{CheckoutPath: t.TempDir()}},
			{WorkspaceID: "low", Number: 2, Worktree: &herdr.WorkspaceWorktreeInfo{RepoRoot: root}},
			{WorkspaceID: "focused", Number: 9, Focused: true, Worktree: &herdr.WorkspaceWorktreeInfo{CheckoutPath: root}},
		},
		Panes: []herdr.PaneInfo{{PaneID: "pane-only", CWD: &paneCWD}},
	}
	got, found, err := matchingWorkspace(snapshot, root)
	if err != nil || !found || got.WorkspaceID != "focused" {
		t.Fatalf("match = %#v, %t, %v", got, found, err)
	}

	snapshot.Workspaces = nil
	if _, found, err := matchingWorkspace(snapshot, root); err != nil || found {
		t.Fatalf("pane cwd incorrectly matched: found=%t err=%v", found, err)
	}
}

func TestResolveSessionAlwaysDerivesNameWithoutListingSessions(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(t.TempDir(), "invocations")
	binaryPath := herdrtest.WriteBinary(t, t.TempDir(), herdrtest.Options{
		InvocationLog: log,
		Version:       herdrtest.VersionOutput,
		UnknownExit:   77,
	})

	resolved, err := ResolveSession(t.Context(), root, herdr.Binary{Path: binaryPath})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Session != project.SessionName(root) || resolved.Source != "derived" || resolved.WorkspaceID != "" {
		t.Fatalf("unexpected resolution: %#v", resolved)
	}
	invocations, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(invocations), "session list") {
		t.Fatalf("resolver inspected Herdr sessions: %s", invocations)
	}
}
