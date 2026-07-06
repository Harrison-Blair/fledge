package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/graph"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() {
	register("graph", runGraph, "fledge graph [--format text|dot|json] [--json] [REQ-###]")
}

type graphNode struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Requirement string   `json:"requirement"`
	DependsOn   []string `json:"depends_on"`
}

func runGraph(args []string) int {
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	format := fs.String("format", "text", "output format: text|dot|json")
	jsonOut := fs.Bool("json", false, "shorthand for --format json")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *jsonOut {
		*format = "json"
	}
	if *format != "text" && *format != "dot" && *format != "json" {
		return usageErr("unknown --format %q (want text|dot|json)", *format)
	}
	var reqFilter string
	if fs.NArg() > 1 {
		return usageErr("at most one requirement filter allowed")
	}
	if fs.NArg() == 1 {
		reqFilter = fs.Arg(0)
	}

	_, set, _, code, ok := loadSet()
	if !ok {
		return code
	}
	tasks := set.Tasks
	if reqFilter != "" {
		if set.Req(reqFilter) == nil {
			return fail("requirement %s not found", reqFilter)
		}
		tasks = filterSubgraph(set, reqFilter)
	}

	g := graph.New(tasks)
	if c := g.Cycle(); c != nil {
		return fail("dependency cycle: %s", strings.Join(c, " -> "))
	}
	waves, err := g.Waves()
	if err != nil {
		return fail("%v", err)
	}
	byID := map[string]*spec.Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}

	switch *format {
	case "json":
		nodes := make([]graphNode, 0, len(tasks))
		for _, t := range tasks {
			deps := t.DependsOn
			if deps == nil {
				deps = []string{}
			}
			nodes = append(nodes, graphNode{
				ID: t.ID, Title: t.Title, Status: t.Status,
				Requirement: t.Requirement, DependsOn: deps,
			})
		}
		return emitJSON(map[string]any{"nodes": nodes, "waves": waves})
	case "dot":
		fmt.Println("digraph fledge {")
		fmt.Println("  rankdir=LR;")
		for _, t := range tasks {
			attrs := fmt.Sprintf("label=%q", t.ID+"\\n"+t.Title)
			switch t.Status {
			case spec.TaskDone:
				attrs += ", style=filled, fillcolor=lightgray"
			case spec.TaskInProgress:
				attrs += ", style=filled, fillcolor=lightyellow"
			}
			fmt.Printf("  %q [%s];\n", t.ID, attrs)
		}
		for _, t := range tasks {
			for _, dep := range t.DependsOn {
				if _, exists := byID[dep]; exists {
					fmt.Printf("  %q -> %q;\n", dep, t.ID)
				}
			}
		}
		fmt.Println("}")
	default: // text
		if len(waves) == 0 {
			fmt.Println("no tasks")
			return ExitOK
		}
		for i, wave := range waves {
			var parts []string
			for _, id := range wave {
				label := id
				switch byID[id].Status {
				case spec.TaskDone:
					label += " [done]"
				case spec.TaskInProgress:
					label += " [in-progress]"
				}
				parts = append(parts, label)
			}
			fmt.Printf("wave %d: %s\n", i+1, strings.Join(parts, ", "))
		}
	}
	return ExitOK
}

// filterSubgraph returns the tasks of one requirement plus the dependency
// closure of tasks they reach in other requirements.
func filterSubgraph(set *spec.Set, reqID string) []*spec.Task {
	include := map[string]bool{}
	var queue []string
	for _, t := range set.Tasks {
		if t.Requirement == reqID {
			include[t.ID] = true
			queue = append(queue, t.ID)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		t := set.Task(id)
		if t == nil {
			continue
		}
		for _, dep := range t.DependsOn {
			if !include[dep] && set.Task(dep) != nil {
				include[dep] = true
				queue = append(queue, dep)
			}
		}
	}
	var out []*spec.Task
	for _, t := range set.Tasks {
		if include[t.ID] {
			out = append(out, t)
		}
	}
	return out
}
