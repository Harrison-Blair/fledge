package adopt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

type call = herdrscript.Call

func client(t *testing.T, calls ...call) libagent.Client {
	t.Helper()
	c := herdrscript.Client(t, calls...)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, b)
	}
	c.Cwd = root
	return c
}

// agent is the live agent in pane with the given name (nil for unnamed).
func agent(pane string, name *string) herdr.AgentResult {
	p := herdrscript.LiveAgent("idle")
	p.PaneID, p.Name = pane, name
	r := herdrscript.Info(p)
	r.Agent.TerminalID = "term_a"
	return r
}
func named(s string) *string { return &s }
func notFound() error        { return &herdr.Error{Code: "agent_not_found", Message: "no agent"} }

func TestAdoptSelfRenamesUnnamedAgent(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t,
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: agent("old:p1", nil)},
		call{Method: "agent.rename", Params: map[string]any{"target": "old:p1", "name": "helper"}, Result: agent("old:p1", named("helper"))},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: agent("old:p1", named("helper"))},
	)
	out := Run(context.Background(), c, Options{Name: "helper"})
	if out.Status != "success" || out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	r := out.Result.(Result)
	if !r.Renamed || *r.Name != "helper" || r.Pane != "old:p1" || r.TerminalID != "term_a" || r.RegisteredBy != "adopt" || r.Parent != nil || r.WorktreePath != nil || r.Profile != nil {
		t.Fatalf("%+v", r)
	}
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	var stored identity.Record
	if err := s.Get(identity.Kind, r.ID, &stored); err != nil || !reflect.DeepEqual(stored, r.Record) {
		t.Fatalf("%+v %v", stored, err)
	}
	last := out.Effects[len(out.Effects)-2:]
	if !reflect.DeepEqual(last, []libagent.Effect{{Action: "updated", Kind: "agent_name", ID: "old:p1"}, {Action: "created", Kind: "agent_record", ID: r.ID}}) {
		t.Fatalf("%+v", out.Effects)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "Adopted helper (old:p1) as "+r.ID+".\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestAdoptNamedAgentWithMatchingNameDoesNotRename(t *testing.T) {
	for _, name := range []string{"worker", ""} {
		c := client(t,
			call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: agent("w1:p3", named("worker"))},
			call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()},
		)
		out := Run(context.Background(), c, Options{Pane: "w1:p3", Name: name})
		if out.Error != nil || out.Result.(Result).Renamed || *out.Result.(Result).Name != "worker" {
			t.Fatalf("%q: %+v %+v", name, out.Error, out.Result)
		}
	}
}

func TestAdoptRejections(t *testing.T) {
	for label, tc := range map[string]struct {
		o     Options
		calls []call
		code  string
	}{
		"different name":  {Options{Pane: "w1:p3", Name: "other"}, []call{{Method: "agent.get", Result: agent("w1:p3", named("worker"))}}, "invalid_input"},
		"unnamed no name": {Options{Pane: "w1:p3"}, []call{{Method: "agent.get", Result: agent("w1:p3", nil)}}, "invalid_input"},
		"bad name":        {Options{Pane: "w1:p3", Name: "Bad Name"}, nil, "invalid_input"},
		"no agent":        {Options{Pane: "w1:p3", Name: "x"}, []call{{Method: "agent.get", Err: notFound()}}, "agent_not_found"},
		"name taken": {Options{Pane: "w1:p3", Name: "taken"}, []call{{Method: "agent.get", Result: agent("w1:p3", nil)},
			{Method: "agent.rename", Params: map[string]any{"target": "w1:p3", "name": "taken"}, Err: &herdr.Error{Code: "agent_name_taken", Message: "taken"}}}, "agent_name_taken"},
		"launch pending": {Options{Pane: "w1:p3", Name: "x"}, []call{{Method: "agent.get", Result: agent("w1:p3", nil)},
			{Method: "agent.rename", Err: &herdr.Error{Code: "agent_launch_pending", Message: "pending"}}}, "agent_launch_pending"},
	} {
		t.Run(label, func(t *testing.T) {
			out := Run(context.Background(), client(t, tc.calls...), tc.o)
			if out.Error == nil || out.Error.Code != tc.code {
				t.Fatalf("%+v", out.Error)
			}
			if ids := records(t, out); len(ids) != 0 {
				t.Fatalf("records created: %v", ids)
			}
		})
	}
}

