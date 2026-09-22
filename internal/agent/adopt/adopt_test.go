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
	if !r.Renamed || *r.Name != "helper" || r.Pane != "old:p1" || r.TerminalID != "term_a" || r.RegisteredBy != "adopt" || r.Parent != nil || r.WorktreePath != nil {
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
