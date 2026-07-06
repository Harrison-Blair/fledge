package cli

import (
	"flag"
	"fmt"
	"sort"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() { register("report", runReport, "fledge report [--json]") }

type reportCounts struct {
	Blocked    int `json:"blocked"`
	Ready      int `json:"ready"`
	InProgress int `json:"in_progress"`
	Done       int `json:"done"`
	Total      int `json:"total"`
}

type reqCompletion struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
}

type orphanTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Requirement string `json:"requirement"`
}

type report struct {
	Counts       reportCounts    `json:"counts"`
	Requirements []reqCompletion `json:"requirements"`
	Orphans      []orphanTask    `json:"orphans"`
}

func runReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	_, set, _, code, ok := loadSet()
	if !ok {
		return code
	}

	rep := buildReport(set)

	if *jsonOut {
		return emitJSON(rep)
	}
	printReport(rep)
	return ExitOK
}

// buildReport computes the report once from set, for both text and JSON
// rendering. Tasks whose requirement reference does not resolve to a loaded
// requirement are surfaced as orphans and excluded from per-requirement counts.
func buildReport(set *spec.Set) report {
	rep := report{
		Requirements: []reqCompletion{},
		Orphans:      []orphanTask{},
	}

	reqDone := map[string]int{}
	reqTotal := map[string]int{}

	for _, t := range set.Tasks {
		switch t.Status {
		case spec.TaskBlocked:
			rep.Counts.Blocked++
		case spec.TaskReady:
			rep.Counts.Ready++
		case spec.TaskInProgress:
			rep.Counts.InProgress++
		case spec.TaskDone:
			rep.Counts.Done++
		}
		rep.Counts.Total++

		if set.Req(t.Requirement) == nil {
			rep.Orphans = append(rep.Orphans, orphanTask{ID: t.ID, Title: t.Title, Requirement: t.Requirement})
			continue
		}
		reqTotal[t.Requirement]++
		if t.Status == spec.TaskDone {
			reqDone[t.Requirement]++
		}
	}

	for _, r := range set.Reqs {
		rep.Requirements = append(rep.Requirements, reqCompletion{
			ID: r.ID, Title: r.Title, Status: r.Status,
			Done: reqDone[r.ID], Total: reqTotal[r.ID],
		})
	}
	sort.Slice(rep.Requirements, func(i, j int) bool { return rep.Requirements[i].ID < rep.Requirements[j].ID })
	sort.Slice(rep.Orphans, func(i, j int) bool { return rep.Orphans[i].ID < rep.Orphans[j].ID })

	return rep
}

func printReport(rep report) {
	c := rep.Counts
	fmt.Printf("Tasks: %d total (blocked: %d, ready: %d, in-progress: %d, done: %d)\n",
		c.Total, c.Blocked, c.Ready, c.InProgress, c.Done)

	fmt.Println("\nRequirements:")
	if len(rep.Requirements) == 0 {
		fmt.Println("  (none)")
	}
	for _, r := range rep.Requirements {
		fmt.Printf("  %s  %s  %s  %d/%d done\n", r.ID, r.Title, r.Status, r.Done, r.Total)
	}

	fmt.Println("\nOrphan tasks (dangling requirement reference):")
	if len(rep.Orphans) == 0 {
		fmt.Println("  (none)")
	}
	for _, o := range rep.Orphans {
		fmt.Printf("  %s  %s  (requirement: %s)\n", o.ID, o.Title, o.Requirement)
	}
}
