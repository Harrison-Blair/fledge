package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/graph"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() {
	register("set", runSet, "fledge set <ID> <field> <value> [--json]  (fields: priority, oversight, depends_on, title)")
}

func runSet(args []string) int {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 3 {
		return usageErr("usage: fledge set <ID> <field> <value>")
	}
	id, field, value := positional[0], positional[1], positional[2]

	switch field {
	case "status":
		return fail("status is not settable here; use `fledge status %s <new-status>`", id)
	case "id", "requirement", "authored", "agent", "fledge_version":
		return fail("cannot set %s: field is immutable", field)
	case "priority", "oversight", "depends_on", "title":
	default:
		return usageErr("unknown field %q (settable: priority, oversight, depends_on, title)")
	}

	_, set, _, code, ok := loadSet()
	if !ok {
		return code
	}
	task := set.Task(id)
	req := set.Req(id)
	if task == nil && req == nil {
		return fail("%s not found", id)
	}

	switch field {
	case "priority":
		if !slices.Contains(spec.Priorities, value) {
			return fail("priority %q not one of %s", value, strings.Join(spec.Priorities, "|"))
		}
		if task != nil {
			task.Priority = value
		} else {
			req.Priority = value
		}
	case "oversight":
		if task == nil {
			return fail("oversight applies to tasks only")
		}
		if value == "none" {
			task.Oversight = ""
		} else if slices.Contains(spec.OversightValues, value) {
			task.Oversight = value
		} else {
			return fail("oversight %q not one of merge|during|none", value)
		}
	case "depends_on":
		if task == nil {
			return fail("depends_on applies to tasks only")
		}
		var deps []string
		if value != "none" && value != "" {
			for _, d := range strings.Split(value, ",") {
				d = strings.TrimSpace(d)
				if d == task.ID {
					return fail("depends_on cannot reference itself")
				}
				if set.Task(d) == nil {
					return fail("depends_on references unknown %s", d)
				}
				deps = append(deps, d)
			}
		}
		old := task.DependsOn
		task.DependsOn = deps
		if c := graph.New(set.Tasks).Cycle(); c != nil {
			task.DependsOn = old
			return fail("edit would create a dependency cycle: %s", strings.Join(c, " -> "))
		}
	case "title":
		if task != nil {
			task.Title = value
			task.Body = retitleHeading(task.Body, task.ID, value)
		} else {
			req.Title = value
			req.Body = retitleHeading(req.Body, req.ID, value)
		}
	}

	var save func() error
	var path string
	if task != nil {
		save, path = task.Save, task.Path
	} else {
		save, path = req.Save, req.Path
	}
	if err := save(); err != nil {
		return fail("%v", err)
	}
	if field == "title" {
		fmt.Fprintf(os.Stderr, "note: filename %s no longer matches the title\n",
			filepath.Base(path))
	}
	if *jsonOut {
		return emitJSON(map[string]string{"id": id, "field": field, "value": value})
	}
	fmt.Printf("%s: %s = %s\n", id, field, value)
	return ExitOK
}

// retitleHeading rewrites the `# <ID>: <title>` heading line, if present.
func retitleHeading(body []byte, id, title string) []byte {
	lines := strings.Split(string(body), "\n")
	prefix := "# " + id + ":"
	for i, l := range lines {
		if strings.HasPrefix(l, prefix) {
			lines[i] = prefix + " " + title
			break
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
