package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// TestTaskListHidesSiblingsAndCousins builds a durable task tree in which the
// caller participates in one branch and asserts that TaskList exposes only that
// branch plus its ancestor chain — never an unrelated sibling or cousin subtree
// that merely shares a visible ancestor. It also confirms TaskShow inherits the
// same visibility and that the privileged user/orchestrator identities still
// see every task.
func TestTaskListHidesSiblingsAndCousins(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	manager, _ := newTestManager(&fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}, &fakeConfirmer{})

	store := messaging.New(root, testSessionName)
	for _, params := range []messaging.RegisterParams{
		{Name: "orchestrator", PaneID: "p-orchestrator", Harness: "codex", Caller: userIdentity, CanDelegate: true},
		{Name: "ancestor", PaneID: "p-ancestor", Harness: "codex", Caller: userIdentity},
		{Name: "worker", PaneID: "p-worker", Harness: "codex", Caller: userIdentity, CanDelegate: true},
		{Name: "grandchild", PaneID: "p-grandchild", Harness: "codex", Caller: userIdentity},
		{Name: "other", PaneID: "p-other", Harness: "codex", Caller: userIdentity},
		{Name: "cousinee", PaneID: "p-cousinee", Harness: "codex", Caller: userIdentity},
	} {
		if _, _, err := store.RegisterAgent(params); err != nil {
			t.Fatalf("register %s: %v", params.Name, err)
		}
	}

	// root -> branch -> child is the caller's chain; root -> sibling -> cousin is
	// an unrelated subtree the caller never touched.
	rootTask := assignTask(t, store, orchestratorIdentity, "ancestor", "", "root work")
	branch := assignTask(t, store, orchestratorIdentity, "worker", rootTask.ID, "branch work")
	child := assignTask(t, store, "worker", "grandchild", branch.ID, "child work")
	sibling := assignTask(t, store, orchestratorIdentity, "other", rootTask.ID, "sibling work")
	cousin := assignTask(t, store, orchestratorIdentity, "cousinee", sibling.ID, "cousin work")

	asCaller := func(paneID string) {
		manager.getenv = func(key string) string {
			if key == "HERDR_PANE_ID" {
				return paneID
			}
			return ""
		}
	}
	ctx := context.Background()

	// The worker participates in branch (assignee) and child (assigner); it sees
	// those plus their ancestor root, in store order, and nothing else.
	asCaller("p-worker")
	got, err := manager.TaskList(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{rootTask.ID, branch.ID, child.ID}; !equalTaskIDs(got, want) {
		t.Fatalf("worker TaskList = %v, want %v (sibling %s and cousin %s must be hidden)",
			taskIDs(got), want, sibling.ID, cousin.ID)
	}

	// TaskShow rides the same visibility set: a hidden sibling is not found.
	if _, err := manager.TaskShow(ctx, root, sibling.ID); !errors.Is(err, messaging.ErrTaskNotFound) {
		t.Fatalf("worker TaskShow(sibling) error = %v, want ErrTaskNotFound", err)
	}
	// A visible task in the caller's chain is still shown.
	if shown, err := manager.TaskShow(ctx, root, branch.ID); err != nil || shown.ID != branch.ID {
		t.Fatalf("worker TaskShow(branch) = %#v, %v; want branch", shown, err)
	}

	all := []string{rootTask.ID, branch.ID, child.ID, sibling.ID, cousin.ID}
	// The direct user (no pane) and the orchestrator see the whole tree.
	asCaller("")
	if got, err := manager.TaskList(ctx, root); err != nil || !equalTaskIDs(got, all) {
		t.Fatalf("user TaskList = %v, %v; want all %v", taskIDs(got), err, all)
	}
	asCaller("p-orchestrator")
	if got, err := manager.TaskList(ctx, root); err != nil || !equalTaskIDs(got, all) {
		t.Fatalf("orchestrator TaskList = %v, %v; want all %v", taskIDs(got), err, all)
	}
}

func assignTask(t *testing.T, store *messaging.Store, caller, assignee, parent, description string) messaging.Task {
	t.Helper()
	task, err := store.AssignTask(caller, assignee, parent, description)
	if err != nil {
		t.Fatalf("assign %q to %q: %v", description, assignee, err)
	}
	return task
}

func taskIDs(tasks []messaging.Task) []string {
	ids := make([]string, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	return ids
}

func equalTaskIDs(tasks []messaging.Task, want []string) bool {
	if len(tasks) != len(want) {
		return false
	}
	for i, task := range tasks {
		if task.ID != want[i] {
			return false
		}
	}
	return true
}
