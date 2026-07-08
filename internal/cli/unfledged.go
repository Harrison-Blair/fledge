package cli

import (
	"flag"
	"fmt"
	"sort"

	"github.com/Harrison-Blair/fledge/internal/repo"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() {
	register("unfledged", runUnfledged, "fledge unfledged [--plumage] [--feathers] [--json]")
}

type unfledgedItem struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Priority  string `json:"priority"`
	Title     string `json:"title"`
	Plumage   string `json:"plumage,omitempty"`
	Oversight string `json:"oversight,omitempty"`
	Path      string `json:"path"`
}

type unfledgedReport struct {
	Plumage  []unfledgedItem `json:"plumage"`
	Feathers []unfledgedItem `json:"feathers"`
	Issues   []string        `json:"issues"`
}

func runUnfledged(args []string) int {
	fs := flag.NewFlagSet("unfledged", flag.ContinueOnError)
	plum := fs.Bool("plumage", false, "show only the plumage section")
	feath := fs.Bool("feathers", false, "show only the feathers section")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	r, set, _, code, ok := loadSet()
	if !ok {
		return code
	}

	// Neither flag (or both) shows both sections; one flag scopes to it.
	showPlumage := *plum || !*feath
	showFeathers := *feath || !*plum

	rep := buildUnfledged(r, set)

	if *jsonOut {
		// Scope the JSON to the selection too, so text and JSON stay identical.
		if !showPlumage {
			rep.Plumage = []unfledgedItem{}
		}
		if !showFeathers {
			rep.Feathers = []unfledgedItem{}
		}
		return emitJSON(rep)
	}
	printUnfledged(rep, showPlumage, showFeathers)
	return ExitOK
}

// buildUnfledged computes the report once, for both text and JSON rendering.
// Every plumage and feather whose status is not fledged is listed; unparseable
// spec files are surfaced as issues rather than aborting the report.
func buildUnfledged(r *repo.Repo, set *spec.Set) unfledgedReport {
	rep := unfledgedReport{
		Plumage:  []unfledgedItem{},
		Feathers: []unfledgedItem{},
		Issues:   []string{},
	}

	for _, req := range set.Reqs {
		if req.Status == spec.ReqFledged {
			continue
		}
		rep.Plumage = append(rep.Plumage, unfledgedItem{
			ID: req.ID, Status: req.Status, Priority: req.Priority,
			Title: req.Title, Path: relPath(r.Root, req.Path),
		})
	}

	for _, t := range set.Tasks {
		if t.Status == spec.TaskFledged {
			continue
		}
		rep.Feathers = append(rep.Feathers, unfledgedItem{
			ID: t.ID, Status: t.Status, Priority: t.Priority,
			Title: t.Title, Plumage: t.Requirement, Oversight: t.Oversight,
			Path: relPath(r.Root, t.Path),
		})
	}

	sortUnfledged(rep.Plumage)
	sortUnfledged(rep.Feathers)

	for _, fe := range set.Errors {
		rep.Issues = append(rep.Issues, fmt.Sprintf("%s: %v", relPath(r.Root, fe.Path), fe.Err))
	}
	sort.Strings(rep.Issues)

	return rep
}

// sortUnfledged orders items by priority, then ID (identical to `fledge ready`).
func sortUnfledged(items []unfledgedItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority < items[j].Priority
		}
		return items[i].ID < items[j].ID
	})
}

func printUnfledged(rep unfledgedReport, showPlumage, showFeathers bool) {
	if showPlumage {
		fmt.Println("Plumage:")
		if len(rep.Plumage) == 0 {
			fmt.Println("  (none)")
		}
		for _, it := range rep.Plumage {
			fmt.Printf("  %s  %s  %s  %s\n", it.ID, it.Status, it.Priority, it.Title)
		}
	}
	if showFeathers {
		fmt.Println("Feathers:")
		if len(rep.Feathers) == 0 {
			fmt.Println("  (none)")
		}
		for _, it := range rep.Feathers {
			fmt.Printf("  %s  %s  %s  %s  (plumage %s)\n", it.ID, it.Status, it.Priority, it.Title, it.Plumage)
		}
	}
	if len(rep.Issues) > 0 {
		fmt.Println("Issues:")
		for _, is := range rep.Issues {
			fmt.Printf("  %s\n", is)
		}
	}
}
