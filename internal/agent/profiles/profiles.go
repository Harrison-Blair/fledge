// Package profiles implements agent profiles: listing effective launch
// profiles or showing one resolved profile, without contacting Herdr.
package profiles

import (
	"context"
	"fmt"
	"io"
	"strconv"
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
	out := libagent.Outcome{Operation: "agent.profiles", Status: "success", Effects: []libagent.Effect{}}
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

// Render writes a profile table, or one profile's resolved settings.
func Render(w io.Writer, o libagent.Outcome) error {
	if o.Error != nil {
		return nil
	}
	switch r := o.Result.(type) {
	case ListResult:
		table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "NAME\tHARNESS\tMODEL\tSOURCE")
		for _, p := range r.Profiles {
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", p.Name, orDash(p.Harness), orDash(p.Model), source(p))
		}
		return table.Flush()
	case ShowResult:
		p := r.Profile
		var b strings.Builder
		fmt.Fprintf(&b, "Profile %s\n  source: %s\n", p.Name, source(p))
		if p.Base != nil {
			fmt.Fprintf(&b, "  extends: %s\n", *p.Base)
		}
		args := []string{}
		for _, a := range p.Args {
			args = append(args, strconv.Quote(a))
		}
		fmt.Fprintf(&b, "  harness: %s\n  model: %s\n  args: %s\n", orDash(p.Harness), orDash(p.Model), orDash(strings.Join(args, " ")))
		if p.Brief() == "" {
			b.WriteString("  role: -\n")
		} else {
			b.WriteString("  role:\n")
			for _, line := range strings.Split(p.Brief(), "\n") {
				if line != "" {
					line = "    " + line
				}
				b.WriteString(line + "\n")
			}
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

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
