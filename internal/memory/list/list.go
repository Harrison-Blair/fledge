// Package list implements memory list: the index entries of the primary
// checkout's memories, optionally of one type.
package list

import (
	"context"
	"io"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
)

// Options keeps only memories of Type when it is set.
type Options struct{ Type string }

// Entry is one memory without its body.
type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}
type Result struct {
	Memories []Entry `json:"memories"`
}

// Run reads the memories, ordered by name, without contacting Herdr.
func Run(ctx context.Context, cwd string, o Options) cli.Outcome {
	out := cli.NewOutcome("memory.list")
	if o.Type != "" {
		if err := memory.ValidateType(o.Type); err != nil {
			out.Fail(err, "validation", false)
			return out
		}
	}
	dir, err := memory.Dir(ctx, cwd)
	var ms []memory.Memory
	if err == nil {
		ms, err = memory.List(dir)
	}
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	entries := []Entry{}
	for _, m := range ms {
		if o.Type == "" || m.Type == o.Type {
			entries = append(entries, Entry{Name: m.Name, Description: m.Description, Type: m.Type})
		}
	}
	out.Result = Result{Memories: entries}
	return out
}

// Render writes the entries as index lines.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	if len(r.Memories) == 0 {
		_, err := io.WriteString(w, "No memories.\n")
		return err
	}
	ms := []memory.Memory{}
	for _, e := range r.Memories {
		ms = append(ms, memory.Memory{Name: e.Name, Description: e.Description})
	}
	_, err := io.WriteString(w, memory.Index(ms))
	return err
}
