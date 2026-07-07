// Package graph computes dependency structure over TASK specs: cycle
// detection, topological waves, and the ready set.
package graph

import (
	"fmt"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

// Graph is the depends_on graph over a set of tasks.
type Graph struct {
	tasks []*spec.Task
	byID  map[string]*spec.Task
}

// New builds a graph. Dangling depends_on references are tolerated (check
// reports them); they count as never-done for readiness and are skipped for
// cycle/wave purposes.
func New(tasks []*spec.Task) *Graph {
	byID := make(map[string]*spec.Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}
	return &Graph{tasks: tasks, byID: byID}
}

// Cycle returns a cycle path (first node repeated at the end), or nil.
func (g *Graph) Cycle() []string {
	const (
		unvisited = 0
		inStack   = 1
		finished  = 2
	)
	state := map[string]int{}
	var stack []string
	var found []string

	var visit func(id string) bool
	visit = func(id string) bool {
		state[id] = inStack
		stack = append(stack, id)
		if t := g.byID[id]; t != nil {
			for _, dep := range t.DependsOn {
				if _, exists := g.byID[dep]; !exists {
					continue
				}
				switch state[dep] {
				case unvisited:
					if visit(dep) {
						return true
					}
				case inStack:
					for i, s := range stack {
						if s == dep {
							found = append(append([]string{}, stack[i:]...), dep)
							return true
						}
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = finished
		return false
	}

	for _, t := range g.tasks {
		if state[t.ID] == unvisited && visit(t.ID) {
			return found
		}
	}
	return nil
}

// Waves returns task IDs grouped into topological layers: a task's wave is
// one past its deepest dependency. Errors on a cycle.
func (g *Graph) Waves() ([][]string, error) {
	if c := g.Cycle(); c != nil {
		return nil, fmt.Errorf("dependency cycle: %s", strings.Join(c, " -> "))
	}
	assigned := map[string]bool{}
	remaining := len(g.tasks)
	var waves [][]string
	for remaining > 0 {
		var wave []string
		for _, t := range g.tasks {
			if assigned[t.ID] {
				continue
			}
			ok := true
			for _, dep := range t.DependsOn {
				if _, exists := g.byID[dep]; exists && !assigned[dep] {
					ok = false
					break
				}
			}
			if ok {
				wave = append(wave, t.ID)
			}
		}
		for _, id := range wave {
			assigned[id] = true
		}
		waves = append(waves, wave)
		remaining -= len(wave)
	}
	return waves, nil
}

// Ready returns IDs of tasks that are not started (blocked/ready hint) and
// whose depends_on are all done, in input order. Dangling deps never count
// as done.
func (g *Graph) Ready() []string {
	var out []string
	for _, t := range g.tasks {
		if t.Status != spec.TaskEgg && t.Status != spec.TaskPipping {
			continue
		}
		ok := true
		for _, dep := range t.DependsOn {
			d := g.byID[dep]
			if d == nil || d.Status != spec.TaskFledged {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, t.ID)
		}
	}
	return out
}
