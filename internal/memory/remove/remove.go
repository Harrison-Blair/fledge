// Package remove implements memory remove: deleting one memory and
// regenerating the index.
package remove

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
)

type Options struct{ Name string }

// Result names the removed memory.
type Result struct {
	Name string `json:"name"`
}

// Run deletes one memory without contacting Herdr.
func Run(ctx context.Context, cwd string, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "memory.remove", Status: "success", Effects: []libagent.Effect{}}
	if err := memory.ValidateName(o.Name); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	if err := memory.Remove(ctx, cwd, o.Name, &out); err != nil {
		out.Fail(err, memory.Phase(err), false)
		return out
	}
	out.Result = Result{Name: o.Name}
	return out
}

// Render writes a successful removal.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Removed memory %s\n", r.Name)
	return err
}
