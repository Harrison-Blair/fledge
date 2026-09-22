// Package list implements task list: every task, oldest first, optionally
// filtered by status, owner, parent, or readiness to assign.
package list

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Options filters by status, by owner agent id, and by parent task id (direct
// children only) when set. Ready keeps created tasks whose prerequisites are
// all satisfied.
type Options struct {
	Status, Owner, Parent string
	Ready                 bool
}

// Row is a task with its owner's name while the owner has a live record, the
// progress of its direct children (null when it has none), and its unmet
// prerequisites.
type Row struct {
	task.Record
	OwnerName *string        `json:"owner_name"`
	Progress  *task.Progress `json:"progress"`
	Waiting   []string       `json:"waiting"`
}
type Result struct {
	Tasks []Row `json:"tasks"`
}

// Run reads tasks from the store without contacting Herdr.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "task.list", Status: "success", Effects: []libagent.Effect{}}
	var err error
	switch {
	case o.Status != "" && !slices.Contains(task.Statuses, o.Status):
		err = libagent.Invalid("--status must be one of %v", task.Statuses)
	case o.Owner != "":
		if task.ValidateID(o.Owner) != nil {
			err = libagent.Invalid("--owner must be an 8 lowercase hexadecimal agent id")
		}
	}
	if err == nil && o.Parent != "" && task.ValidateID(o.Parent) != nil {
		err = libagent.Invalid("--parent must be an 8 lowercase hexadecimal task id")
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	rows, err := load(ctx, c.Cwd, o)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	out.Result = Result{Tasks: rows}
	return out
}

func load(ctx context.Context, cwd string, o Options) ([]Row, error) {
	rows := []Row{}
	s, err := task.Existing(ctx, cwd)
	if err != nil || s == nil {
		return rows, err
	}
	tasks, err := task.List(s)
	if err != nil {
		return nil, err
	}
	live, err := identity.LiveByTerminal(s)
	if err != nil {
		return nil, err
	}
	names := map[string]*string{}
	for _, rec := range live {
		names[rec.ID] = rec.Name
	}
	byID := task.Index(tasks)
	for _, r := range tasks {
		waiting := task.Unmet(r, byID)
		if o.Ready && (r.Status != task.Created || len(waiting) > 0) {
			continue
		}
		if o.Status != "" && r.Status != o.Status || o.Owner != "" && (r.Owner == nil || *r.Owner != o.Owner) || o.Parent != "" && (r.Parent == nil || *r.Parent != o.Parent) {
			continue
		}
		row := Row{Record: r, Progress: task.ChildProgress(r.ID, tasks), Waiting: waiting}
		if r.Owner != nil {
			row.OwnerName = names[*r.Owner]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Render writes a successful list as a table.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	if len(r.Tasks) == 0 {
		_, err := fmt.Fprintln(w, "No tasks.")
		return err
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tSTATUS\tOWNER\tPARENT\tWAITING\tPROGRESS\tTITLE")
	for _, t := range r.Tasks {
		owner := "-"
		switch {
		case t.OwnerName != nil:
			owner = *t.OwnerName
		case t.Owner != nil:
			owner = *t.Owner
		}
		waiting := strings.Join(t.Waiting, ",")
		if waiting == "" {
			waiting = "-"
		}
		progress := "-"
		if t.Progress != nil {
			progress = t.Progress.String()
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", t.ID, t.Status, owner, libagent.Display(t.Parent), waiting, progress, t.Title)
	}
	return table.Flush()
}
