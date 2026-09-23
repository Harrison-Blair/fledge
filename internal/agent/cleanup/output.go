package cleanup

import (
	"fmt"
	"io"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

var (
	workerVerbs   = map[string]string{"planned": "stop", "done": "stopped", "skipped": "hold", "failed": "failed"}
	checkoutVerbs = map[string]string{"planned": "remove", "done": "removed", "skipped": "keep", "failed": "failed"}
)

// Render writes a cleanup plan or its results, one line per worker and
// checkout with the reason for any not acted on. It also runs after a failure.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if !ok {
		return nil
	}
	if len(r.Workers) == 0 && len(r.Checkouts) == 0 {
		_, err := fmt.Fprintln(w, "No spawned workers or created checkouts to clean up.")
		return err
	}
	want := "done"
	format := "Stopped %d of %d workers and removed %d of %d checkouts.\n"
	if r.DryRun {
		want, format = "planned", "Dry run: would stop %d of %d workers and remove %d of %d checkouts.\n"
	}
	type row struct{ verb, subject string }
	var rows []row
	var stopping, removing int
	reason := func(s string, r *string) string {
		if r != nil {
			return s + ": " + *r
		}
		return s
	}
	for _, x := range r.Workers {
		if x.Outcome == want {
			stopping++
		}
		rows = append(rows, row{workerVerbs[x.Outcome], reason(libagent.Display(x.Name)+" ("+x.ID+")", x.Reason)})
	}
	for _, x := range r.Checkouts {
		if x.Outcome == want {
			removing++
		}
		detail := "detached HEAD"
		if x.Branch != nil {
			detail = "branch " + *x.Branch
		}
		if x.Base != nil {
			detail += ", from " + *x.Base
		}
		rows = append(rows, row{checkoutVerbs[x.Outcome], reason(x.Path+" ("+detail+")", x.Reason)})
	}
	if _, err := fmt.Fprintf(w, format, stopping, len(r.Workers), removing, len(r.Checkouts)); err != nil {
		return err
	}
	width := 0
	for _, x := range rows {
		width = max(width, len(x.verb))
	}
	for _, x := range rows {
		if _, err := fmt.Fprintf(w, "  %-*s  %s\n", width, x.verb, x.subject); err != nil {
			return err
		}
	}
	return nil
}
