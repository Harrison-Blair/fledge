package spawn

import (
	"context"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// register records the started agent a. A failure is reported on the result
// without failing the spawn, since the agent is already running.
func (s *spawner) register(ctx context.Context, a herdr.AgentDetails, out *libagent.Outcome) {
	result := out.Result.(*Result)
	store, err := identity.OpenStore(ctx, s.Cwd, out)
	var rec identity.Record
	if err == nil {
		rec, err = identity.Register(ctx, store, s.Client, a, "spawn", result.WorktreePath)
	}
	if err != nil {
		reason := err.Error()
		result.RegistrationError = &reason
		return
	}
	result.ID, result.Registered = &rec.ID, true
	out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "agent_record", ID: rec.ID})
}
