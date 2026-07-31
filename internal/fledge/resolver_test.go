package fledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
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
	methods := make([]string, 0, len(herdr.RequiredMethods))
	for _, method := range herdr.RequiredMethods {
		methods = append(methods, fmt.Sprintf(`{"method":{"const":%s}}`, strconv.Quote(method)))
	}
	schema := fmt.Sprintf(`{"protocol":17,"requests":[%s]}`, strings.Join(methods, ","))
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %s
if [ "$1" = "--version" ]; then
  echo "herdr 0.7.5"
elif [ "$1" = "api" ] && [ "$2" = "schema" ]; then
  printf '%%s\n' %s
else
  exit 77
fi
`, strconv.Quote(log), strconv.Quote(schema))
	binaryPath := filepath.Join(t.TempDir(), "herdr-fake")
	if err := os.WriteFile(binaryPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

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