// Rejections decided from Herdr alone must not create .fledge or the store.
func TestAdoptRejectionBeforeRegistrationLeavesNoFiles(t *testing.T) {
	for label, tc := range map[string]struct {
		o    Options
		get  call
		code string
	}{
		"missing pane":    {Options{Pane: "w9:p9", Name: "x"}, call{Method: "agent.get", Params: map[string]any{"target": "w9:p9"}, Err: notFound()}, "agent_not_found"},
		"transport":       {Options{Pane: "w1:p3", Name: "x"}, call{Method: "agent.get", Err: &herdr.Error{Code: "connection_error", Message: "down"}}, "connection_error"},
		"different name":  {Options{Pane: "w1:p3", Name: "other"}, call{Method: "agent.get", Result: agent("w1:p3", named("worker"))}, "invalid_input"},
		"unnamed no name": {Options{Pane: "w1:p3"}, call{Method: "agent.get", Result: agent("w1:p3", nil)}, "invalid_input"},
	} {
		t.Run(label, func(t *testing.T) {
			c := client(t, tc.get)
			out := Run(context.Background(), c, tc.o)
			if out.Status != "rejected" || out.Error == nil || out.Error.Code != tc.code || len(out.Effects) != 0 {
				t.Fatalf("%+v %+v", out, out.Error)
			}
			if _, err := os.Stat(filepath.Join(c.Cwd, ".fledge")); !os.IsNotExist(err) {
				t.Fatalf("created .fledge: %v", err)
			}
		})
	}
}

func records(t *testing.T, out libagent.Outcome) []string {
	for _, e := range out.Effects {
		if e.Kind == "agent_record" {
			return []string{e.ID}
		}
	}
	return nil
}

func TestAdoptRefusesRegisteredTerminal(t *testing.T) {
	c := client(t,
		call{Method: "agent.get", Result: agent("w1:p3", named("worker"))},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()},
		call{Method: "agent.get", Result: agent("w1:p3", named("worker"))},
		// A named agent is not renamed, so Register, after its caller lookup,
		// makes the refusal.
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()},
	)
	first := Run(context.Background(), c, Options{Pane: "w1:p3"})
	if first.Error != nil {
		t.Fatalf("%+v", first.Error)
	}
	id := first.Result.(Result).ID
	out := Run(context.Background(), c, Options{Pane: "w1:p3"})
	if out.Error == nil || out.Error.Code != "agent_already_registered" || !strings.Contains(out.Error.Message, id) {
		t.Fatalf("%+v", out.Error)
	}
}

func TestAdoptOutsidePaneRequiresPane(t *testing.T) {
	c := client(t)
	c.CallerPane = ""
	if out := Run(context.Background(), c, Options{Name: "x"}); out.ExitCode() != 2 {
		t.Fatalf("%+v", out)
	}
}

func TestAdoptOutsideRepositoryFailsBeforeRename(t *testing.T) {
	c := herdrscript.Client(t, call{Method: "agent.get", Result: agent("w1:p3", nil)})
	out := Run(context.Background(), c, Options{Pane: "w1:p3", Name: "x"})
	if out.Error == nil || out.Error.Phase != "state" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
}

// barrierHerdr serves agent.get concurrently: the target is a named live
// agent, and the caller lookup, the last Herdr call before registration,
// waits until every adopter has reached it (or fails after a bound, so an
// adopter that stops early cannot hang the test).
type barrierHerdr struct{ arrived sync.WaitGroup }

func (b *barrierHerdr) Call(_ context.Context, method string, params any, result any) error {
	target := params.(map[string]any)["target"]
	if method != "agent.get" {
		return fmt.Errorf("unexpected %s", method)
	}
	if target == "old:p1" {
		b.arrived.Done()
		all := make(chan struct{})
		go func() { b.arrived.Wait(); close(all) }()
		select {
		case <-all:
		case <-time.After(5 * time.Second):
			return fmt.Errorf("not every adopter reached registration")
		}
		return notFound()
	}
	data, _ := json.Marshal(agent("w1:p3", named("worker")))
	return json.Unmarshal(data, result)
}

