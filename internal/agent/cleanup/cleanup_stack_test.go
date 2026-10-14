package cleanup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// stack is a repo with three stacked layer checkouts l1, l2, and l3, each
// created by an ended worker of caller from the layer below it (l1 from dev),
// and a stack parent task. merge is a merge commit on dev that is in no
// dev ref yet. The layer heads are not in merge, as after a rebase on GitHub.
type stack struct {
	repo
	caller  herdr.AgentDetails
	layers  []string
	workers []string
	merge   string
}

func newStack(t *testing.T) stack {
	t.Helper()
	r := newRepo(t)
	gittest.Git(t, r.root, "update-ref", "refs/remotes/origin/dev", "dev")
	st := stack{repo: r, caller: agent("w1:p1", "w1", "t_caller", "orchestrator", "working")}
	callerRec := r.register(t, st.caller, nil, "adopt", nil)
	base := "dev"
	for _, name := range []string{"l1", "l2", "l3"} {
		path := filepath.Join(r.root, ".fledge", "worktrees", name)
		gittest.Git(t, r.root, "worktree", "add", "-q", "-b", name, path, base)
		gittest.Git(t, path, "commit", "-qm", name, "--allow-empty")
		w := agent("w9:p1", "w9", "t_"+name, name, "idle")
		rec := r.register(t, w, &callerRec.ID, "spawn", created(t, path, base))
		r.endRecord(t, rec.ID)
		st.layers, st.workers = append(st.layers, path), append(st.workers, rec.ID)
		base = name
	}
	st.merge = strings.TrimSpace(gittest.Git(t, r.root, "commit-tree", "-p", "dev", "-m", "merge", "dev^{tree}"))
	return st
}

// seed stores the stack parent task and one layer task per layer, owned by
// the worker of that layer: landed with the merge commit merge(i), or
// verified when merge(i) is nil.
func (st stack) seed(t *testing.T, merge func(i int) *string) {
	t.Helper()
	parent := tasktest.Seed(t, st.root, task.Record{Title: "stack", Status: task.Verified, Shape: task.ShapeStack})
	for i, path := range st.layers {
		owner := st.workers[i]
		status := task.Landed
		if merge(i) == nil {
			status = task.Verified
		}
		tasktest.Seed(t, st.root, task.Record{Title: filepath.Base(path), Parent: &parent, Owner: &owner, Status: status, MergeCommit: merge(i), TargetBranch: s("dev")})
	}
}

// dryRun plans the cleanup of the stack.
func (st stack) dryRun(t *testing.T) Result {
	t.Helper()
	out := Run(context.Background(), client(t, st.repo,
		herdrscript.Get(st.caller.PaneID, st.caller), herdrscript.List(st.caller), call{Method: "worktree.list", Result: st.listing(nil, st.layers...)},
	), Options{DryRun: true})
	if out.Status != "success" || out.ExitCode() != 0 {
		t.Fatalf("%+v", out)
	}
	return out.Result.(Result)
}

func (st stack) outcomes(res Result) []string {
	var got []string
	for _, path := range st.layers {
		got = append(got, checkout(res, path).Outcome)
	}
	return got
}

func all(outcome string) string { return strings.Repeat(outcome+" ", 2) + outcome }

// The checkout of a stacked layer is judged against the stack base: it is
// merged when the merge commit of its task is in refs/heads/<dev> or in
// refs/remotes/origin/<dev>, not when its head is in the base it was created from.
func TestStackLayerJudgedByMergeCommit(t *testing.T) {
	for _, tc := range []struct {
		name string
		refs []string
		want string
	}{
		{"both dev refs hold the merge commit", []string{"refs/heads/dev", "refs/remotes/origin/dev"}, "planned"},
		{"only origin dev holds the merge commit", []string{"refs/remotes/origin/dev"}, "planned"},
		{"only local dev holds the merge commit", []string{"refs/heads/dev"}, "planned"},
		{"stale dev refs", nil, "skipped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t)
			st.seed(t, func(int) *string { return &st.merge })
			for _, ref := range tc.refs {
				gittest.Git(t, st.root, "update-ref", ref, st.merge)
			}
			res := st.dryRun(t)
			if got := strings.Join(st.outcomes(res), " "); got != all(tc.want) {
				t.Fatalf("%s: %+v", got, res.Checkouts)
			}
			for _, path := range st.layers {
				c := checkout(res, path)
				if c.Base == nil || *c.Base != "dev" {
					t.Fatalf("base: %+v", c)
				}
				if tc.want == "skipped" && !strings.Contains(*c.Reason, "merged into dev: no") {
					t.Fatalf("reason: %s", *c.Reason)
				}
			}
		})
	}
}

