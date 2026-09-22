// Package depend implements task depend: adding and removing a task's
// prerequisites without creating a dependency cycle.
package depend

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options names the task and the prerequisite ids to add and remove.
type Options struct {
	ID            string
	After, Remove []string
}

// Run removes then adds prerequisites of a task that is not verified or
// cancelled, under one store lock. Adding a present prerequisite or removing
// an absent one changes nothing. Each added prerequisite must exist and must
// not already depend, directly or indirectly, on the task.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.depend", Status: "success", Effects: []libagent.Effect{}}
	err := task.ValidateID(o.ID)
	switch {
	case err != nil:
	case len(o.After) == 0 && len(o.Remove) == 0:
		err = libagent.Invalid("pass at least one --after or --remove")
	default:
		for _, id := range slices.Concat(o.After, o.Remove) {
			switch {
			case task.ValidateID(id) != nil:
				err = libagent.Invalid("--after and --remove take 8 lowercase hexadecimal task ids")
			case slices.Contains(o.After, id) && slices.Contains(o.Remove, id):
				err = libagent.Invalid("task %s is both added and removed", id)
			}
		}
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	s, err := task.Existing(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	r, err := task.Update(s, o.ID, func(r *task.Record) error {
		if err := task.Require(r, "changing prerequisites", task.Created, task.Assigned, task.Completed); err != nil {
			return err
		}
		rs, err := task.List(s)
		if err != nil {
			return err
		}
		byID := task.Index(rs)
		r.After = slices.DeleteFunc(r.After, func(id string) bool { return slices.Contains(o.Remove, id) })
		for _, id := range o.After {
			if slices.Contains(r.After, id) {
				continue
			}
			if _, ok := byID[id]; !ok {
				return &herdr.Error{Code: "task_not_found", Message: fmt.Sprintf("no task with id %s", id)}
			}
			if id == r.ID {
				return &herdr.Error{Code: "task_dependency_cycle", Message: fmt.Sprintf("task %s cannot run after itself", id)}
			}
			if path := dependsOn(byID, id, r.ID); path != nil {
				return &herdr.Error{Code: "task_dependency_cycle", Message: fmt.Sprintf("task %s cannot run after %s, which already runs after it (%s, each after the next)", r.ID, id, strings.Join(path, " → "))}
			}
			r.After = append(r.After, id)
		}
		return nil
	})
	if err != nil {
		out.Fail(err, "task", false)
		return out
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "task", ID: r.ID})
	out.Result = r
	return out
}

// dependsOn returns the prerequisite chain from task from to task to, or nil
// when from does not depend on to.
func dependsOn(byID map[string]task.Record, from, to string) []string {
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == to {
			path := []string{}
			for ; id != ""; id = prev[id] {
				path = append(path, id)
			}
			slices.Reverse(path)
			return path
		}
		for _, next := range byID[id].After {
			if _, seen := prev[next]; !seen {
				prev[next] = id
				queue = append(queue, next)
			}
		}
	}
	return nil
}

// Render writes the task's prerequisites after a successful change.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(task.Record)
	if o.Error != nil || !ok {
		return nil
	}
	var err error
	if len(r.After) == 0 {
		_, err = fmt.Fprintf(w, "Task %s has no prerequisites.\n", r.ID)
	} else {
		_, err = fmt.Fprintf(w, "Task %s is after %s.\n", r.ID, strings.Join(r.After, ", "))
	}
	return err
}
