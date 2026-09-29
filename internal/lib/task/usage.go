package task

import (
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/usage"
)

// Usage holds historical snapshots: the worker's, taken at completion, and
// the latest verifier's, taken at verification, by Fledge versions that
// recorded them. Records keep them readable; nothing writes new ones.
type Usage struct {
	Worker   *UsageSnapshot `json:"worker"`
	Verifier *UsageSnapshot `json:"verifier"`
}

// UsageSnapshot is one agent's harness usage between two task timestamps.
// AgentID is null for an unregistered verifier. ElapsedSeconds comes from the
// task timestamps, not the harness. Reason explains an unavailable basis and
// any caveat on a measured one.
type UsageSnapshot struct {
	AgentID        *string       `json:"agent_id"`
	Harness        *string       `json:"harness"`
	Session        *UsageSession `json:"session"`
	Window         UsageWindow   `json:"window"`
	ElapsedSeconds int64         `json:"elapsed_seconds"`
	Tokens         usage.Tokens  `json:"tokens"`
	Cost           *usage.Cost   `json:"cost"`
	Models         []string      `json:"models"`
	Turns          int           `json:"turns"`
	Basis          string        `json:"basis"`
	Reason         *string       `json:"reason"`
	CollectedAt    string        `json:"collected_at"`
}

// UsageSession is the native session ref a snapshot read.
type UsageSession struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// UsageWindow is the pair of task timestamps a snapshot covers.
type UsageWindow struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Observer is identity.ObserveSession, replaceable in tests.
type Observer func(s *state.Store, id string, session herdr.AgentSession, now time.Time) (identity.Record, bool, error)

// Observe stores live's session ref on rec through observe, as seen at now,
// and returns the updated record. It is best effort: a failed write is a
// warning effect and rec is returned as it was.
func Observe(s *state.Store, observe Observer, rec identity.Record, live *herdr.AgentDetails, now time.Time, out *libagent.Outcome) identity.Record {
	if live == nil || live.AgentSession == nil {
		return rec
	}
	updated, changed, err := observe(s, rec.ID, *live.AgentSession, now)
	switch {
	case err != nil:
		out.Effects = append(out.Effects, libagent.Effect{Action: "warning", Kind: "native_session", ID: rec.ID})
		return rec
	case changed:
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "native_session", ID: rec.ID})
	}
	return updated
}
