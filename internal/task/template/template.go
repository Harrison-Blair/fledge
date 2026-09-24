// Package template implements task template: the brief or proposal skeleton,
// printed without contacting Herdr or the task store.
package template

import (
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/brief"
	"github.com/Harrison-Blair/fledge/internal/lib/proposal"
)

type Options struct{ Proposal bool }

// Result names the skeleton printed and holds its text.
type Result struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

func Run(o Options) libagent.Outcome {
	r := Result{Kind: "brief", Text: brief.Skeleton()}
	if o.Proposal {
		r = Result{Kind: "proposal", Text: proposal.Skeleton()}
	}
	return libagent.Outcome{Operation: "task.template", Status: "success", Result: r, Effects: []libagent.Effect{}}
}

// Render writes the skeleton text exactly as returned.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := io.WriteString(w, r.Text)
	return err
}
