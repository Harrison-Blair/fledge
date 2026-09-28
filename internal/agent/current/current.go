// Package current implements agent current: the caller's own Fledge record,
// its parent, and the tasks assigned to it.
package current

import (
	"context"
	"errors"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Task is one task the caller owns in the assigned state.
type Task struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Result is the caller's live record, its parent's name while the parent's
// record exists, and its assigned tasks, oldest first.
type Result struct {
	identity.Record
	ParentName *string `json:"parent_name"`
	Tasks      []Task  `json:"tasks"`
}

// Run resolves the agent in the caller's pane to its live record.
func Run(ctx context.Context, c libagent.Client) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.current", Status: "success", Effects: []libagent.Effect{}}
	s, err := identity.Existing(ctx, c.Cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	rec, err := identity.RequireCaller(ctx, s, c)
	if err != nil {
		out.Fail(err, "identity", false)
		return out
	}
	r := Result{Record: rec, Tasks: []Task{}}
	if r.ParentName, err = parentName(s, rec.Parent); err == nil {
		r.Tasks, err = assigned(s, rec.ID)
	}
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	out.Result = r
	return out
}

// parentName is the name on parent's record, ended or not; nil when the
// record or its name is missing.
func parentName(s *state.Store, parent *string) (*string, error) {
	if parent == nil {
		return nil, nil
	}
	var rec identity.Record
	err := s.Get(identity.Kind, *parent, &rec)
	var missing *state.NotFoundError
	if errors.As(err, &missing) {
		return nil, nil
	}
	return rec.Name, err
}

func assigned(s *state.Store, owner string) ([]Task, error) {
	all, err := task.List(s)
	tasks := []Task{}
	for _, r := range all {
		if r.Status == task.Assigned && r.Owner != nil && *r.Owner == owner {
			tasks = append(tasks, Task{ID: r.ID, Title: r.Title})
		}
	}
	return tasks, err
}

// Render writes a successful current outcome as labeled lines.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	parent := libagent.Display(r.Parent)
	if r.ParentName != nil {
		parent += " (" + *r.ParentName + ")"
	}
	for _, f := range []struct{ label, value string }{
		{"Fledge ID", r.ID}, {"Name", libagent.Display(r.Name)}, {"Pane", r.Pane}, {"Workspace ID", r.WorkspaceID},
		{"Harness", libagent.Display(r.Harness)}, {"Worktree", libagent.Display(r.WorktreePath)}, {"Profile", libagent.Display(r.Profile)}, {"Parent", parent},
	} {
		if _, err := fmt.Fprintf(w, "%s: %s\n", f.label, f.value); err != nil {
			return err
		}
	}
	if len(r.Tasks) == 0 {
		_, err := fmt.Fprintln(w, "Assigned tasks: none")
		return err
	}
	if _, err := fmt.Fprintln(w, "Assigned tasks:"); err != nil {
		return err
	}
	for _, t := range r.Tasks {
		if _, err := fmt.Fprintf(w, "  %s  %s\n", t.ID, t.Title); err != nil {
			return err
		}
	}
	return nil
}
