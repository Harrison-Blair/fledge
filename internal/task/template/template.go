// Package template implements task template: the brief or proposal skeleton,
// printed without contacting Herdr or the task store.
package template

import (
	"io"

	"github.com/Harrison-Blair/fledge/internal/lib/brief"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/proposal"
)

type Options struct{ Proposal bool }

// Result names the skeleton printed and holds its text.
type Result struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

func Run(o Options) cli.Outcome {
	r := Result{Kind: "brief", Text: brief.Skeleton()}
	if o.Proposal {
		r = Result{Kind: "proposal", Text: proposal.Skeleton()}
	}
	out := cli.NewOutcome("task.template")
	out.Result = r
	return out
}

// Render writes the skeleton text exactly as returned.
func Render(w io.Writer, o cli.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	_, err := io.WriteString(w, r.Text)
	return err
}
