package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() {
	register("new", runNew,
		"fledge new plumage --title <t> [--priority P1] [--agent <s>] [--json]\n"+
			"  fledge new feather --title <t> --plumage PLM-### [--depends-on a,b] [--priority P1] [--oversight merge|during] [--force] [--json]")
}

const defaultAgent = "fledge-orchestrate/planning"

func runNew(args []string) int {
	if len(args) == 0 || (args[0] != "plumage" && args[0] != "feather") {
		return usageErr("usage: fledge new plumage|feather ...")
	}
	kind := args[0]

	fs := flag.NewFlagSet("new "+kind, flag.ContinueOnError)
	title := fs.String("title", "", "spec title (required)")
	priority := fs.String("priority", "P1", "priority P0..P3")
	agent := fs.String("agent", defaultAgent, "authoring agent recorded in frontmatter")
	reqID := fs.String("plumage", "", "parent plumage (feather only)")
	dependsOn := fs.String("depends-on", "", "comma-separated feather IDs (feather only)")
	oversight := fs.String("oversight", "", "merge|during (feather only)")
	force := fs.Bool("force", false, "allow linking to an egg plumage")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if *title == "" {
		return usageErr("--title is required")
	}
	if !slices.Contains(spec.Priorities, *priority) {
		return usageErr("priority %q not one of %s", *priority, strings.Join(spec.Priorities, "|"))
	}
	if *oversight != "" && !slices.Contains(spec.OversightValues, *oversight) {
		return usageErr("oversight %q not one of %s", *oversight, strings.Join(spec.OversightValues, "|"))
	}

	r, set, _, code, ok := loadSet()
	if !ok {
		return code
	}
	authored := time.Now().UTC().Format(time.RFC3339)
	version := r.Version(binaryVersion)

	var id, path string
	var err error
	switch kind {
	case "plumage":
		dir := r.RequirementsDir()
		id, path, err = spec.AllocateAndCreate(dir, "PLM", func(id string) (string, []byte) {
			path := filepath.Join(dir, fmt.Sprintf("%s-%s.md", id, spec.Kebab(*title)))
			req := &spec.Requirement{
				ID: id, Title: *title, Status: spec.ReqEgg, Priority: *priority,
				Authored: authored, Agent: *agent, FledgeVersion: version,
				Body: spec.RequirementBody(id, *title),
			}
			return path, req.Render()
		})
		if err != nil {
			return fail("%v", err)
		}
	case "feather":
		if *reqID == "" {
			return usageErr("--plumage is required for feathers")
		}
		parent := set.Req(*reqID)
		if parent == nil {
			return fail("plumage %s does not exist", *reqID)
		}
		if parent.Status == spec.ReqEgg && !*force {
			return fail("plumage %s is still an egg; hatch it first or pass --force", *reqID)
		}
		var deps []string
		if *dependsOn != "" {
			for _, d := range strings.Split(*dependsOn, ",") {
				deps = append(deps, strings.TrimSpace(d))
			}
		}
		allDone := true
		for _, d := range deps {
			dep := set.Task(d)
			if dep == nil {
				return fail("depends_on references unknown %s", d)
			}
			if dep.Status != spec.TaskFledged {
				allDone = false
			}
		}
		status := spec.TaskEgg
		if allDone {
			status = spec.TaskPipping
		}
		dir := r.TasksDir()
		id, path, err = spec.AllocateAndCreate(dir, "FTHR", func(id string) (string, []byte) {
			path := filepath.Join(dir, fmt.Sprintf("%s-%s.md", id, spec.Kebab(*title)))
			task := &spec.Task{
				ID: id, Title: *title, Requirement: *reqID, Status: status,
				Priority: *priority, DependsOn: deps, Oversight: *oversight,
				Authored: authored, Agent: *agent, FledgeVersion: version,
				Body: spec.TaskBody(id, *title, *reqID),
			}
			return path, task.Render()
		})
		if err != nil {
			return fail("%v", err)
		}
	}

	rel := relPath(r.Root, path)
	if *jsonOut {
		return emitJSON(map[string]string{"id": id, "path": rel})
	}
	fmt.Printf("created %s\n", rel)
	return ExitOK
}
