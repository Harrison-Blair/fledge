package spawn

import (
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/lib/profiles"
)

// applyProfile fills launch settings the caller left unset from p. An explicit
// harness that differs from the profile's drops the profile's model and args,
// which are specific to its harness; an explicit model or explicit native
// arguments replace the profile's.
func applyProfile(o Options, p profiles.Profile) Options {
	crossHarness := o.Harness != "" && p.Harness != "" && o.Harness != p.Harness
	if o.Harness == "" {
		o.Harness = p.Harness
	}
	if crossHarness {
		return o
	}
	if o.Model == "" {
		o.Model = p.Model
	}
	if len(o.Args) == 0 {
		o.Args = append([]string{}, p.Args...)
	}
	return o
}

// profileBrief renders p with only the reads present under dir, returning
// the missing reads. A missing read never fails a spawn.
func profileBrief(p profiles.Profile, dir string) (string, []string) {
	var present, missing []string
	for _, r := range p.Reads {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(r))); err != nil {
			missing = append(missing, r)
		} else {
			present = append(present, r)
		}
	}
	p.Reads = present
	return p.Brief(), missing
}

// firstPrompt places a profile brief before the task body, separated by a
// blank line, omitting whichever is absent.
func firstPrompt(brief, body string) string {
	switch {
	case brief == "":
		return body
	case body == "":
		return brief
	}
	return brief + "\n\n" + body
}
