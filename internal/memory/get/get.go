// Package get implements memory get: one memory in full.
package get

import (
	"context"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
)

type Options struct{ Name string }

// Run reads one memory without contacting Herdr.
func Run(ctx context.Context, cwd string, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "memory.get", Status: "success", Effects: []libagent.Effect{}}
	if err := memory.ValidateName(o.Name); err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	dir, err := memory.Dir(ctx, cwd)
	if err != nil {
		out.Fail(err, "state", false)
		return out
	}
	m, err := memory.Get(dir, o.Name)
	if err != nil {
		out.Fail(err, memory.Phase(err), false)
		return out
	}
	out.Result = m
	return out
}

// Render writes the memory as its file.
func Render(w io.Writer, o libagent.Outcome) error {
	m, ok := o.Result.(memory.Memory)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := w.Write(memory.Format(m))
	return err
}
