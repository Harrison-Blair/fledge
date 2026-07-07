package cli

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/Harrison-Blair/fledge/internal/check"
	"github.com/Harrison-Blair/fledge/internal/graph"
)

func init() { register("ready", runReady, "fledge ready [--json]") }

type readyTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Priority    string `json:"priority"`
	Requirement string `json:"requirement"`
	Oversight   string `json:"oversight,omitempty"`
	Path        string `json:"path"`
}

func runReady(args []string) int {
	fs := flag.NewFlagSet("ready", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	r, set, locked, code, ok := loadSet()
	if !ok {
		return code
	}

	// Refuse to compute readiness over a broken spec set.
	findings := check.Run(set, locked, r.EvidenceDir())
	if check.HasErrors(findings) {
		for _, f := range findings {
			if f.Severity == check.Error {
				fmt.Fprintf(os.Stderr, "ERROR %s: %s\n", relPath(r.Root, f.File), f.Message)
			}
		}
		return fail("spec set has errors; fix them (see `fledge check`) before dispatch")
	}

	var out []readyTask
	for _, id := range graph.New(set.Tasks).Ready() {
		if slices.Contains(locked, id) {
			continue
		}
		t := set.Task(id)
		out = append(out, readyTask{
			ID: t.ID, Title: t.Title, Priority: t.Priority,
			Requirement: t.Requirement, Oversight: t.Oversight,
			Path: relPath(r.Root, t.Path),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})

	if *jsonOut {
		if out == nil {
			out = []readyTask{}
		}
		return emitJSON(out)
	}
	if len(out) == 0 {
		fmt.Println("no ready tasks")
		return ExitOK
	}
	for _, t := range out {
		line := fmt.Sprintf("%s  %s  %s  (req %s)", t.ID, t.Priority, t.Title, t.Requirement)
		if t.Oversight != "" {
			line += fmt.Sprintf("  [oversight: %s]", t.Oversight)
		}
		fmt.Println(line)
	}
	return ExitOK
}