// Cleanup runs no git fetch: the layers stay held until a git fetch brings
// the merge commit into refs/remotes/origin/<dev>, even with a stale
// refs/heads/<dev>.
func TestStackLayerPlannedAfterFetch(t *testing.T) {
	st := newStack(t)
	st.seed(t, func(int) *string { return &st.merge })
	origin := t.TempDir()
	gittest.Git(t, origin, "init", "-q", "--bare")
	gittest.Git(t, st.root, "push", "-q", origin, st.merge+":refs/heads/dev")
	gittest.Git(t, st.root, "remote", "add", "origin", origin)
	if got := strings.Join(st.outcomes(st.dryRun(t)), " "); got != all("skipped") {
		t.Fatalf("before fetch: %s", got)
	}
	gittest.Git(t, st.root, "fetch", "-q", "origin", "dev")
	if got := strings.Join(st.outcomes(st.dryRun(t)), " "); got != all("planned") {
		t.Fatalf("after fetch: %s", got)
	}
	if strings.TrimSpace(gittest.Git(t, st.root, "rev-parse", "refs/heads/dev")) == st.merge {
		t.Fatal("refs/heads/dev changed")
	}
}

// A stacked layer with no merge commit is held, even when dev holds the
// merge commit of the other layers.
func TestStackLayerWithoutMergeCommitHeld(t *testing.T) {
	st := newStack(t)
	st.seed(t, func(i int) *string {
		if i == 1 {
			return nil
		}
		return &st.merge
	})
	gittest.Git(t, st.root, "update-ref", "refs/heads/dev", st.merge)
	gittest.Git(t, st.root, "update-ref", "refs/remotes/origin/dev", st.merge)
	if got := strings.Join(st.outcomes(st.dryRun(t)), " "); got != "planned skipped planned" {
		t.Fatalf("%s", got)
	}
}

// Cleanup removes the checkouts of the merged layers and keeps their branches.
func TestStackLayerCheckoutsRemovedBranchesKept(t *testing.T) {
	st := newStack(t)
	st.seed(t, func(int) *string { return &st.merge })
	gittest.Git(t, st.root, "update-ref", "refs/heads/dev", st.merge)
	gittest.Git(t, st.root, "update-ref", "refs/remotes/origin/dev", st.merge)
	calls := []call{herdrscript.Get(st.caller.PaneID, st.caller), herdrscript.List(st.caller), {Method: "worktree.list", Result: st.listing(nil, st.layers...)}}
	for range st.layers {
		calls = append(calls, call{Method: "worktree.list", Result: st.listing(nil, st.layers...)}, herdrscript.List(st.caller), herdrscript.List(st.caller))
	}
	out := Run(context.Background(), client(t, st.repo, calls...), Options{})
	if out.Status != "success" || out.ExitCode() != 0 {
		t.Fatalf("%+v", out)
	}
	if got := strings.Join(st.outcomes(out.Result.(Result)), " "); got != all("done") {
		t.Fatalf("%s", got)
	}
	for _, path := range st.layers {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("checkout %s kept", path)
		}
	}
	if got := strings.Fields(gittest.Git(t, st.root, "branch", "--list", "l*")); strings.Join(got, " ") != "l1 l2 l3" {
		t.Fatalf("branches: %q", got)
	}
}
