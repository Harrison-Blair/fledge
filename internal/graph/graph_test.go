package graph

import (
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

func task(id, status string, deps ...string) *spec.Task {
	return &spec.Task{ID: id, Status: status, DependsOn: deps}
}

func TestWaves(t *testing.T) {
	g := New([]*spec.Task{
		task("TASK-001", "done"),
		task("TASK-002", "ready", "TASK-001"),
		task("TASK-003", "blocked", "TASK-001"),
		task("TASK-004", "blocked", "TASK-002", "TASK-003"),
	})
	waves, err := g.Waves()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"TASK-001"},
		{"TASK-002", "TASK-003"},
		{"TASK-004"},
	}
	if !reflect.DeepEqual(waves, want) {
		t.Errorf("waves = %v, want %v", waves, want)
	}
}

func TestWavesCycle(t *testing.T) {
	g := New([]*spec.Task{
		task("TASK-001", "ready", "TASK-002"),
		task("TASK-002", "ready", "TASK-001"),
	})
	if _, err := g.Waves(); err == nil {
		t.Error("want cycle error")
	}
}

func TestCycle(t *testing.T) {
	tests := []struct {
		name  string
		tasks []*spec.Task
		want  bool
	}{
		{"acyclic", []*spec.Task{task("TASK-001", "ready"), task("TASK-002", "ready", "TASK-001")}, false},
		{"self-loop", []*spec.Task{task("TASK-001", "ready", "TASK-001")}, true},
		{"two-cycle", []*spec.Task{
			task("TASK-001", "ready", "TASK-002"),
			task("TASK-002", "ready", "TASK-001"),
		}, true},
		{"cycle behind chain", []*spec.Task{
			task("TASK-001", "ready"),
			task("TASK-002", "ready", "TASK-001", "TASK-004"),
			task("TASK-003", "ready", "TASK-002"),
			task("TASK-004", "ready", "TASK-003"),
		}, true},
		{"dangling dep is not a cycle", []*spec.Task{task("TASK-001", "ready", "TASK-099")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycle := New(tt.tasks).Cycle()
			if (cycle != nil) != tt.want {
				t.Errorf("Cycle() = %v, want cycle=%v", cycle, tt.want)
			}
			if tt.want && (len(cycle) < 2 || cycle[0] != cycle[len(cycle)-1]) {
				t.Errorf("cycle path should start and end at the same node: %v", cycle)
			}
		})
	}
}

func TestReady(t *testing.T) {
	g := New([]*spec.Task{
		task("TASK-001", "done"),
		task("TASK-002", "ready", "TASK-001"),          // deps done → ready
		task("TASK-003", "blocked", "TASK-001"),        // stale hint, deps done → ready
		task("TASK-004", "blocked", "TASK-002"),        // dep not done → not ready
		task("TASK-005", "in-progress", "TASK-001"),    // started → not ready
		task("TASK-006", "ready", "TASK-099"),          // dangling dep → not ready
		task("TASK-007", "blocked"),                    // no deps → ready
	})
	want := []string{"TASK-002", "TASK-003", "TASK-007"}
	if got := g.Ready(); !reflect.DeepEqual(got, want) {
		t.Errorf("Ready() = %v, want %v", got, want)
	}
}
