package agentcontext

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }

func TestBuildSortsByNameAndIncludesOrchestrator(t *testing.T) {
	t.Parallel()
	fx := fixture{
		files: map[string]string{
			claudePath("orch"): claudeTranscript,
			codexPath("work"):  codexTranscript,
		},
		now: fixedNow(),
	}
	agents := []LiveAgent{
		{Name: "worker", Harness: "codex", Ref: Ref{Kind: "id", Value: "work"}, Revision: 7},
		{Name: "orchestrator", Harness: "claude", Ref: Ref{Kind: "id", Value: "orch"}, Revision: 3},
	}
	report := Build(agents, fx.deps())

	if report.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", report.SchemaVersion, SchemaVersion)
	}
	if !report.GeneratedAt.Equal(fixedNow()) {
		t.Errorf("GeneratedAt = %v, want injected clock value", report.GeneratedAt)
	}
	if len(report.Agents) != 2 {
		t.Fatalf("len(Agents) = %d, want 2", len(report.Agents))
	}
	if report.Agents[0].Name != "orchestrator" || report.Agents[1].Name != "worker" {
		t.Errorf("order = [%q %q], want [orchestrator worker]", report.Agents[0].Name, report.Agents[1].Name)
	}
	orch := report.Agents[0]
	if orch.Revision != 3 {
		t.Errorf("orchestrator revision = %d, want 3 (correlation metadata)", orch.Revision)
	}
	// Idle agent with authoritative telemetry: available, reason null.
	if orch.Status != StatusAvailable || orch.Reason != nil {
		t.Errorf("orchestrator status/reason = %q/%v, want available/nil", orch.Status, orch.Reason)
	}
	if orch.Used == nil || *orch.Used != 21002 || orch.Percent == nil {
		t.Errorf("orchestrator figure = %+v, want populated", orch)
	}
}

func TestBuildInFlightSuppressesFigure(t *testing.T) {
	t.Parallel()
	// The transcript would yield a reading, but the agent is in-flight.
	fx := fixture{files: map[string]string{claudePath("busy"): claudeTranscript}, now: fixedNow()}
	report := Build([]LiveAgent{{Name: "a", Harness: "claude", Ref: Ref{Kind: "id", Value: "busy"}, InFlight: true}}, fx.deps())
	got := report.Agents[0]
	if got.Status != StatusUnknown {
		t.Errorf("status = %q, want unknown", got.Status)
	}
	if got.Reason == nil || *got.Reason != ReasonAgentWorking {
		t.Errorf("reason = %v, want agent_working", got.Reason)
	}
	if got.Used != nil || got.Window != nil || got.Percent != nil || got.ObservedAt != nil {
		t.Errorf("in-flight agent must null every figure field, got %+v", got)
	}
}

func TestBuildInFlightGatesBeforeSessionAndHarnessValidation(t *testing.T) {
	t.Parallel()
	fx := fixture{now: fixedNow()}
	cases := []LiveAgent{
		{Name: "no-ref", Harness: "claude", Ref: Ref{}, InFlight: true},                            // empty ref
		{Name: "bad-harness", Harness: "gemini", Ref: Ref{Kind: "id", Value: "x"}, InFlight: true}, // unsupported harness
	}
	for _, agent := range cases {
		t.Run(agent.Name, func(t *testing.T) {
			got := Build([]LiveAgent{agent}, fx.deps()).Agents[0]
			// In-flight wins regardless of the unrelated missing session/harness.
			if got.Reason == nil || *got.Reason != ReasonAgentWorking {
				t.Errorf("reason = %v, want agent_working (in-flight gates first)", got.Reason)
			}
		})
	}
}

func TestBuildIdleReasons(t *testing.T) {
	t.Parallel()
	fx := fixture{
		files:   map[string]string{claudePath("empty"): `{"type":"user","message":{"role":"user"}}` + "\n"},
		exports: map[string]string{}, // export of an unknown id fails -> telemetry_unavailable
		now:     fixedNow(),
	}
	cases := []struct {
		name   string
		agent  LiveAgent
		reason string
	}{
		{"native session unavailable", LiveAgent{Name: "x", Harness: "claude", Ref: Ref{}}, ReasonNativeSessionUnavailable},
		{"invalid native session kind", LiveAgent{Name: "x", Harness: "claude", Ref: Ref{Kind: "other", Value: "z"}}, ReasonNativeSessionUnavailable},
		{"unsupported harness", LiveAgent{Name: "x", Harness: "gemini", Ref: Ref{Kind: "id", Value: "z"}}, ReasonUnsupportedFormat},
		{"transcript not found", LiveAgent{Name: "x", Harness: "claude", Ref: Ref{Kind: "id", Value: "missing"}}, ReasonTranscriptNotFound},
		{"awaiting first response", LiveAgent{Name: "x", Harness: "claude", Ref: Ref{Kind: "id", Value: "empty"}}, ReasonAwaitingFirstResponse},
		{"telemetry unavailable", LiveAgent{Name: "x", Harness: "opencode", Ref: Ref{Kind: "id", Value: "gone"}}, ReasonTelemetryUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Build([]LiveAgent{tc.agent}, fx.deps()).Agents[0]
			if got.Status != StatusUnknown {
				t.Errorf("status = %q, want unknown", got.Status)
			}
			if got.Reason == nil || *got.Reason != tc.reason {
				t.Errorf("reason = %v, want %q", got.Reason, tc.reason)
			}
			if got.Used != nil || got.Window != nil || got.Percent != nil || got.ObservedAt != nil {
				t.Errorf("unknown agent must have nil figure fields, got %+v", got)
			}
		})
	}
}

