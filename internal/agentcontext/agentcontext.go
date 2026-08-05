// Package agentcontext derives each live agent's context-window usage from the
// harness's own on-disk session transcript and renders a deterministic,
// versioned report.
//
// The package core is pure: Build takes an explicit list of live agents and a
// Deps bundle that injects every side effect (clock, file reads, globbing, and
// the bounded OpenCode export edge). Nothing here reads the environment, the
// wall clock, or the filesystem on its own, so the whole pipeline is driven by
// fixtures in tests.
//
// The report never stores anything sensitive: no transcript content, no native
// session identifiers, and no filesystem paths. It carries only the agent name,
// its harness, the Herdr revision that proves the correlation, and the
// normalized token counts.
package agentcontext

import "time"

// SchemaVersion is the version stamped into every persisted and printed report.
// Bump it whenever the JSON shape changes in a way consumers must notice.
const SchemaVersion = 1

// The seven stable reasons are the complete, closed vocabulary of the report's
// reason field. Exactly one is set on every agent; callers and tests match on
// these exact strings, so never reword one without treating it as a schema
// change, and never emit a value outside this set.
//
//   - ReasonAgentWorking: the agent is actively working, so any earlier
//     completed figure is suppressed as stale until the turn settles.
//   - ReasonAwaitingFirstResponse: the session exists but no completed model
//     response has produced usage yet.
//   - ReasonAfterCompaction: a compaction record is the latest event after the
//     last usage record, so the pre-compaction figure is stale and no
//     post-compaction response has landed yet. Distinct from awaiting-first: a
//     prior response existed, it was superseded by the compaction.
//   - ReasonNativeSessionUnavailable: Herdr reported no usable agent_session.
//   - ReasonTranscriptNotFound: the exact session transcript is absent.
//   - ReasonTelemetryUnavailable: an IO, command, or tool call for the telemetry
//     failed.
//   - ReasonUnsupportedFormat: the harness is not one Fledge collects, or its
//     transcript schema was not recognized.
const (
	ReasonAgentWorking             = "agent_working"
	ReasonAwaitingFirstResponse    = "awaiting_first_response"
	ReasonAfterCompaction          = "after_compaction"
	ReasonNativeSessionUnavailable = "native_session_unavailable"
	ReasonTranscriptNotFound       = "transcript_not_found"
	ReasonTelemetryUnavailable     = "telemetry_unavailable"
	ReasonUnsupportedFormat        = "unsupported_format"
)

// PublicReasons is the closed set of reason strings the report may contain.
var PublicReasons = []string{
	ReasonAgentWorking,
	ReasonAwaitingFirstResponse,
	ReasonAfterCompaction,
	ReasonNativeSessionUnavailable,
	ReasonTranscriptNotFound,
	ReasonTelemetryUnavailable,
	ReasonUnsupportedFormat,
}

// Ref is the native session reference Herdr reports for an agent. Kind is
// "id" or "path"; Value is the harness's own session identifier. Correlation is
// exact: a collector locates a transcript only by this Value, never by cwd,
// mtime, or pane heuristics.
type Ref struct {
	Kind  string
	Value string
}

// LiveAgent is the minimal projection of one Herdr agent the report needs. It
// deliberately excludes Herdr's richer pane/tab data so the core stays
// decoupled from the herdr package and trivially constructible in tests.
// InFlight is true when Herdr reports the agent as actively working, which
// suppresses any figure as mid-turn.
type LiveAgent struct {
	Name     string
	Harness  string
	Ref      Ref
	Revision int
	InFlight bool
}

// Status is the two-valued health of an agent's usage figure.
const (
	// StatusAvailable means the agent is idle and its numeric fields carry
	// authoritative latest-completed telemetry; Reason is null.
	StatusAvailable = "available"
	// StatusUnknown means no authoritative figure is being reported — the agent
	// is in-flight, or telemetry is awaiting/compacted/unavailable. The numeric
	// fields are null and Reason explains why.
	StatusUnknown = "unknown"
)

// AgentContext is one agent's line in the report.
//
// When Status is available the numeric fields (Window, Used, Percent) and
// ObservedAt carry authoritative telemetry and Reason is null. When Status is
// unknown every numeric field and ObservedAt is null and Reason holds exactly
// one of the seven stable strings above. A live/in-flight agent is always
// unknown with reason agent_working, even if an earlier completed response
// exists, so a mid-turn figure is never reported as authoritative.
type AgentContext struct {
	Name       string     `json:"name"`
	Harness    string     `json:"harness"`
	Revision   int        `json:"revision"`
	Status     string     `json:"status"`
	Window     *int       `json:"context_window"`
	Used       *int       `json:"used_tokens"`
	Percent    *float64   `json:"used_percent"`
	ObservedAt *time.Time `json:"observed_at"`
	Reason     *string    `json:"reason"`
}

// Report is the versioned, deterministic top-level document. Agents is sorted
// by name so identical inputs always render byte-for-byte identically.
type Report struct {
	SchemaVersion int            `json:"schema_version"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Agents        []AgentContext `json:"agents"`
}
