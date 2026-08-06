package messaging

import (
	"os"
	"strings"
	"testing"
)

// A diagnostic reader must survive a ledger written by a newer Fledge, so an
// unrecognized field is data it ignores rather than a decode failure.
func TestDecodeLedgerLineIsLenientAboutUnknownFields(t *testing.T) {
	t.Parallel()

	line := `{"version":1,"type":"task_completed","at":"2026-01-01T00:00:00Z","session_id":"s",` +
		`"task_id":"t-1","task_status":"completed","actor":"orchestrator","invented_field":"x"}`
	entry, ok := DecodeLedgerLine([]byte(line))
	if !ok {
		t.Fatal("DecodeLedgerLine rejected a line carrying an unknown field")
	}
	if entry.Type != "task_completed" || entry.TaskID != "t-1" || entry.Actor != "orchestrator" {
		t.Fatalf("entry = %#v", entry)
	}
	if entry.TaskStatus != TaskCompleted || entry.At.IsZero() {
		t.Fatalf("entry status/time = %s / %v", entry.TaskStatus, entry.At)
	}
}

func TestDecodeLedgerLineRejectsUnusableLines(t *testing.T) {
	t.Parallel()

	for name, line := range map[string]string{
		"blank":      "   ",
		"not json":   "not-json",
		"no type":    `{"version":1,"at":"2026-01-01T00:00:00Z","session_id":"s"}`,
		"no time":    `{"version":1,"type":"task_progress","session_id":"s"}`,
		"wrong type": `{"version":1,"type":5,"at":"2026-01-01T00:00:00Z","session_id":"s"}`,
	} {
		if _, ok := DecodeLedgerLine([]byte(line)); ok {
			t.Fatalf("DecodeLedgerLine accepted %s: %s", name, line)
		}
	}
}

// The caller a transition was authorized against is the only record of who
// acted: an orchestrator completing a worker's task must not read back as the
// worker completing its own.
func TestTaskTransitionsAndProgressRecordTheCaller(t *testing.T) {
	t.Parallel()

	store := coordinationTestStore(t)
	if _, _, err := store.RegisterAgent(RegisterParams{Name: "worker", PaneID: "p1", Harness: "codex", Caller: UserIdentity}); err != nil {
		t.Fatal(err)
	}
	task, err := store.AssignTask(OrchestratorIdentity, "worker", "", "do the work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordProgress("worker", task.ID, "halfway"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionTask(OrchestratorIdentity, task.ID, TaskCanceled, "no longer needed"); err != nil {
		t.Fatal(err)
	}
	actors := map[string]string{}
	contents, err := os.ReadFile(store.LogPath())
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSuffix(string(contents), "\n"), "\n") {
		entry, ok := DecodeLedgerLine([]byte(line))
		if !ok {
			t.Fatalf("undecodable ledger line %q", line)
		}
		actors[entry.Type] = entry.Actor
	}
	if actors[eventTaskProgress] != "worker" {
		t.Fatalf("task_progress actor = %q", actors[eventTaskProgress])
	}
	if actors[eventTaskCanceled] != OrchestratorIdentity {
		t.Fatalf("task_canceled actor = %q", actors[eventTaskCanceled])
	}
}

// A descendant canceled by an ancestor transition records the ancestor's
// caller, even though that caller was not authorized against the descendant.
func TestCascadeCancelRecordsTheAncestorCallerOnDescendants(t *testing.T) {
	t.Parallel()

	store := coordinationTestStore(t)
	for _, params := range []RegisterParams{
		{Name: "lead", PaneID: "p1", Harness: "codex", Caller: UserIdentity, CanDelegate: true},
		{Name: "worker", PaneID: "p2", Harness: "codex", Caller: UserIdentity},
	} {
		if _, _, err := store.RegisterAgent(params); err != nil {
			t.Fatal(err)
		}
	}
	parent, err := store.AssignTask(OrchestratorIdentity, "lead", "", "lead the work")
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.AssignTask("lead", "worker", parent.ID, "do the work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionTask(UserIdentity, parent.ID, TaskCanceled, "stop the work"); err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(store.LogPath())
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSuffix(string(contents), "\n"), "\n") {
		entry, ok := DecodeLedgerLine([]byte(line))
		if !ok {
			t.Fatalf("undecodable ledger line %q", line)
		}
		if entry.Type == eventTaskCanceled && entry.TaskID == child.ID {
			if entry.Actor != UserIdentity {
				t.Fatalf("descendant task_canceled actor = %q, want %q", entry.Actor, UserIdentity)
			}
			return
		}
	}
	t.Fatalf("no task_canceled event found for descendant %s", child.ID)
}
