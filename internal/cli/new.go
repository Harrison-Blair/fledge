package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() {
	register("new", runNew,
		"fledge new req --title <t> [--priority P1] [--agent <s>] [--json]\n"+
			"  fledge new task --title <t> --req REQ-### [--depends-on a,b] [--priority P1] [--oversight merge|during] [--force] [--json]")
}

const defaultAgent = "fledge-orchestrate/planning"

func runNew(args []string) int {
	if len(args) == 0 || (args[0] != "req" && args[0] != "task") {
		return usageErr("usage: fledge new req|task ...")
	}
	kind := args[0]

	fs := flag.NewFlagSet("new "+kind, flag.ContinueOnError)
	title := fs.String("title", "", "spec title (required)")
	priority := fs.String("priority", "P1", "priority P0..P3")
	agent := fs.String("agent", defaultAgent, "authoring agent recorded in frontmatter")
	reqID := fs.String("req", "", "parent requirement (task only)")
	dependsOn := fs.String("depends-on", "", "comma-separated task IDs (task only)")
	oversight := fs.String("oversight", "", "merge|during (task only)")
	force := fs.Bool("force", false, "allow linking to a draft requirement")
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
	var content []byte
	switch kind {
	case "req":
		dir := r.RequirementsDir()
		var err error
		if id, err = spec.NextID(dir, "REQ"); err != nil {
			return fail("%v", err)
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%s.md", id, spec.Kebab(*title)))
		req := &spec.Requirement{
			ID: id, Title: *title, Status: spec.ReqDraft, Priority: *priority,
			Authored: authored, Agent: *agent, FledgeVersion: version,
			Body: spec.RequirementBody(id, *title),
		}
		content = req.Render()
	case "task":
		if *reqID == "" {
			return usageErr("--req is required for tasks")
		}
		parent := set.Req(*reqID)
		if parent == nil {
			return fail("requirement %s does not exist", *reqID)
		}
		if parent.Status == spec.ReqDraft && !*force {
			return fail("requirement %s is still draft; approve it first or pass --force", *reqID)
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
			if dep.Status != spec.TaskDone {
				allDone = false
			}
		}
		status := spec.TaskBlocked
		if allDone {
			status = spec.TaskReady
		}
		dir := r.TasksDir()
		var err error
		if id, err = spec.NextID(dir, "TASK"); err != nil {
			return fail("%v", err)
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%s.md", id, spec.Kebab(*title)))
		task := &spec.Task{
			ID: id, Title: *title, Requirement: *reqID, Status: status,
			Priority: *priority, DependsOn: deps, Oversight: *oversight,
			Authored: authored, Agent: *agent, FledgeVersion: version,
			Body: spec.TaskBody(id, *title, *reqID),
		}
		content = task.Render()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail("%v", err)
	}
	// O_EXCL: a concurrent allocation of the same ID fails loudly.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fail("%v", err)
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return fail("%v", err)
	}
	if err := f.Close(); err != nil {
		return fail("%v", err)
	}

	rel := relPath(r.Root, path)
	if *jsonOut {
		return emitJSON(map[string]string{"id": id, "path": rel})
	}
	fmt.Printf("created %s\n", rel)
	return ExitOK
}
