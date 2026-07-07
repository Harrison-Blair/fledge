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
		task("FTHR-001", "fledged"),
		task("FTHR-002", "pipping", "FTHR-001"),
		task("FTHR-003", "egg", "FTHR-001"),
		task("FTHR-004", "egg", "FTHR-002", "FTHR-003"),
	})
	waves, err := g.Waves()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"FTHR-001"},
		{"FTHR-002", "FTHR-003"},
		{"FTHR-004"},
	}
	if !reflect.DeepEqual(waves, want) {
		t.Errorf("waves = %v, want %v", waves, want)
	}
}

func TestWavesCycle(t *testing.T) {
	g := New([]*spec.Task{
		task("FTHR-001", "pipping", "FTHR-002"),
		task("FTHR-002", "pipping", "FTHR-001"),
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
		{"acyclic", []*spec.Task{task("FTHR-001", "pipping"), task("FTHR-002", "pipping", "FTHR-001")}, false},
		{"self-loop", []*spec.Task{task("FTHR-001", "pipping", "FTHR-001")}, true},
		{"two-cycle", []*spec.Task{
			task("FTHR-001", "pipping", "FTHR-002"),
			task("FTHR-002", "pipping", "FTHR-001"),
		}, true},
		{"cycle behind chain", []*spec.Task{
			task("FTHR-001", "pipping"),
			task("FTHR-002", "pipping", "FTHR-001", "FTHR-004"),
			task("FTHR-003", "pipping", "FTHR-002"),
			task("FTHR-004", "pipping", "FTHR-003"),
		}, true},
		{"dangling dep is not a cycle", []*spec.Task{task("FTHR-001", "pipping", "FTHR-099")}, false},
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
		task("FTHR-001", "fledged"),
		task("FTHR-002", "pipping", "FTHR-001"),  // deps done → ready
		task("FTHR-003", "egg", "FTHR-001"),      // stale hint, deps done → ready
		task("FTHR-004", "egg", "FTHR-002"),      // dep not done → not ready
		task("FTHR-005", "hatching", "FTHR-001"), // started → not ready
		task("FTHR-006", "pipping", "FTHR-099"),  // dangling dep → not ready
		task("FTHR-007", "egg"),                  // no deps → ready
	})
	want := []string{"FTHR-002", "FTHR-003", "FTHR-007"}
	if got := g.Ready(); !reflect.DeepEqual(got, want) {
		t.Errorf("Ready() = %v, want %v", got, want)
	}
}