func TestConcurrentAdoptsRegisterOnce(t *testing.T) {
	const n = 2
	fake := &barrierHerdr{}
	fake.arrived.Add(n)
	c := client(t)
	c.API = fake
	// Create .fledge up front: concurrent first-time creation is not under test.
	if _, err := identity.OpenStore(context.Background(), c.Cwd, &libagent.Outcome{}); err != nil {
		t.Fatal(err)
	}
	outs := make([]libagent.Outcome, n)
	var wg sync.WaitGroup
	for i := range outs {
		wg.Go(func() { outs[i] = Run(context.Background(), c, Options{Pane: "w1:p3"}) })
	}
	wg.Wait()
	var winner string
	refused := 0
	for _, out := range outs {
		switch {
		case out.Error == nil:
			if winner != "" {
				t.Fatalf("two adopts succeeded: %s and %s", winner, out.Result.(Result).ID)
			}
			winner = out.Result.(Result).ID
		case out.Error.Code == "agent_already_registered":
			refused++
		default:
			t.Fatalf("%+v", out.Error)
		}
	}
	if winner == "" || refused != n-1 {
		t.Fatalf("winner %q, %d refused", winner, refused)
	}
	for _, out := range outs {
		if out.Error != nil && !strings.Contains(out.Error.Message, winner) {
			t.Fatalf("refusal does not name the winner %s: %s", winner, out.Error.Message)
		}
	}
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	if ids, err := s.List(identity.Kind); err != nil || len(ids) != 1 || ids[0] != winner {
		t.Fatalf("records %v, %v", ids, err)
	}
	if rec, err := identity.Live(s, "term_a"); err != nil || rec == nil || rec.ID != winner {
		t.Fatalf("%+v %v", rec, err)
	}
}

// A different harness started in an adopted terminal can be adopted afresh.
func TestAdoptAfterHarnessChange(t *testing.T) {
	codex := agent("w1:p3", named("worker"))
	codex.Agent.Agent = named("codex")
	c := client(t,
		call{Method: "agent.get", Result: codex},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()},
		call{Method: "agent.get", Result: agent("w1:p3", named("worker"))},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()},
	)
	first := Run(context.Background(), c, Options{Pane: "w1:p3"})
	if first.Error != nil {
		t.Fatalf("%+v", first.Error)
	}
	out := Run(context.Background(), c, Options{Pane: "w1:p3"})
	if out.Error != nil || out.Result.(Result).ID == first.Result.(Result).ID || *out.Result.(Result).Harness != "claude" {
		t.Fatalf("%+v", out)
	}
}

// countPrechecks counts adopt's scans of the agent records outside Register,
// which itself scans once.
func countPrechecks(t *testing.T) *int {
	t.Helper()
	n, real := 0, registered
	registered = func(s *state.Store, a herdr.AgentDetails) (*identity.Record, error) { n++; return real(s, a) }
	t.Cleanup(func() { registered = real })
	return &n
}

// Adopting a named agent scans the agent records once, in Register. Renaming
// an unnamed agent is a Herdr call that cannot run under the store lock, so
// adopt scans once more beforehand to find a record of the terminal to name
// instead of registering anew: two scans, a deliberate exception to one scan
// per adopt.
func TestAdoptScansOnceUnlessItMustRenameFirst(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	n := countPrechecks(t)
	c := client(t,
		call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: agent("w1:p3", named("worker"))},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()},
	)
	if out := Run(context.Background(), c, Options{Pane: "w1:p3"}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if *n != 0 {
		t.Fatalf("adopting a named agent scanned %d extra times, want 0", *n)
	}
	*n = 0
	c = client(t,
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: agent("old:p1", nil)},
		call{Method: "agent.rename", Params: map[string]any{"target": "old:p1", "name": "helper"}, Result: agent("old:p1", named("helper"))},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: agent("old:p1", named("helper"))},
	)
	if out := Run(context.Background(), c, Options{Name: "helper"}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if *n != 1 {
		t.Fatalf("adopting with a rename scanned %d extra times, want 1", *n)
	}
}

