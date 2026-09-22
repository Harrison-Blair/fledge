package worktree

import (
	"context"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

type call = herdrscript.Call

func listing(root string) herdr.WorktreeListResult {
	return herdr.WorktreeListResult{Type: "worktree_list", Source: struct {
		RepoRoot string `json:"repo_root"`
	}{RepoRoot: root}, Worktrees: []herdr.Worktree{}}
}
func created(kind, path string) herdr.CreatedResult {
	p := herdrscript.Pane("w2:p1", "w2", "w2:t1")
	open := false
	r := herdr.CreatedResult{Type: kind, Workspace: herdr.Workspace{ID: "w2"}, Tab: herdr.Tab{ID: "w2:t1", WorkspaceID: "w2"}, RootPane: p, Worktree: herdr.Worktree{Path: path}}
	if kind == "worktree_opened" {
		r.AlreadyOpen = &open
	}
	return r
}

// Herdr rejects create/open from a linked checkout (linked_worktree_source),
// while worktree.list resolves a linked cwd through to the primary checkout.
func TestLinkedCwdSourceUsesListedPrimaryRoot(t *testing.T) {
	src := Source{Cwd: "/repo/.fledge/worktrees/linked"}
	api := herdrscript.Client(t,
		call{Method: "worktree.list", Params: map[string]any{"cwd": src.Cwd}, Result: listing("/repo")},
		call{Method: "worktree.create", Params: map[string]any{"cwd": "/repo", "branch": "topic", "base": "main", "path": "/repo/.fledge/worktrees/topic", "focus": false}, Result: created("worktree_created", "/repo/.fledge/worktrees/topic")},
		call{Method: "worktree.open", Params: map[string]any{"cwd": "/repo", "path": "/elsewhere", "focus": false}, Result: created("worktree_opened", "/elsewhere")},
	)
	l, err := List(context.Background(), api, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Create(context.Background(), api, src, l, "topic", "main", "/repo/.fledge/worktrees/topic"); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(context.Background(), api, src, l, "/elsewhere"); err != nil {
		t.Fatal(err)
	}
}
func TestWorkspaceSourceIsSentUnchanged(t *testing.T) {
	src := Source{WorkspaceID: "w1"}
	api := herdrscript.Client(t,
		call{Method: "worktree.list", Params: map[string]any{"workspace_id": "w1"}, Result: listing("/repo")},
		call{Method: "worktree.create", Params: map[string]any{"workspace_id": "w1", "branch": "topic", "path": "/p", "focus": false}, Result: created("worktree_created", "/p")},
	)
	l, err := List(context.Background(), api, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Create(context.Background(), api, src, l, "topic", "", "/p"); err != nil {
		t.Fatal(err)
	}
}
func TestIncompleteResultsRejected(t *testing.T) {
	src := Source{Cwd: "/repo"}
	incomplete := listing("")
	if _, err := List(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: incomplete}), src); err == nil {
		t.Fatal("accepted listing without repo root")
	}
	l := listing("/repo")
	opened := created("worktree_opened", "/p")
	opened.AlreadyOpen = nil
	missingPath := created("worktree_created", "")
	for name, tc := range map[string]struct {
		method string
		result herdr.CreatedResult
	}{
		"open without already_open": {"worktree.open", opened},
		"open with create type":     {"worktree.open", created("worktree_created", "/p")},
		"create without path":       {"worktree.create", missingPath},
		"create with open type":     {"worktree.create", created("worktree_opened", "/p")},
	} {
		t.Run(name, func(t *testing.T) {
			api := herdrscript.Client(t, call{Method: tc.method, Result: tc.result})
			var err error
			if tc.method == "worktree.open" {
				_, err = Open(context.Background(), api, src, l, "/p")
			} else {
				_, err = Create(context.Background(), api, src, l, "b", "", "/p")
			}
			if err == nil {
				t.Fatal("accepted incomplete result")
			}
		})
	}
}
