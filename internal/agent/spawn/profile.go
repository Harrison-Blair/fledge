package spawn

import (
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

// firstPrompt places a profile role before the task body, separated by a
// blank line, omitting whichever is absent.
func firstPrompt(role, body string) string {
	switch {
	case role == "":
		return body
	case body == "":
		return role
	}
	return role + "\n\n" + body
}