// seed stores a live record of the adopt fixture's terminal as registered
// from pane w1:p9 in workspace w9 with the given name, a parent, and a
// worktree, and returns it.
func seed(t *testing.T, c libagent.Client, name *string) identity.Record {
	t.Helper()
	s, err := identity.OpenStore(context.Background(), c.Cwd, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	d := agent("w1:p9", name).Agent
	d.WorkspaceID = "w9"
	tree := "/repo/.fledge/worktrees/w"
	rec, err := identity.Register(context.Background(), s, libagent.Client{}, d, "spawn", &identity.Checkout{Path: tree}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(identity.Kind, rec.ID, &rec, func() error { rec.Parent = named("0000beef"); return nil }); err != nil {
		t.Fatal(err)
	}
	return rec
}

func stored(t *testing.T, c libagent.Client, id string) identity.Record {
	t.Helper()
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	if ids, err := s.List(identity.Kind); err != nil || len(ids) != 1 {
		t.Fatalf("records %v %v", ids, err)
	}
	var rec identity.Record
	if err := s.Get(identity.Kind, id, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// A registered terminal whose live agent is unnamed is named through Herdr and
// keeps its record: the same id, parent, registration, and worktree, with the
// new name and the pane where it now runs. Whether the record kept a former
// name does not matter; Herdr's live name decides.
func TestAdoptNamesRegisteredUnnamedAgentKeepingRecord(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	for label, former := range map[string]*string{"lost name": named("worker"), "never named": nil} {
		t.Run(label, func(t *testing.T) {
			c := client(t,
				call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: agent("w1:p3", nil)},
				call{Method: "agent.rename", Params: map[string]any{"target": "w1:p3", "name": "helper"}, Result: agent("w1:p3", named("helper"))},
			)
			existing := seed(t, c, former)
			out := Run(context.Background(), c, Options{Pane: "w1:p3", Name: "helper"})
			if out.Status != "success" || out.Error != nil {
				t.Fatalf("%+v", out.Error)
			}
			want := existing
			want.Name, want.Pane, want.WorkspaceID = named("helper"), "w1:p3", "w1"
			r := out.Result.(Result)
			if !r.Renamed || !reflect.DeepEqual(r.Record, want) {
				t.Fatalf("%+v", r)
			}
			if got := stored(t, c, existing.ID); !reflect.DeepEqual(got, want) {
				t.Fatalf("%+v", got)
			}
			if !reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "updated", Kind: "agent_name", ID: "w1:p3"}, {Action: "updated", Kind: "agent_record", ID: existing.ID}}) {
				t.Fatalf("%+v", out.Effects)
			}
			var b bytes.Buffer
			if err := out.Write(&b, false, Render); err != nil || b.String() != "Adopted helper (w1:p3) as "+existing.ID+".\n" {
				t.Fatalf("%q %v", b.String(), err)
			}
		})
	}
}

// When naming a registered agent fails, or its outcome is uncertain, the record
// is left as it was.
func TestAdoptNamingRegisteredAgentFailures(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	other := agent("w1:p3", named("helper"))
	other.Agent.TerminalID = "term_b"
	for label, tc := range map[string]struct {
		rename call
		status string
		code   string
	}{
		"name taken":       {call{Method: "agent.rename", Err: &herdr.Error{Code: "agent_name_taken", Message: "taken"}}, "rejected", "agent_name_taken"},
		"launch pending":   {call{Method: "agent.rename", Err: &herdr.Error{Code: "agent_launch_pending", Message: "pending"}}, "rejected", "agent_launch_pending"},
		"mismatched reply": {call{Method: "agent.rename", Result: other}, "unknown", "protocol_error"},
		"uncertain":        {call{Method: "agent.rename", Err: &herdr.Error{Code: "timeout", Message: "slow", Uncertain: true}}, "unknown", "timeout"},
	} {
		t.Run(label, func(t *testing.T) {
			c := client(t, call{Method: "agent.get", Result: agent("w1:p3", nil)}, tc.rename)
			existing := seed(t, c, named("worker"))
			out := Run(context.Background(), c, Options{Pane: "w1:p3", Name: "helper"})
			if out.Status != tc.status || out.Error == nil || out.Error.Code != tc.code || out.Error.Phase != "agent.rename" || len(out.Effects) != 0 {
				t.Fatalf("%+v %+v", out, out.Error)
			}
			if got := stored(t, c, existing.ID); !reflect.DeepEqual(got, existing) {
				t.Fatalf("%+v", got)
			}
		})
	}
}

