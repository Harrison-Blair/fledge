package cleanup

import (
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// A landed task is finished, so it does not hold its owner.
func TestTaskHoldTreatsLandedAsFinished(t *testing.T) {
	owner := "0000000a"
	tasks := []task.Record{{ID: "00000001", Owner: &owner, Status: task.Landed}}
	if got := taskHold(tasks, owner, false); got != "" {
		t.Fatalf("%q", got)
	}
	tasks = append(tasks, task.Record{ID: "00000002", Owner: &owner, Status: task.Completed})
	if got := taskHold(tasks, owner, false); got != "task 00000002 is completed" {
		t.Fatalf("%q", got)
	}
}
