// Package models implements agent models: listing locally discoverable models.
package models

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	libmodels "github.com/Harrison-Blair/fledge/internal/lib/models"
)

type Options struct {
	Harness string
}
type Result struct {
	Models []libmodels.Row `json:"models"`
}

// Run lists locally discoverable models, optionally limited to one harness kind.
func Run(ctx context.Context, d libmodels.Discovery, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.models", Status: "success", Effects: []libagent.Effect{}}
	if o.Harness != "" {
		if err := libagent.ValidateHarness(o.Harness); err != nil {
			out.Fail(err, "validation", false)
			return out
		}
	}
	rows := []libmodels.Row{}
	for _, kind := range libmodels.ModelHarnesses() {
		if o.Harness != "" && kind != o.Harness {
			continue
		}
		found, err := d.Discover(ctx, kind)
		if err != nil {
			continue
		}
		rows = append(rows, found...)
	}
	slices.SortFunc(rows, func(a, b libmodels.Row) int {
		return cmp.Or(strings.Compare(a.Harness, b.Harness), strings.Compare(a.Model, b.Model))
	})
	out.Result = Result{Models: rows}
	return out
}

// Render writes a successful models outcome as a table.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	if len(r.Models) == 0 {
		_, err := fmt.Fprintln(w, "No models discovered.")
		return err
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "HARNESS\tMODEL\tNAME")
	for _, m := range r.Models {
		fmt.Fprintf(table, "%s\t%s\t%s\n", m.Harness, m.Model, display(m.Name))
	}
	return table.Flush()
}
func display(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