// A record that ends after adopt looked it up but before it stores the new
// name leaves a partial outcome: Herdr renamed the agent, the record did not
// change.
func TestAdoptRenamedButRecordEndedIsPartial(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	var existing identity.Record
	var c libagent.Client
	c = client(t,
		call{Method: "agent.get", Result: agent("w1:p3", nil)},
		call{Method: "agent.rename", Result: agent("w1:p3", named("helper")), Before: func() {
			s, err := identity.Existing(context.Background(), c.Cwd)
			if err == nil {
				err = identity.End(s, existing.ID)
			}
			if err != nil {
				t.Error(err)
			}
		}},
	)
	existing = seed(t, c, named("worker"))
	out := Run(context.Background(), c, Options{Pane: "w1:p3", Name: "helper"})
	if out.Status != "partial" || out.Error == nil || out.Error.Code != "agent_identity_stale" || out.Error.Phase != "state" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if !reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "updated", Kind: "agent_name", ID: "w1:p3"}}) {
		t.Fatalf("%+v", out.Effects)
	}
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	var got identity.Record
	if err := s.Get(identity.Kind, existing.ID, &got); err != nil || got.EndedAt == nil || *got.Name != "worker" || got.Pane != "w1:p9" {
		t.Fatalf("%+v %v", got, err)
	}
}

// withSession is the live agent in pane carrying a claude session ref.
func withSession(pane string, name *string) herdr.AgentResult {
	r := agent(pane, name)
	source, harness, kind, value := "herdr:claude", "claude", "id", "s-1"
	r.Agent.AgentSession = &herdr.AgentSession{Source: &source, Agent: &harness, Kind: &kind, Value: &value}
	return r
}

func TestAdoptRecordsNativeSession(t *testing.T) {
	c := client(t,
		call{Method: "agent.get", Result: withSession("w1:p3", named("worker"))},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()},
	)
	out := Run(context.Background(), c, Options{Pane: "w1:p3"})
	r := out.Result.(Result)
	if out.Error != nil || r.NativeSession == nil || r.NativeSession.Kind != "id" || r.NativeSession.Value != "s-1" || r.NativeSession.Harness != "claude" {
		t.Fatalf("%+v %+v", out.Error, r.NativeSession)
	}
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	var stored identity.Record
	if err := s.Get(identity.Kind, r.ID, &stored); err != nil || !reflect.DeepEqual(stored, r.Record) {
		t.Fatalf("%+v %v", stored, err)
	}
	if last := out.Effects[len(out.Effects)-1]; last != (libagent.Effect{Action: "updated", Kind: "native_session", ID: r.ID}) {
		t.Fatalf("%+v", out.Effects)
	}
}

func TestAdoptNamingRegisteredAgentRecordsNativeSession(t *testing.T) {
	c := client(t,
		call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: agent("w1:p3", nil)},
		call{Method: "agent.rename", Params: map[string]any{"target": "w1:p3", "name": "helper"}, Result: withSession("w1:p3", named("helper"))},
	)
	s, err := identity.OpenStore(context.Background(), c.Cwd, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := identity.Register(context.Background(), s, libagent.Client{}, agent("w1:p3", nil).Agent, "spawn", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), c, Options{Pane: "w1:p3", Name: "helper"})
	r := out.Result.(Result)
	if out.Error != nil || r.ID != rec.ID || r.NativeSession == nil || r.NativeSession.Value != "s-1" {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

func TestAdoptSessionWriteFailureIsWarning(t *testing.T) {
	restore := observeSession
	t.Cleanup(func() { observeSession = restore })
	observeSession = func(*state.Store, string, herdr.AgentSession, time.Time) (identity.Record, bool, error) {
		return identity.Record{}, false, fmt.Errorf("disk full")
	}
	c := client(t,
		call{Method: "agent.get", Result: withSession("w1:p3", named("worker"))},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()},
	)
	out := Run(context.Background(), c, Options{Pane: "w1:p3"})
	r := out.Result.(Result)
	if out.Status != "success" || out.Error != nil || out.ExitCode() != 0 || r.NativeSession != nil {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if last := out.Effects[len(out.Effects)-1]; last != (libagent.Effect{Action: "warning", Kind: "native_session", ID: r.ID}) {
		t.Fatalf("%+v", out.Effects)
	}
}
