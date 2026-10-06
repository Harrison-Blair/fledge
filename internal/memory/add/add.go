// Package add implements memory add: storing one new fact in the primary
// checkout's memories and regenerating their index.
package add

import (
	"context"
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
)

type Options struct {
	Name, Description, Type, Body, File string
	BodySet, FileSet                    bool
}

// Run validates and stores a new memory without contacting Herdr.
func Run(ctx context.Context, cwd string, o Options, in io.Reader) libagent.Outcome {
	out := libagent.NewOutcome("memory.add")
	body, err := libagent.ReadText(in, libagent.TextInput{Body: o.Body, BodyFlag: "body", BodySet: o.BodySet, File: o.File, FileFlag: "file", FileSet: o.FileSet, Required: true, Noun: "body"})
	m := memory.Memory{Name: o.Name, Description: o.Description, Type: o.Type, Body: body}
	if err == nil {
		err = memory.Validate(m)
	}
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	if err := memory.Add(ctx, cwd, m, &out); err != nil {
		out.Fail(err, memory.Phase(err), false)
		return out
	}
	out.Result = m
	return out
}

// Render writes a successful addition.
func Render(w io.Writer, o libagent.Outcome) error {
	m, ok := o.Result.(memory.Memory)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := fmt.Fprintf(w, "Added memory %s: %s\n", m.Name, m.Description)
	return err
}
