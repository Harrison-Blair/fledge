// Package profiles implements agent profiles: listing effective role
// profiles or showing one profile's brief, without contacting Herdr.
package profiles

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	libprofiles "github.com/Harrison-Blair/fledge/internal/lib/profiles"
)

// Options selects one profile to show; an empty Name lists them all.
type Options struct{ Name string }

// ListResult holds every effective profile, sorted by name.
type ListResult struct {
	Profiles []libprofiles.Profile `json:"profiles"`
}

// ShowResult holds one resolved profile.
type ShowResult struct {
	Profile libprofiles.Profile `json:"profile"`
}

// Run resolves profiles for the checkout containing cwd.
func Run(ctx context.Context, cwd string, o Options) libagent.Outcome {
	out := libagent.NewOutcome("agent.profiles")
	if o.Name != "" {
		p, err := libprofiles.Load(ctx, cwd, o.Name)
		if err != nil {
			out.Fail(err, "validation", false)
			return out
		}
		out.Result = ShowResult{Profile: p}
		return out
	}
	list, err := libprofiles.List(ctx, cwd)
	if err != nil {
		out.Fail(err, "validation", false)
		return out
	}
	out.Result = ListResult{Profiles: list}
	return out
}

// Render writes a profile table, or one profile's source and brief.
func Render(w io.Writer, o libagent.Outcome) error {
	if o.Error != nil {
		return nil
	}
	switch r := o.Result.(type) {
	case ListResult:
		table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "NAME\tSOURCE")
		for _, p := range r.Profiles {
			fmt.Fprintf(table, "%s\t%s\n", p.Name, source(p))
		}
		return table.Flush()
	case ShowResult:
		p := r.Profile
		var b strings.Builder
		fmt.Fprintf(&b, "Profile %s\n  source: %s\n  brief:\n", p.Name, source(p))
		for _, line := range strings.Split(p.Brief(), "\n") {
			if line != "" {
				line = "    " + line
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		_, err := io.WriteString(w, b.String())
		return err
	}
	return nil
}

func source(p libprofiles.Profile) string {
	if p.Path != nil {
		return *p.Path
	}
	return "built-in"
}
