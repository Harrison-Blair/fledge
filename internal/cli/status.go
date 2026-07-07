package cli

import (
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() { register("status", runStatus, "fledge status <ID> [<new-status>] [--force] [--json]") }

// Legal transitions. Keys are "from", values are allowed "to".
var (
	taskTransitions = map[string][]string{
		spec.TaskBlocked:    {spec.TaskInProgress},
		spec.TaskReady:      {spec.TaskInProgress},
		spec.TaskInProgress: {spec.TaskDone, spec.TaskReady},
	}
	reqTransitions = map[string][]string{
		spec.ReqDraft:    {spec.ReqApproved},
		spec.ReqApproved: {spec.ReqDone, spec.ReqDraft},
	}
	taskStatuses = []string{spec.TaskBlocked, spec.TaskReady, spec.TaskInProgress, spec.TaskDone}
	reqStatuses  = []string{spec.ReqDraft, spec.ReqApproved, spec.ReqDone}
)

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	force := fs.Bool("force", false, "bypass transition legality (not enum validity)")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) < 1 || len(positional) > 2 {
		return usageErr("usage: fledge status <ID> [<new-status>]")
	}
	id := positional[0]

	_, set, _, code, ok := loadSet()
	if !ok {
		return code
	}

	var current *string
	var enum []string
	var transitions map[string][]string
	var save func() error
	var body []byte
	if t := set.Task(id); t != nil {
		current, enum, transitions, save, body = &t.Status, taskStatuses, taskTransitions, t.Save, t.Body
	} else if req := set.Req(id); req != nil {
		current, enum, transitions, save, body = &req.Status, reqStatuses, reqTransitions, req.Save, req.Body
	} else {
		return fail("%s not found", id)
	}

	if len(positional) == 1 {
		if *jsonOut {
			return emitJSON(map[string]string{"id": id, "status": *current})
		}
		fmt.Printf("%s: %s\n", id, *current)
		return ExitOK
	}

	next := positional[1]
	if !slices.Contains(enum, next) {
		return fail("status %q not one of %s", next, strings.Join(enum, "|"))
	}
	if !*force && !slices.Contains(transitions[*current], next) {
		return fail("illegal transition %s -> %s (use --force to override)", *current, next)
	}
	if next == spec.ReqDone && !*force {
		if unchecked := uncheckedCriteria(body); len(unchecked) > 0 {
			return fail("%s: acceptance criteria unchecked: %s (use --force to override)", id, strings.Join(unchecked, ", "))
		}
	}
	from := *current
	*current = next
	if err := save(); err != nil {
		return fail("%v", err)
	}
	if *jsonOut {
		return emitJSON(map[string]string{"id": id, "from": from, "to": next})
	}
	fmt.Printf("%s: %s -> %s\n", id, from, next)
	return ExitOK
}

// setTaskStatus is shared with lock/unlock: transition a task uncondition-
// ally and save. Returns the previous status.
func setTaskStatus(t *spec.Task, next string) (string, error) {
	from := t.Status
	t.Status = next
	return from, t.Save()
}
