package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	libusage "github.com/Harrison-Blair/fledge/internal/lib/usage"
)

type call = herdrscript.Call

// piSession is a pi session file with two responses: 1.2k input, 34k output,
// 2.5M cache reads, 10 cache writes, and $0.61 of pi-estimated cost.
const piSession = `{"type":"session","version":3,"id":"pi-1","timestamp":"2026-01-03T10:00:00.000Z","cwd":"/repo"}
{"type":"message","id":"e1","parentId":null,"timestamp":"2026-01-03T10:00:05.000Z","message":{"role":"assistant","provider":"p","model":"model-one","usage":{"input":1000,"output":30000,"cacheRead":2500000,"cacheWrite":10,"reasoning":0,"totalTokens":0,"cost":{"total":0.5}}}}
{"type":"message","id":"e2","parentId":"e1","timestamp":"2026-01-03T10:10:00.000Z","message":{"role":"assistant","provider":"p","model":"model-two","usage":{"input":200,"output":4000,"cacheRead":0,"cacheWrite":0,"reasoning":0,"totalTokens":0,"cost":{"total":0.11}}}}
`

// fixture is a repository whose home holds one pi session file.
type fixture struct {
	cwd     string
	session string
	d       libusage.Discovery
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	t.Setenv("HERDR_SESSION", "")
	home := t.TempDir()
	session := filepath.Join(home, ".pi", "agent", "sessions", "--repo--", "2026-01-03T10-00-00-000Z_pi-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte(piSession), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("file readers must not run commands")
		return nil, nil
	}
	return fixture{cwd: identitytest.Repository(t), session: session, d: libusage.Discovery{Home: home, Run: run}}
}

// piAgent is a live pi agent named name in pane with terminal, reporting
// session as a path ref when it is nonempty.
func piAgent(pane, terminal, name, session string) herdr.AgentDetails {
	p := herdrscript.Pane(pane, "w1", "w1:t2")
	harness, cwd := "pi", "/repo"
	p.AgentStatus, p.Name, p.Agent, p.Cwd = "idle", &name, &harness, &cwd
	a := herdrscript.Info(p).Agent
	a.TerminalID = terminal
	if session != "" {
		source, kind := "herdr:pi", "path"
		a.AgentSession = &herdr.AgentSession{Source: &source, Agent: &harness, Kind: &kind, Value: &session}
	}
	return a
}

func listCall(agents ...herdr.AgentDetails) call {
	if agents == nil {
		agents = []herdr.AgentDetails{}
	}
	return call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": agents}}
}

func getCall(target string, a herdr.AgentDetails) call {
	return call{Method: "agent.get", Params: map[string]any{"target": target}, Result: herdr.AgentResult{Type: "agent_info", Agent: a}}
}

func client(t *testing.T, cwd string, calls ...call) libagent.Client {
	c := herdrscript.Client(t, calls...)
	c.Cwd = cwd
	return c
}

