package agentcontext

import (
	"errors"
	"math"
	"sort"
	"time"
)

// Deps injects every side effect the collectors need. The zero value is not
// usable; callers build one that wires the real filesystem and export command,
// while tests supply in-memory doubles.
type Deps struct {
	// Home is the user's home directory, the root of every harness's
	// per-user session store.
	Home string
	// Now supplies the report's generation timestamp.
	Now func() time.Time
	// ReadFile reads a whole transcript file.
	ReadFile func(path string) ([]byte, error)
	// Glob expands a shell-style pattern, used to locate a transcript by its
	// globally unique native id without knowing the project sub-directory.
	Glob func(pattern string) ([]string, error)
	// OpenCodeExport runs the bounded `opencode export --pure --sanitize
	// <sessionID>` edge and returns its JSON. It is the only OpenCode access
	// path; the SQLite store is never read directly.
	OpenCodeExport func(sessionID string) ([]byte, error)
}

// reading is a collector's normalized result. Used is always input-side context
// only (prompt + cache), never output or reasoning tokens. Window and observedAt
// are optional because not every harness records them.
type reading struct {
	used          int
	window        int
	hasWindow     bool
	observedAt    time.Time
	hasObservedAt bool
}

// Sentinel errors let Build translate a collector failure into a stable reason
// without the collector needing to know the reason strings. errTelemetry wraps
// any IO/command failure a collector encounters.
var (
	errTranscriptNotFound    = errors.New(ReasonTranscriptNotFound)
	errAwaitingFirstResponse = errors.New(ReasonAwaitingFirstResponse)
	errAfterCompaction       = errors.New(ReasonAfterCompaction)
	errNativeSession         = errors.New(ReasonNativeSessionUnavailable)
	errUnsupportedFormat     = errors.New(ReasonUnsupportedFormat)
)

// collector reads one harness's transcript for a native session ref.
type collector func(ref Ref, deps Deps) (reading, error)

func collectorFor(harness string) (collector, bool) {
	switch harness {
	case "claude":
		return collectClaude, true
	case "codex":
		return collectCodex, true
	case "pi":
		return collectPi, true
	case "opencode":
		return collectOpenCode, true
	default:
		return nil, false
	}
}

// Build assembles the deterministic report. Agents are sorted by name (ties
// broken by harness) so the output is stable across runs; the orchestrator is
// treated like any other named agent and is never filtered out.
func Build(agents []LiveAgent, deps Deps) Report {
	sorted := append([]LiveAgent(nil), agents...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Harness < sorted[j].Harness
	})

	report := Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   deps.Now(),
		Agents:        make([]AgentContext, 0, len(sorted)),
	}
	for _, agent := range sorted {
		report.Agents = append(report.Agents, buildAgent(agent, deps))
	}
	return report
}

func buildAgent(agent LiveAgent, deps Deps) AgentContext {
	entry := AgentContext{Name: agent.Name, Harness: agent.Harness, Revision: agent.Revision}

	// In-flight gates first, before any session or harness validation, so the
	// working state is stable and never changes because of an unrelated missing
	// integration. A mid-turn figure is suppressed even if an earlier completed
	// response exists.
	if agent.InFlight {
		return unknown(entry, ReasonAgentWorking)
	}
	// Idle: structural incapability next — no native session, or a harness
	// Fledge cannot collect — then collector telemetry.
	if agent.Ref.Value == "" || (agent.Ref.Kind != "id" && agent.Ref.Kind != "path") {
		return unknown(entry, ReasonNativeSessionUnavailable)
	}
	collect, ok := collectorFor(agent.Harness)
	if !ok {
		return unknown(entry, ReasonUnsupportedFormat)
	}

	result, err := collect(agent.Ref, deps)
	if err != nil {
		return unknown(entry, reasonFor(err))
	}

	// Idle with authoritative latest-completed telemetry: available, reason null.
	entry.Status = StatusAvailable
	used := result.used
	entry.Used = &used
	if result.hasWindow && result.window > 0 {
		window := result.window
		entry.Window = &window
		percent := math.Round(float64(used)/float64(window)*10000) / 100
		entry.Percent = &percent
	}
	if result.hasObservedAt {
		observed := result.observedAt
		entry.ObservedAt = &observed
	}
	return entry
}

// unknown marks an agent's figure unobtainable, leaving every numeric field nil.
func unknown(entry AgentContext, reason string) AgentContext {
	entry.Status = StatusUnknown
	entry.Reason = &reason
	return entry
}

// reasonFor maps a collector error to one of the seven stable reasons. Any
// error that is not one of the recognized sentinels is an IO/command failure,
// reported as telemetry_unavailable rather than leaking the underlying message.
func reasonFor(err error) string {
	switch {
	case errors.Is(err, errTranscriptNotFound):
		return ReasonTranscriptNotFound
	case errors.Is(err, errAwaitingFirstResponse):
		return ReasonAwaitingFirstResponse
	case errors.Is(err, errAfterCompaction):
		return ReasonAfterCompaction
	case errors.Is(err, errNativeSession):
		return ReasonNativeSessionUnavailable
	case errors.Is(err, errUnsupportedFormat):
		return ReasonUnsupportedFormat
	default:
		return ReasonTelemetryUnavailable
	}
}