// TestBuildEmitsOnlyPublicReasons guards the closed reason vocabulary: every
// reason the builder can produce must be one of the seven allowed strings, and
// an available agent's reason must be null.
func TestBuildEmitsOnlyPublicReasons(t *testing.T) {
	t.Parallel()
	allowed := make(map[string]bool, len(PublicReasons))
	for _, reason := range PublicReasons {
		allowed[reason] = true
	}
	if len(allowed) != 7 {
		t.Fatalf("PublicReasons has %d entries, want exactly 7", len(allowed))
	}

	fx := fixture{
		files: map[string]string{
			claudePath("ok"):    claudeTranscript,
			claudePath("empty"): `{"x":1}` + "\n",
			claudePath("comp"):  claudeCompactedTranscript,
		},
		now: fixedNow(),
	}
	agents := []LiveAgent{
		{Name: "available", Harness: "claude", Ref: Ref{Kind: "id", Value: "ok"}},
		{Name: "working", Harness: "claude", Ref: Ref{Kind: "id", Value: "ok"}, InFlight: true},
		{Name: "awaiting", Harness: "claude", Ref: Ref{Kind: "id", Value: "empty"}},
		{Name: "compacted", Harness: "claude", Ref: Ref{Kind: "id", Value: "comp"}},
		{Name: "no-session", Harness: "claude", Ref: Ref{}},
		{Name: "not-found", Harness: "claude", Ref: Ref{Kind: "id", Value: "missing"}},
		{Name: "bad-format", Harness: "gemini", Ref: Ref{Kind: "id", Value: "z"}},
		{Name: "telemetry", Harness: "opencode", Ref: Ref{Kind: "id", Value: "gone"}},
	}
	for _, agent := range Build(agents, fx.deps()).Agents {
		switch agent.Status {
		case StatusAvailable:
			if agent.Reason != nil {
				t.Errorf("%s: available agent must have null reason, got %v", agent.Name, *agent.Reason)
			}
		case StatusUnknown:
			if agent.Reason == nil || !allowed[*agent.Reason] {
				t.Errorf("%s: reason %v is not in the allowed public set", agent.Name, agent.Reason)
			}
		default:
			t.Errorf("%s: status %q is neither available nor unknown", agent.Name, agent.Status)
		}
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	t.Parallel()
	fx := fixture{files: map[string]string{piPath("p"): piTranscript}, now: fixedNow()}
	agents := []LiveAgent{{Name: "a", Harness: "pi", Ref: Ref{Kind: "id", Value: "p"}}}

	first, err := json.Marshal(Build(agents, fx.deps()))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(Build(agents, fx.deps()))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("Build is not deterministic:\n%s\n%s", first, second)
	}
}

func TestBuildNullableFieldsSerializeAsJSONNull(t *testing.T) {
	t.Parallel()
	// An available Pi agent has a known used total but an unknown window, so
	// window and percent stay null while reason (available) is also null.
	fx := fixture{files: map[string]string{piPath("p"): piTranscript}, now: fixedNow()}
	report := Build([]LiveAgent{{Name: "a", Harness: "pi", Ref: Ref{Kind: "id", Value: "p"}}}, fx.deps())
	encoded, err := json.Marshal(report.Agents[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"status":"available"`, `"context_window":null`, `"used_percent":null`, `"reason":null`, `"used_tokens":1374`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("encoded agent = %s, want to contain %q", encoded, want)
		}
	}
}

func TestBuildComputesPercent(t *testing.T) {
	t.Parallel()
	fx := fixture{files: map[string]string{codexPath("c"): codexTranscript}, now: fixedNow()}
	report := Build([]LiveAgent{{Name: "a", Harness: "codex", Ref: Ref{Kind: "id", Value: "c"}}}, fx.deps())
	got := report.Agents[0]
	if got.Percent == nil {
		t.Fatal("percent is nil, want computed value")
	}
	// 18194 / 258400 * 100 = 7.04 (rounded to 2 dp).
	if *got.Percent != 7.04 {
		t.Errorf("percent = %v, want 7.04", *got.Percent)
	}
}