func record(t *testing.T, cwd, id string) identity.Record {
	t.Helper()
	s, err := identity.Existing(context.Background(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	var rec identity.Record
	if err := s.Get(identity.Kind, id, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func rows(t *testing.T, out libagent.Outcome) []Row {
	t.Helper()
	r, ok := out.Result.(Result)
	if out.Error != nil || !ok {
		t.Fatalf("%+v", out)
	}
	return r.Agents
}

// A filter match with a live session ref is measured from that ref, and the
// ref is persisted on the agent's record.
func TestUsageFilterMeasuresAndPersistsLiveRef(t *testing.T) {
	f := newFixture(t)
	a := piAgent("w1:p3", "term_a", "worker", f.session)
	rec := identitytest.Register(t, f.cwd, a)
	out := Run(context.Background(), client(t, f.cwd, listCall(a)), f.d, Options{Selection: selector.Selection{Filter: selector.Filter{Harnesses: []string{"pi"}}}})
	got := rows(t, out)
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	r := got[0]
	if r.Basis != libusage.Measured || r.Turns != 2 || r.Tokens.Input != 1200 || r.Cost == nil || r.Harness != "pi" ||
		r.AgentID == nil || *r.AgentID != rec.ID || r.Pane == nil || *r.Pane != "w1:p3" || r.Name == nil || *r.Name != "worker" || r.ElapsedSeconds == nil {
		t.Fatalf("%+v", r)
	}
	if stored := record(t, f.cwd, rec.ID).NativeSession; stored == nil || stored.Value != f.session || stored.Kind != "path" {
		t.Fatalf("ref not persisted: %+v", stored)
	}
	if len(out.Effects) != 1 || out.Effects[0] != (libagent.Effect{Action: "updated", Kind: "native_session", ID: rec.ID}) {
		t.Fatalf("%+v", out.Effects)
	}
}

// --id of an ended agent reads the ref persisted on its record, and elapsed
// runs from registration to the end.
func TestUsageEndedIDUsesPersistedRef(t *testing.T) {
	f := newFixture(t)
	a := piAgent("w1:p3", "term_a", "worker", f.session)
	rec := identitytest.Register(t, f.cwd, a)
	s, err := identity.Existing(context.Background(), f.cwd)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := identity.ObserveSession(s, rec.ID, *a.AgentSession, time.Now()); err != nil {
		t.Fatal(err)
	}
	registered, _ := time.Parse(time.RFC3339, rec.RegisteredAt)
	ended := registered.Add(90 * time.Second).Format(time.RFC3339)
	if err := s.Update(identity.Kind, rec.ID, &rec, func() error { rec.EndedAt = &ended; return nil }); err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), client(t, f.cwd), f.d, Options{Selection: selector.Selection{IDs: []string{rec.ID}}})
	got := rows(t, out)
	if len(got) != 1 || got[0].Basis != libusage.Measured || got[0].Turns != 2 || got[0].Harness != "pi" || got[0].Name == nil || *got[0].Name != "worker" || got[0].ElapsedSeconds == nil || *got[0].ElapsedSeconds != 90 || *got[0].Pane != "w1:p3" {
		t.Fatalf("%+v", got)
	}
	if len(out.Effects) != 0 {
		t.Fatalf("%+v", out.Effects)
	}
}

// A live --id whose terminal is gone from Herdr falls back to its record too.
func TestUsageGoneIDUsesPersistedRef(t *testing.T) {
	f := newFixture(t)
	a := piAgent("w1:p3", "term_a", "worker", f.session)
	rec := identitytest.Register(t, f.cwd, a)
	s, _ := identity.Existing(context.Background(), f.cwd)
	if _, _, err := identity.ObserveSession(s, rec.ID, *a.AgentSession, time.Now()); err != nil {
		t.Fatal(err)
	}
	c := client(t, f.cwd,
		call{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}},
		listCall(),
		call{Method: "pane.list", Result: map[string]any{"type": "pane_list", "panes": []any{}}})
	got := rows(t, Run(context.Background(), c, f.d, Options{Selection: selector.Selection{IDs: []string{rec.ID}}}))
	if len(got) != 1 || got[0].Basis != libusage.Measured || *got[0].AgentID != rec.ID {
		t.Fatalf("%+v", got)
	}
}

// An ended --id without a recorded ref is an unavailable row naming the
// recorded harness.
func TestUsageEndedIDWithoutRefIsUnavailable(t *testing.T) {
	f := newFixture(t)
	rec := identitytest.Register(t, f.cwd, piAgent("w1:p3", "term_a", "worker", ""))
	s, _ := identity.Existing(context.Background(), f.cwd)
	if err := identity.End(s, rec.ID); err != nil {
		t.Fatal(err)
	}
	got := rows(t, Run(context.Background(), client(t, f.cwd), f.d, Options{Selection: selector.Selection{IDs: []string{rec.ID}}}))
	if len(got) != 1 || got[0].Basis != libusage.Unavailable || got[0].Reason != "no native session ref observed" || got[0].Harness != "pi" || *got[0].Name != "worker" {
		t.Fatalf("%+v", got)
	}
}

// An unknown --id fails the same way the other selector commands do.
func TestUsageUnknownIDFails(t *testing.T) {
	f := newFixture(t)
	identitytest.Register(t, f.cwd, piAgent("w1:p3", "term_a", "worker", ""))
	out := Run(context.Background(), client(t, f.cwd), f.d, Options{Selection: selector.Selection{IDs: []string{"0000beef"}}})
	if out.Error == nil || out.Error.Code != "agent_record_not_found" || out.ExitCode() != 1 {
		t.Fatalf("%+v", out)
	}
}

// A target with no session ref live or recorded is an unavailable row, not an
// error.
func TestUsageWithoutRefIsUnavailable(t *testing.T) {
	f := newFixture(t)
	a := piAgent("w1:p4", "term_b", "stray", "")
	got := rows(t, Run(context.Background(), client(t, f.cwd, getCall("w1:p4", a)), f.d, Options{Selection: selector.Selection{Panes: []string{"w1:p4"}}}))
	if len(got) != 1 || got[0].Basis != libusage.Unavailable || got[0].Reason != "no native session ref observed" || got[0].Harness != "pi" || got[0].AgentID != nil || got[0].ElapsedSeconds != nil {
		t.Fatalf("%+v", got)
	}
}

// A --name target is attributed to its live record, so its ref is persisted.
func TestUsageNameAttributesRecord(t *testing.T) {
	f := newFixture(t)
	a := piAgent("w1:p3", "term_a", "worker", f.session)
	rec := identitytest.Register(t, f.cwd, a)
	got := rows(t, Run(context.Background(), client(t, f.cwd, getCall("worker", a)), f.d, Options{Selection: selector.Selection{Names: []string{"worker"}}}))
	if len(got) != 1 || got[0].AgentID == nil || *got[0].AgentID != rec.ID || record(t, f.cwd, rec.ID).NativeSession == nil {
		t.Fatalf("%+v", got)
	}
}

// A failed persist is a warning effect; the row is still reported.
func TestUsagePersistFailureWarns(t *testing.T) {
	f := newFixture(t)
	a := piAgent("w1:p3", "term_a", "worker", f.session)
	rec := identitytest.Register(t, f.cwd, a)
	unchanged := identitytest.ReadOnly(t, f.cwd, rec.ID)
	out := Run(context.Background(), client(t, f.cwd, listCall(a)), f.d, Options{Selection: selector.Selection{Filter: selector.Filter{Registered: true}}})
	if got := rows(t, out); len(got) != 1 || got[0].Basis != libusage.Measured || got[0].AgentID == nil || *got[0].AgentID != rec.ID || out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	if len(out.Effects) != 1 || out.Effects[0] != (libagent.Effect{Action: "warning", Kind: "native_session", ID: rec.ID}) {
		t.Fatalf("%+v", out.Effects)
	}
	unchanged()
}

// An empty filter selection is no_agents_matched, as for stop and message.
func TestUsageEmptySelectionFails(t *testing.T) {
	f := newFixture(t)
	out := Run(context.Background(), client(t, f.cwd, listCall(piAgent("w1:p3", "term_a", "worker", ""))), f.d, Options{Selection: selector.Selection{Filter: selector.Filter{States: []string{"working"}}}})
	if out.Status != "rejected" || out.ExitCode() != 1 || out.Error.Code != "no_agents_matched" || out.Error.Phase != "selection" {
		t.Fatalf("%+v", out)
	}
}

func TestUsageRejectsInvalidSelection(t *testing.T) {
	f := newFixture(t)
	for _, sel := range []selector.Selection{
		{},
		{Names: []string{"a"}, Filter: selector.Filter{States: []string{"idle"}}},
		{IDs: []string{"nope"}},
		{Filter: selector.Filter{States: []string{"asleep"}}},
	} {
		out := Run(context.Background(), client(t, f.cwd), f.d, Options{Selection: sel})
		if out.ExitCode() != 2 || out.Error.Phase != "validation" {
			t.Fatalf("%+v: %+v", sel, out)
		}
	}
}

func TestRenderTable(t *testing.T) {
	id, name, pane := "0000beef", "worker", "w1:p3"
	elapsed := int64(3725)
	result := Result{Agents: []Row{
		{AgentID: &id, Name: &name, Pane: &pane, ElapsedSeconds: &elapsed, Summary: libusage.Summary{Harness: "pi", Models: []string{"model-one", "model-two"}, Turns: 2,
			Tokens: libusage.Tokens{Input: 1200, Output: 34000, CacheRead: 2500000, CacheWrite: 10}, Cost: &libusage.Cost{Amount: 0.61, Currency: "USD", Basis: "estimate"}, Basis: libusage.Measured}},
		{Pane: &pane, Summary: libusage.Summary{Harness: "cursor", Basis: libusage.Unavailable, Reason: "no native session ref observed"}},
	}}
	var b bytes.Buffer
	if err := Render(&b, libagent.Outcome{Status: "success", Result: result}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	want := [][]string{
		{"NAME", "HARNESS", "MODELS", "TURNS", "INPUT", "OUTPUT", "CACHE-R", "CACHE-W", "COST", "ELAPSED", "BASIS"},
		{"worker", "pi", "model-one,model-two", "2", "1.2k", "34k", "2.5M", "10", "$0.61", "(est)", "1h2m5s", "measured"},
		{"-", "cursor", "-", "-", "-", "-", "-", "-", "-", "-", "unavailable", "(no", "native", "session", "ref", "observed)"},
	}
	if len(lines) != len(want) {
		t.Fatalf("%q", b.String())
	}
	for i, fields := range want {
		if got := strings.Fields(lines[i]); strings.Join(got, "|") != strings.Join(fields, "|") {
			t.Fatalf("line %d: %q want %q", i, got, fields)
		}
	}
}

func TestRenderWarning(t *testing.T) {
	var b bytes.Buffer
	out := libagent.Outcome{Status: "success", Result: Result{Agents: []Row{}}, Effects: []libagent.Effect{{Action: "warning", Kind: "native_session", ID: "0000beef"}}}
	if err := Render(&b, out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "warning: could not record the native session ref of agent 0000beef") {
		t.Fatalf("%q", b.String())
	}
}

func TestCount(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1k", 1250: "1.2k", 34000: "34k", 999999: "999.9k", 1000000: "1M", 2500000: "2.5M", 12345678: "12.3M"} {
		if got := count(n); got != want {
			t.Errorf("count(%d) = %q, want %q", n, got, want)
		}
	}
}

// --json carries the identity fields and every Summary field, subagents
// included.
func TestJSONFields(t *testing.T) {
	f := newFixture(t)
	a := piAgent("w1:p3", "term_a", "worker", f.session)
	identitytest.Register(t, f.cwd, a)
	out := Run(context.Background(), client(t, f.cwd, listCall(a)), f.d, Options{Selection: selector.Selection{Filter: selector.Filter{Registered: true}}})
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Result struct {
			Agents []map[string]json.RawMessage `json:"agents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b.Bytes(), &decoded); err != nil || len(decoded.Result.Agents) != 1 {
		t.Fatalf("%v %s", err, b.String())
	}
	for _, key := range []string{"agent_id", "name", "pane", "elapsed_seconds", "harness", "session_kind", "session_value", "models", "turns", "tokens", "cost", "first", "last", "subagents", "basis", "reason", "sources"} {
		if _, ok := decoded.Result.Agents[0][key]; !ok {
			t.Errorf("missing %s in %s", key, b.String())
		}
	}
}
