package spawn

import (
	"context"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// register records the started agent a with the name of any profile it was
// spawned with, and a's Herdr-reported session ref when it carries one. A
// failure is reported on the result, and a failed session write as a warning
// effect, without failing the spawn, since the agent is already running.
func (s *spawner) register(ctx context.Context, a herdr.AgentDetails, out *libagent.Outcome) {
	result := out.Result.(*Result)
	store, err := identity.OpenStore(ctx, s.Cwd, out)
	var rec identity.Record
	var profile *string
	if result.Profile != nil {
		profile = &result.Profile.Name
	}
	if err == nil {
		rec, err = identity.Register(ctx, store, s.Client, a, "spawn", s.checkout, profile)
	}
	if err != nil {
		reason := err.Error()
		result.RegistrationError = &reason
		return
	}
	result.ID, result.Registered = &rec.ID, true
	out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "agent_record", ID: rec.ID})
	if a.AgentSession == nil {
		return
	}
	switch _, changed, err := observeSession(store, rec.ID, *a.AgentSession, s.now()); {
	case err != nil:
		out.Effects = append(out.Effects, libagent.Effect{Action: "warning", Kind: "native_session", ID: rec.ID})
	case changed:
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "native_session", ID: rec.ID})
	}
}

// observeSession is replaceable so tests can fail its write.
var observeSession = identity.ObserveSession

// withHarness returns a with the requested harness when Herdr has not
// classified the agent yet, as after agent.start or an early agent.wait, so a
// different harness later in this terminal is not attributed to its record.
func withHarness(a herdr.AgentDetails, harness string) herdr.AgentDetails {
	if a.Agent == nil {
		a.Agent = &harness
	}
	return a
}
