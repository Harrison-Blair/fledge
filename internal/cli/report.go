package cli

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lock"
	"github.com/Harrison-Blair/fledge/internal/repo"
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

type blockedTask struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Unmet []string `json:"unmet"`
}

type lockEntry struct {
	Task  string `json:"task"`
	Owner string `json:"owner"`
}

type issues struct {
	ParseErrors  []string `json:"parse_errors"`
	DanglingRefs []string `json:"dangling_refs"`
}

type report struct {
	Counts       reportCounts    `json:"counts"`
	Requirements []reqCompletion `json:"requirements"`
	Orphans      []orphanTask    `json:"orphans"`
	Blocked      []blockedTask   `json:"blocked"`
	Locks        []lockEntry     `json:"locks"`
	Issues       issues          `json:"issues"`
}

func runReport(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	r, set, _, code, ok := loadSet()
	if !ok {
		return code
	}

	rep := buildReport(r, set)

	if *jsonOut {
		return emitJSON(rep)
	}
	printReport(rep)
	return ExitOK
}

// buildReport computes the report once from set, for both text and JSON
// rendering. Tasks whose requirement reference does not resolve to a loaded
// requirement are surfaced as orphans and excluded from per-requirement counts.
func buildReport(r *repo.Repo, set *spec.Set) report {
	rep := report{
		Requirements: []reqCompletion{},
		Orphans:      []orphanTask{},
		Blocked:      []blockedTask{},
		Locks:        []lockEntry{},
		Issues:       issues{ParseErrors: []string{}, DanglingRefs: []string{}},
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

	for _, t := range set.Tasks {
		if t.Requirement != "" && set.Req(t.Requirement) == nil {
			rep.Issues.DanglingRefs = append(rep.Issues.DanglingRefs,
				fmt.Sprintf("%s requirement %s not found", t.ID, t.Requirement))
		}

		if t.Status == spec.TaskDone {
			continue
		}
		var unmet []string
		for _, dep := range t.DependsOn {
			d := set.Task(dep)
			if d == nil {
				rep.Issues.DanglingRefs = append(rep.Issues.DanglingRefs,
					fmt.Sprintf("%s depends_on %s not found", t.ID, dep))
				unmet = append(unmet, dep)
			} else if d.Status != spec.TaskDone {
				unmet = append(unmet, dep)
			}
		}
		if len(unmet) > 0 {
			sort.Strings(unmet)
			rep.Blocked = append(rep.Blocked, blockedTask{ID: t.ID, Title: t.Title, Unmet: unmet})
		}
	}
	sort.Slice(rep.Blocked, func(i, j int) bool { return rep.Blocked[i].ID < rep.Blocked[j].ID })
	sort.Strings(rep.Issues.DanglingRefs)

	for _, fe := range set.Errors {
		rep.Issues.ParseErrors = append(rep.Issues.ParseErrors, fmt.Sprintf("%s: %v", relPath(r.Root, fe.Path), fe.Err))
	}
	sort.Strings(rep.Issues.ParseErrors)

	if recs, err := lock.List(r.LocksDir()); err == nil {
		for _, rec := range recs {
			rep.Locks = append(rep.Locks, lockEntry{Task: rec.Task, Owner: rec.Owner})
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

	fmt.Println("\nBlocked detail:")
	if len(rep.Blocked) == 0 {
		fmt.Println("  (none)")
	}
	for _, b := range rep.Blocked {
		fmt.Printf("  %s  %s  (unmet: %s)\n", b.ID, b.Title, strings.Join(b.Unmet, ", "))
	}

	fmt.Println("\nLocks:")
	if len(rep.Locks) == 0 {
		fmt.Println("  (none)")
	}
	for _, l := range rep.Locks {
		fmt.Printf("  %s  %s\n", l.Task, l.Owner)
	}

	fmt.Println("\nIssues:")
	if len(rep.Issues.ParseErrors) == 0 && len(rep.Issues.DanglingRefs) == 0 {
		fmt.Println("  (none)")
	}
	for _, p := range rep.Issues.ParseErrors {
		fmt.Printf("  parse error: %s\n", p)
	}
	for _, d := range rep.Issues.DanglingRefs {
		fmt.Printf("  dangling ref: %s\n", d)
	}
}
