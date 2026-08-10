package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// Each verb must reach the manager as the right transition, with the detail the
// caller supplied. A verb wired to the wrong status silently corrupts the
// durable task record, so the mapping is asserted exhaustively.
func TestTaskVerbsMapToTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		args   []string
		want   taskCall
		detail string
	}{
		{args: []string{"progress", "t-1", "halfway"},
			want: taskCall{verb: "progress", dir: "/project/nested", target: "t-1", detail: "halfway"}},
		{args: []string{"blocked", "t-1", "need input"},
			want: taskCall{verb: "transition", dir: "/project/nested", target: "t-1", status: messaging.TaskBlocked, detail: "need input"}},
		{args: []string{"needs-decision", "t-1", "which one"},
			want: taskCall{verb: "transition", dir: "/project/nested", target: "t-1", status: messaging.TaskNeedsDecision, detail: "which one"}},
		{args: []string{"resume", "t-1"},
			want: taskCall{verb: "transition", dir: "/project/nested", target: "t-1", status: messaging.TaskActive}},
		{args: []string{"complete", "t-1"},
			want: taskCall{verb: "transition", dir: "/project/nested", target: "t-1", status: messaging.TaskCompleted}},
		{args: []string{"fail", "t-1", "broke"},
			want: taskCall{verb: "transition", dir: "/project/nested", target: "t-1", status: messaging.TaskFailed, detail: "broke"}},
		{args: []string{"cancel", "t-1"},
			want: taskCall{verb: "transition", dir: "/project/nested", target: "t-1", status: messaging.TaskCanceled}},
	}
	for _, current := range cases {
		t.Run(current.args[0], func(t *testing.T) {
			manager := &fakeManager{taskResult: messaging.Task{ID: "t-1", Status: current.want.status}}
			output, err := runRootCommand(t, manager, append([]string{"agent", "task"}, current.args...)...)
			if err != nil {
				t.Fatal(err)
			}
			if len(manager.taskCalls) != 1 || manager.taskCalls[0] != current.want {
				t.Fatalf("calls = %#v, want %#v", manager.taskCalls, current.want)
			}
			if !strings.Contains(output, "t-1") {
				t.Fatalf("output = %q", output)
			}
		})
	}
}

// Transitions that carry a reason must refuse to record a blank one; the rest
// must stay usable without a detail.
func TestTaskDetailRequirements(t *testing.T) {
	t.Parallel()

	for _, verb := range []string{"progress", "blocked", "needs-decision", "fail"} {
		t.Run("required/"+verb, func(t *testing.T) {
			manager := &fakeManager{}
			_, err := runRootCommand(t, manager, "agent", "task", verb, "t-1")
			if err == nil || !strings.Contains(err.Error(), "--file") {
				t.Fatalf("error = %v, want a missing-detail error", err)
			}
			if len(manager.taskCalls) != 0 {
				t.Fatalf("calls = %#v, want none", manager.taskCalls)
			}
		})
	}
	for _, verb := range []string{"resume", "complete", "cancel"} {
		t.Run("optional/"+verb, func(t *testing.T) {
			manager := &fakeManager{taskResult: messaging.Task{ID: "t-1"}}
			if _, err := runRootCommand(t, manager, "agent", "task", verb, "t-1"); err != nil {
				t.Fatalf("%s without detail: %v", verb, err)
			}
		})
	}
}

func TestTaskDetailAndAssignmentAcceptFileBodies(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(path, []byte("read the whole diff\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &fakeManager{taskResult: messaging.Task{ID: "t-9", Assignee: "worker"}}
	if _, err := runRootCommand(t, manager, "agent", "task", "assign", "worker", "--parent-task", "t-1", "--file", path); err != nil {
		t.Fatal(err)
	}
	// File bodies reach the manager verbatim; the store is what trims them.
	want := taskCall{verb: "assign", dir: "/project/nested", target: "worker", parent: "t-1", detail: "read the whole diff\n"}
	if len(manager.taskCalls) != 1 || manager.taskCalls[0] != want {
		t.Fatalf("calls = %#v, want %#v", manager.taskCalls, want)
	}

	manager = &fakeManager{taskResult: messaging.Task{ID: "t-9"}}
	if _, err := runRootCommand(t, manager, "agent", "task", "blocked", "t-9", "--file", path); err != nil {
		t.Fatal(err)
	}
	if len(manager.taskCalls) != 1 || manager.taskCalls[0].detail != "read the whole diff\n" {
		t.Fatalf("calls = %#v", manager.taskCalls)
	}
}

func TestTaskListAndShowRenderCoordinationState(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	manager := &fakeManager{taskListResult: []messaging.Task{
		{ID: "t-1", Status: messaging.TaskActive, Assignee: "worker", Assigner: "orchestrator", Description: "review\nthe diff", CreatedAt: created},
		{ID: "t-2", Status: messaging.TaskBlocked, Assignee: "child", Assigner: "worker", ParentID: "t-1", Description: "sub work", CreatedAt: created},
	}}
	output, err := runRootCommand(t, manager, "agent", "task", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ID", "STATUS", "PARENT", "t-1", "t-2", "blocked", "review the diff"} {
		if !strings.Contains(output, want) {
			t.Fatalf("list output %q is missing %q", output, want)
		}
	}

	manager = &fakeManager{taskResult: messaging.Task{ID: "t-2", Status: messaging.TaskBlocked,
		Assignee: "child", Assigner: "worker", ParentID: "t-1", Description: "sub work", Detail: "waiting on review"}}
	output, err = runRootCommand(t, manager, "agent", "task", "show", "t-2")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ID: t-2", "Status: blocked", "Parent: t-1", "waiting on review"} {
		if !strings.Contains(output, want) {
			t.Fatalf("show output %q is missing %q", output, want)
		}
	}
}

func TestAgentListRendersRegistryStates(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{agentListResult: []messaging.Agent{
		{Name: "orchestrator", PaneID: "p1", Harness: "codex", CanDelegate: true, Active: true, Status: "working"},
		{Name: "child", PaneID: "p3", Harness: "claude", Active: true, Status: "idle", ParentTaskID: "t-1"},
		{Name: "departed", PaneID: "p4", Harness: "codex", Active: false, Status: "working"},
	}}
	output, err := runRootCommand(t, manager, "agent", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NAME", "CAN DELEGATE", "PARENT TASK", "orchestrator", "working", "idle", "t-1"} {
		if !strings.Contains(output, want) {
			t.Fatalf("agent list output %q is missing %q", output, want)
		}
	}
	// An inactive registration must never render as a live state.
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "departed") && !strings.Contains(line, "stopped") {
			t.Fatalf("stopped agent rendered as %q", line)
		}
	}
}

func TestTaskCommandsSurfaceManagerErrors(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{taskErr: errors.New("agent has no task capacity")}
	if _, err := runRootCommand(t, manager, "agent", "task", "assign", "worker", "more work"); err == nil ||
		!strings.Contains(err.Error(), "no task capacity") {
		t.Fatalf("error = %v", err)
	}
}
