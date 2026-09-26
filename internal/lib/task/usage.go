package task

import (
	"context"
	"strings"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/usage"
)

// Usage holds the worker's snapshot, taken at completion, and the latest
// verifier's, taken at verification.
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

// Reader is usage.Read with its Discovery bound.
type Reader func(ctx context.Context, kind string, ref usage.Ref, w usage.Window) usage.Summary

// LocalReader reads the real harness stores.
func LocalReader(ctx context.Context, kind string, ref usage.Ref, w usage.Window) usage.Summary {
	return usage.Read(ctx, usage.LocalDiscovery(), kind, ref, w)
}

// CollectUsage reads agent's usage between task timestamps from and to. The
// session ref and harness come from live, the agent's live details, else
// from the record; cwd is the last fallback for the session's working
// directory. A nil agent, or one with no ref, is unavailable. Notes lead the
// reason. It never fails: reader problems become an unavailable basis.
func CollectUsage(ctx context.Context, read Reader, agent *identity.Record, live *herdr.AgentDetails, cwd, from, to string, now time.Time, notes ...string) *UsageSnapshot {
	start, end := parseTime(from), parseTime(to)
	u := &UsageSnapshot{Window: UsageWindow{From: from, To: to}, CollectedAt: now.UTC().Format(time.RFC3339Nano)}
	if start != nil && end != nil {
		u.ElapsedSeconds = int64(end.Sub(*start).Seconds())
	}
	reasons := notes
	var harness, kind, value string
	var cwds []*string
	if agent != nil {
		u.AgentID = &agent.ID
		if live != nil {
			harness = deref(live.Agent)
			cwds = append(cwds, live.ForegroundCwd, live.Cwd)
			if s := live.AgentSession; s != nil && deref(s.Value) != "" {
				kind, value = deref(s.Kind), *s.Value
			}
		}
		if n := agent.NativeSession; value == "" && n != nil {
			kind, value = n.Kind, n.Value
			if harness == "" {
				harness = n.Harness
			}
		}
		if harness == "" {
			harness = deref(agent.Harness)
		}
		cwds = append(cwds, agent.WorktreePath, &cwd)
	}
	if harness != "" {
		u.Harness = &harness
	}
	switch {
	case agent == nil:
		u.Basis = usage.Unavailable
	case value == "":
		u.Basis = usage.Unavailable
		reasons = append(reasons, "no native session ref observed")
	default:
		u.Session = &UsageSession{Kind: kind, Value: value}
		ref := usage.Ref{Kind: kind, Value: value}
		for _, c := range cwds {
			if ref.Cwd = deref(c); ref.Cwd != "" {
				break
			}
		}
		s := read(ctx, harness, ref, usage.Window{From: start, To: end})
		u.Tokens, u.Cost, u.Models, u.Turns, u.Basis = s.Tokens, s.Cost, s.Models, s.Turns, s.Basis
		if s.Reason != "" {
			reasons = append(reasons, s.Reason)
		}
	}
	if len(reasons) > 0 {
		reason := strings.Join(reasons, "; ")
		u.Reason = &reason
	}
	return u
}

// Observer is identity.ObserveSession, replaceable in tests.
type Observer func(s *state.Store, id string, session herdr.AgentSession, now time.Time) (identity.Record, bool, error)

// Observe stores live's session ref on rec through observe and returns the
// updated record. It is best effort: a failed write is a warning effect and
// rec is returned as it was.
func Observe(s *state.Store, observe Observer, rec identity.Record, live *herdr.AgentDetails, out *libagent.Outcome) identity.Record {
	if live == nil || live.AgentSession == nil {
		return rec
	}
	updated, changed, err := observe(s, rec.ID, *live.AgentSession, time.Now())
	switch {
	case err != nil:
		out.Effects = append(out.Effects, libagent.Effect{Action: "warning", Kind: "native_session", ID: rec.ID})
		return rec
	case changed:
		out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "native_session", ID: rec.ID})
	}
	return updated
}

func parseTime(s string) *time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	return &t
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
