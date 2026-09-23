package identity

import (
	"context"
	"errors"
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
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

type call = herdrscript.Call

func repository(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, b)
	}
	return root
}

// client scripts calls with the caller in old:p1 and cwd inside a fresh repository.
func client(t *testing.T, calls ...call) libagent.Client {
	c := herdrscript.Client(t, calls...)
	c.Cwd = repository(t)
	return c
}
func store(t *testing.T, c libagent.Client) *state.Store {
	t.Helper()
	s, err := OpenStore(context.Background(), c.Cwd, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func details(pane, terminal string) herdr.AgentDetails {
	p := herdrscript.LiveAgent("idle")
	p.PaneID = pane
	d := herdrscript.Info(p).Agent
	d.TerminalID = terminal
	return d
}
func info(d herdr.AgentDetails) herdr.AgentResult {
	return herdr.AgentResult{Type: "agent_info", Agent: d}
}
func notFound() error {
	return &herdr.Error{Code: "agent_not_found", Message: "no agent"}
}
func code(err error) string {
	var remote *herdr.Error
	if errors.As(err, &remote) {
		return remote.Code
	}
	return ""
}

func TestRegisterRecordsAgentWithoutParent(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()})
	s := store(t, c)
	tree := "/repo/.fledge/worktrees/w"
	before := time.Now().Add(-time.Second)
	rec, err := Register(context.Background(), s, c, details("w1:p3", "term_a"), "spawn", &tree)
	if err != nil {
		t.Fatal(err)
	}
	var stored Record
	if err := s.Get(Kind, rec.ID, &stored); err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339, stored.RegisteredAt)
	if err != nil || at.Before(before) {
		t.Fatalf("registered_at %q: %v", stored.RegisteredAt, err)
	}
	if stored.ID != rec.ID || *stored.Name != "worker" || stored.Pane != "w1:p3" || stored.WorkspaceID != "w1" || *stored.Harness != "claude" ||
		*stored.Session != "dev" || stored.TerminalID != "term_a" || stored.Parent != nil || stored.RegisteredBy != "spawn" ||
		*stored.WorktreePath != tree || stored.EndedAt != nil {
		t.Fatalf("%+v", stored)
	}
}

func TestRegisterParentIsCallersLiveRecord(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	parent := details("old:p1", "term_parent")
	c := client(t, call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: info(parent)})
	s := store(t, c)
	p, err := Register(context.Background(), s, c, parent, "adopt", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := Register(context.Background(), s, c, details("w1:p3", "term_child"), "spawn", nil)
	if err != nil {
		t.Fatal(err)
	}
	if child.Parent == nil || *child.Parent != p.ID {
		t.Fatalf("parent %v want %s", child.Parent, p.ID)
	}
}

func TestRegisterParentNullWhenCallerRecordIsStale(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	// The caller's pane now hosts a different terminal than the record names.
	c := client(t, call{Method: "agent.get", Err: notFound()},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: info(details("old:p1", "term_new"))})
	s := store(t, c)
	if _, err := Register(context.Background(), s, c, details("old:p1", "term_old"), "adopt", nil); err != nil {
		t.Fatal(err)
	}
	child, err := Register(context.Background(), s, c, details("w1:p3", "term_child"), "spawn", nil)
	if err != nil || child.Parent != nil {
		t.Fatalf("%+v %v", child, err)
	}
}

func TestRegisterRequiresTerminalID(t *testing.T) {
	c := client(t)
	if _, err := Register(context.Background(), store(t, c), c, details("w1:p3", ""), "spawn", nil); err == nil {
		t.Fatal("registered an agent without a terminal id")
	}
}

func TestOpenStoreOutsideRepositoryFails(t *testing.T) {
	out := libagent.Outcome{}
	if _, err := OpenStore(context.Background(), t.TempDir(), &out); err == nil || len(out.Effects) != 0 {
		t.Fatalf("%v %+v", err, out)
	}
}

func TestOpenStoreCreatesIgnoredStateDirectory(t *testing.T) {
	root := repository(t)
	out := libagent.Outcome{}
	if _, err := OpenStore(context.Background(), root, &out); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, ".fledge", "state")); err != nil || !info.IsDir() {
		t.Fatal(err)
	}
	if len(out.Effects) == 0 {
		t.Fatal("created .fledge without recording effects")
	}
}

func registered(t *testing.T, c libagent.Client, d herdr.AgentDetails) Record {
	t.Helper()
	c.CallerPane = ""
	rec, err := Register(context.Background(), store(t, c), c, d, "spawn", nil)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestResolveMatchesLiveTerminal(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	live := details("w1:p3", "term_a")
	c := client(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: info(live)})
	rec := registered(t, c, live)
	got, a, err := Resolve(context.Background(), store(t, c), c, rec.ID)
	if err != nil || got.ID != rec.ID || a.TerminalID != "term_a" {
		t.Fatalf("%+v %+v %v", got, a, err)
	}
}

// listed is an agent.list result of agents.
func listed(agents ...herdr.AgentDetails) map[string]any {
	return map[string]any{"type": "agent_list", "agents": append([]herdr.AgentDetails{}, agents...)}
}

// panes is a pane.list result of panes.
func panes(panes ...herdr.AgentDetails) map[string]any {
	return map[string]any{"type": "pane_list", "panes": append([]herdr.AgentDetails{}, panes...)}
}

// shell is terminal's pane after its agent exited, leaving no agent in it.
func shell(pane, terminal string) herdr.AgentDetails {
	d := details(pane, terminal)
	d.Name, d.Agent, d.AgentStatus = nil, nil, "unknown"
	return d
}

// moved is terminal's agent after Herdr moved its pane into workspace ws.
func moved(pane, ws, terminal string) herdr.AgentDetails {
	d := details(pane, terminal)
	d.WorkspaceID = ws
	return d
}

func TestResolveFailsClosedWhenTerminalIsGone(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	for name, get := range map[string]call{
		"different terminal": {Method: "agent.get", Result: info(details("w1:p3", "term_b"))},
		"pane without agent": {Method: "agent.get", Err: notFound()},
	} {
		t.Run(name, func(t *testing.T) {
			c := client(t, get, call{Method: "agent.list", Result: listed(details("w1:p3", "term_b"))},
				call{Method: "pane.list", Result: panes(details("w1:p3", "term_b"), shell("w1:p4", "term_c"))})
			rec := registered(t, c, details("w1:p3", "term_a"))
			s := store(t, c)
			if _, _, err := Resolve(context.Background(), s, c, rec.ID); code(err) != "agent_identity_stale" {
				t.Fatalf("%v", err)
			}
			var stored Record
			if err := s.Get(Kind, rec.ID, &stored); err != nil || stored.EndedAt == nil {
				t.Fatalf("record not ended: %+v %v", stored, err)
			}
			if _, err := time.Parse(time.RFC3339, *stored.EndedAt); err != nil {
				t.Fatal(err)
			}
			if live, err := Live(s, "term_a"); err != nil || live != nil {
				t.Fatalf("ended record still live: %+v %v", live, err)
			}
		})
	}
}

// A terminal that still exists but no longer hosts an agent is stale, not
// ended: its harness may be restarted in place.
func TestResolveTerminalWithoutAgentStaysLive(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, call{Method: "agent.get", Err: notFound()}, call{Method: "agent.list", Result: listed()},
		call{Method: "pane.list", Result: panes(shell("w2:p1", "term_a"))})
	rec := registered(t, c, details("w1:p3", "term_a"))
	s := store(t, c)
	if _, _, err := Resolve(context.Background(), s, c, rec.ID); code(err) != "agent_identity_stale" {
		t.Fatalf("%v", err)
	}
	if live, err := Live(s, "term_a"); err != nil || live == nil {
		t.Fatalf("%+v %v", live, err)
	}
}

func TestResolveLookupFailuresLeaveRecordLive(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	down := &herdr.Error{Code: "connection_error", Message: "down"}
	gone := call{Method: "agent.get", Err: notFound()}
	list := func(result any) []call { return []call{gone, {Method: "agent.list", Result: result}} }
	for name, tc := range map[string]struct {
		calls []call
		want  string
	}{
		"agent.get transport":   {[]call{{Method: "agent.get", Err: down}}, "connection_error"},
		"agent.list transport":  {[]call{gone, {Method: "agent.list", Err: down}}, "connection_error"},
		"agent.list type":       {list(map[string]any{"type": "pane_list"}), "protocol_error"},
		"agents missing":        {list(map[string]any{"type": "agent_list"}), "protocol_error"},
		"agents null":           {list(map[string]any{"type": "agent_list", "agents": nil}), "protocol_error"},
		"invalid matching":      {list(listed(herdr.AgentDetails{TerminalID: "term_a"})), "protocol_error"},
		"matching without id":   {list(listed(moved("w2:p1", "w2", ""))), "protocol_error"},
		"invalid unrelated":     {list(listed(herdr.AgentDetails{TerminalID: "term_b"})), "protocol_error"},
		"pane.list transport":   {append(list(listed()), call{Method: "pane.list", Err: down}), "connection_error"},
		"pane.list type":        {append(list(listed()), call{Method: "pane.list", Result: map[string]any{"type": "agent_list"}}), "protocol_error"},
		"panes missing":         {append(list(listed()), call{Method: "pane.list", Result: map[string]any{"type": "pane_list"}}), "protocol_error"},
		"panes null":            {append(list(listed()), call{Method: "pane.list", Result: map[string]any{"type": "pane_list", "panes": nil}}), "protocol_error"},
		"invalid pane":          {append(list(listed()), call{Method: "pane.list", Result: panes(shell("w1:p4", "term_c"), herdr.AgentDetails{TerminalID: "term_b"})}), "protocol_error"},
		"pane without terminal": {append(list(listed()), call{Method: "pane.list", Result: panes(shell("w1:p4", ""))}), "protocol_error"},
	} {
		t.Run(name, func(t *testing.T) {
			c := client(t, tc.calls...)
			rec := registered(t, c, details("w1:p3", "term_a"))
			s := store(t, c)
			if _, _, err := Resolve(context.Background(), s, c, rec.ID); code(err) != tc.want {
				t.Fatalf("%v", err)
			}
			if live, err := Live(s, "term_a"); err != nil || live == nil || live.Pane != "w1:p3" {
				t.Fatalf("%+v %v", live, err)
			}
		})
	}
}

// Herdr gives a pane moved across workspaces a new ID but keeps its terminal.
func TestResolveFollowsMovedTerminal(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	for name, get := range map[string]call{
		"pane gone":   {Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Err: notFound()},
		"pane reused": {Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: info(details("w1:p3", "term_b"))},
	} {
		t.Run(name, func(t *testing.T) {
			there := moved("w2:p1", "w2", "term_a")
			c := client(t, get, call{Method: "agent.list", Result: listed(details("w1:p3", "term_b"), there)})
			rec := registered(t, c, details("w1:p3", "term_a"))
			s := store(t, c)
			got, a, err := Resolve(context.Background(), s, c, rec.ID)
			if err != nil || got.ID != rec.ID || got.Pane != "w2:p1" || got.WorkspaceID != "w2" || a.PaneID != "w2:p1" || a.TerminalID != "term_a" {
				t.Fatalf("%+v %+v %v", got, a, err)
			}
			var stored Record
			if err := s.Get(Kind, rec.ID, &stored); err != nil || stored.Pane != "w2:p1" || stored.WorkspaceID != "w2" || stored.EndedAt != nil {
				t.Fatalf("%+v %v", stored, err)
			}
		})
	}
}

func TestVerifyMatchesTerminalNotPane(t *testing.T) {
	rec := Record{ID: "0000beef", Pane: "w1:p3", TerminalID: "term_a"}
	if err := Verify(rec, moved("w2:p1", "w2", "term_a")); err != nil {
		t.Fatal(err)
	}
	if err := Verify(rec, details("w1:p3", "term_b")); code(err) != "agent_identity_stale" {
		t.Fatalf("%v", err)
	}
}

func TestResolveOtherSessionIsStale(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	rec := registered(t, c, details("w1:p3", "term_a"))
	t.Setenv("HERDR_SESSION", "other")
	if _, _, err := Resolve(context.Background(), store(t, c), c, rec.ID); code(err) != "agent_identity_stale" {
		t.Fatalf("%v", err)
	}
}

func TestResolveEndedRecordIsStale(t *testing.T) {
	c := client(t)
	rec := registered(t, c, details("w1:p3", "term_a"))
	s := store(t, c)
	var r Record
	if err := s.Update(Kind, rec.ID, &r, func() error { ended := "2026-09-22T00:00:00Z"; r.EndedAt = &ended; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Resolve(context.Background(), s, c, rec.ID); code(err) != "agent_identity_stale" {
		t.Fatalf("%v", err)
	}
}

func TestResolveMissingAndInvalidIDs(t *testing.T) {
	c := client(t)
	if _, _, err := Resolve(context.Background(), store(t, c), c, "0000beef"); code(err) != "agent_record_not_found" {
		t.Fatalf("%v", err)
	}
	var input *libagent.InputError
	if _, _, err := Resolve(context.Background(), store(t, c), c, "../x"); !errors.As(err, &input) {
		t.Fatalf("%v", err)
	}
}

func TestLiveFindsUnendedRecordInSession(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	rec := registered(t, c, details("w1:p3", "term_a"))
	s := store(t, c)
	got, err := Live(s, "term_a")
	if err != nil || got == nil || got.ID != rec.ID {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err := Live(s, "term_b"); err != nil || got != nil {
		t.Fatalf("%+v %v", got, err)
	}
	t.Setenv("HERDR_SESSION", "other")
	if got, err := Live(s, "term_a"); err != nil || got != nil {
		t.Fatalf("other session matched: %+v %v", got, err)
	}
}

func TestTargetValidatesExactlyOneSelector(t *testing.T) {
	for _, tg := range []Target{{}, {Name: "a", Pane: "p"}, {Name: "a", ID: "0000beef"}, {Pane: "p", ID: "0000beef"}} {
		var input *libagent.InputError
		if err := tg.Validate(); !errors.As(err, &input) {
			t.Fatalf("%+v: %v", tg, err)
		}
	}
	for _, tg := range []Target{{Name: "a"}, {Pane: "p"}, {ID: "0000beef"}} {
		if err := tg.Validate(); err != nil {
			t.Fatalf("%+v: %v", tg, err)
		}
	}
}

func TestTargetGetByNameAndID(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	live := details("w1:p3", "term_a")
	c := client(t, call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: info(live)},
		call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: info(live)})
	a, target, rec, err := Target{Name: "worker"}.Get(context.Background(), c)
	if err != nil || target != "worker" || rec != nil || a.TerminalID != "term_a" {
		t.Fatalf("%+v %s %+v %v", a, target, rec, err)
	}
	want := registered(t, c, live)
	a, target, rec, err = Target{ID: want.ID}.Get(context.Background(), c)
	if err != nil || target != "w1:p3" || rec == nil || rec.ID != want.ID {
		t.Fatalf("%+v %s %+v %v", a, target, rec, err)
	}
}

func TestTargetGetIDWithoutStore(t *testing.T) {
	c := client(t)
	_, _, _, err := Target{ID: "0000beef"}.Get(context.Background(), c)
	if code(err) != "agent_record_not_found" {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Cwd, ".fledge")); !os.IsNotExist(err) {
		t.Fatalf("lookup created .fledge: %v", err)
	}
}

// A caller whose terminal moved to a new pane keeps its record, which is
// updated to the pane it now occupies.
func TestCallerFollowsMovedTerminal(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, call{Method: "agent.get", Err: notFound()},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: info(moved("old:p1", "old", "term_parent"))})
	s := store(t, c)
	parent, err := Register(context.Background(), s, c, details("w1:p9", "term_parent"), "adopt", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := Register(context.Background(), s, c, details("w1:p3", "term_child"), "spawn", nil)
	if err != nil || child.Parent == nil || *child.Parent != parent.ID {
		t.Fatalf("%+v %v", child, err)
	}
	var stored Record
	if err := s.Get(Kind, parent.ID, &stored); err != nil || stored.Pane != "old:p1" || stored.WorkspaceID != "old" {
		t.Fatalf("%+v %v", stored, err)
	}
}

func TestRegisterRefusesTerminalWithLiveRecord(t *testing.T) {
	c := client(t)
	first := registered(t, c, details("w1:p3", "term_a"))
	_, err := Register(context.Background(), store(t, c), libagent.Client{}, details("w1:p3", "term_a"), "spawn", nil)
	var remote *herdr.Error
	if !errors.As(err, &remote) || remote.Code != "agent_already_registered" || !strings.Contains(remote.Message, first.ID) {
		t.Fatalf("%v", err)
	}
	if ids, _ := store(t, c).List(Kind); len(ids) != 1 {
		t.Fatalf("records %v", ids)
	}
}

func TestExistingOnStateWithoutLockCreatesNothing(t *testing.T) {
	root := repository(t)
	dir := filepath.Join(root, ".fledge", "state")
	if err := os.MkdirAll(filepath.Join(dir, Kind), 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := Existing(context.Background(), root)
	if err != nil || s == nil {
		t.Fatalf("%v %v", s, err)
	}
	if list, _ := os.ReadDir(dir); len(list) != 1 || list[0].Name() != Kind {
		t.Fatalf("state entries changed: %v", list)
	}
}

func TestCallerWithoutAgentHasNoRecord(t *testing.T) {
	c := client(t, call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Err: notFound()})
	if rec, err := Caller(context.Background(), store(t, c), c); rec != nil || err != nil {
		t.Fatalf("%+v %v", rec, err)
	}
}

func TestCallerPropagatesLookupFailures(t *testing.T) {
	for name, get := range map[string]call{
		"transport": {Method: "agent.get", Err: &herdr.Error{Code: "connection_error", Message: "down"}},
		"protocol":  {Method: "agent.get", Result: herdr.AgentResult{Type: "pane_info"}},
		"other":     {Method: "agent.get", Err: &herdr.Error{Code: "agent_not_ready", Message: "busy"}},
	} {
		t.Run(name, func(t *testing.T) {
			c := client(t, get, get)
			rec, err := Caller(context.Background(), store(t, c), c)
			if rec != nil || err == nil {
				t.Fatalf("%+v %v", rec, err)
			}
			if _, err := Register(context.Background(), store(t, c), c, details("w1:p3", "term_a"), "spawn", nil); err == nil {
				t.Fatal("registered without a provable parent lookup")
			}
		})
	}
}

func TestRelocateRefusesEndedRecord(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	rec := registered(t, c, details("w1:p3", "term_a"))
	s := store(t, c)
	if err := End(s, rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Relocate(s, rec, moved("w2:p1", "w2", "term_a")); code(err) != "agent_identity_stale" {
		t.Fatalf("%v", err)
	}
	var stored Record
	if err := s.Get(Kind, rec.ID, &stored); err != nil || stored.Pane != "w1:p3" || stored.WorkspaceID != "w1" {
		t.Fatalf("%+v %v", stored, err)
	}
}

func TestEndKeepsOriginalTime(t *testing.T) {
	c := client(t)
	rec := registered(t, c, details("w1:p3", "term_a"))
	s := store(t, c)
	first := "2026-01-01T00:00:00Z"
	var r Record
	if err := s.Update(Kind, rec.ID, &r, func() error { r.EndedAt = &first; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := End(s, rec.ID); err != nil {
		t.Fatal(err)
	}
	var stored Record
	if err := s.Get(Kind, rec.ID, &stored); err != nil || stored.EndedAt == nil || *stored.EndedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("%+v %v", stored, err)
	}
}

// A record that cannot be ended reports the store failure, not staleness.
func TestResolveEndFailureIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, call{Method: "agent.get", Err: notFound()}, call{Method: "agent.list", Result: listed()}, call{Method: "pane.list", Result: panes()})
	rec := registered(t, c, details("w1:p3", "term_a"))
	s := store(t, c)
	agents := filepath.Join(c.Cwd, ".fledge", "state", Kind)
	if err := os.Chmod(agents, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(agents, 0o700) })
	if _, _, err := Resolve(context.Background(), s, c, rec.ID); err == nil || code(err) == "agent_identity_stale" {
		t.Fatalf("%v", err)
	}
	if live, err := Live(s, "term_a"); err != nil || live == nil {
		t.Fatalf("%+v %v", live, err)
	}
}

// running is d with its live harness set to harness, or unknown when "".
func running(d herdr.AgentDetails, harness string) herdr.AgentDetails {
	d.Agent = nil
	if harness != "" {
		d.Agent = &harness
	}
	return d
}

func ended(t *testing.T, s *state.Store, id string) bool {
	t.Helper()
	var rec Record
	if err := s.Get(Kind, id, &rec); err != nil {
		t.Fatal(err)
	}
	return rec.EndedAt != nil
}

func TestMismatchedComparesOnlyKnownHarnesses(t *testing.T) {
	for _, tc := range []struct {
		recorded, live string
		want           bool
	}{{"codex", "claude", true}, {"claude", "claude", false}, {"", "claude", false}, {"codex", "", false}, {"", "", false}} {
		rec := Record{TerminalID: "term_a"}
		if tc.recorded != "" {
			rec.Harness = &tc.recorded
		}
		if got := Mismatched(rec, running(details("w1:p3", "term_a"), tc.live)); got != tc.want {
			t.Errorf("recorded %q live %q: got %v", tc.recorded, tc.live, got)
		}
	}
}

func TestCallerEndsRecordOfDifferentHarness(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: info(running(details("old:p1", "term_a"), "claude"))})
	s := store(t, c)
	old := registered(t, c, running(details("old:p1", "term_a"), "codex"))
	rec, err := Caller(context.Background(), s, c)
	if err != nil || rec != nil {
		t.Fatalf("%+v %v", rec, err)
	}
	if !ended(t, s, old.ID) {
		t.Fatal("mismatched record still live")
	}
}

func TestCallerKeepsRecordWhenHarnessUnknown(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: info(running(details("old:p1", "term_a"), ""))})
	s := store(t, c)
	old := registered(t, c, running(details("old:p1", "term_a"), "codex"))
	rec, err := Caller(context.Background(), s, c)
	if err != nil || rec == nil || rec.ID != old.ID || ended(t, s, old.ID) {
		t.Fatalf("%+v %v", rec, err)
	}
}

func TestResolveEndsRecordOfDifferentHarness(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: info(running(details("w1:p3", "term_a"), "claude"))})
	s := store(t, c)
	old := registered(t, c, running(details("w1:p3", "term_a"), "codex"))
	_, _, err := Resolve(context.Background(), s, c, old.ID)
	if code(err) != "agent_identity_stale" || !strings.Contains(err.Error(), "codex") || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("%v", err)
	}
	if !ended(t, s, old.ID) {
		t.Fatal("mismatched record still live")
	}
}

func TestRegisterReplacesRecordOfDifferentHarness(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	s := store(t, c)
	old := registered(t, c, running(details("w1:p3", "term_a"), "codex"))
	now := running(details("w1:p3", "term_a"), "claude")
	if err := Unregistered(s, now); err != nil {
		t.Fatal(err)
	}
	rec, err := Register(context.Background(), s, libagent.Client{}, now, "adopt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ended(t, s, old.ID) {
		t.Fatal("mismatched record still live")
	}
	if live, err := Live(s, "term_a"); err != nil || live == nil || live.ID != rec.ID {
		t.Fatalf("%+v %v", live, err)
	}
}

func TestVerifyRefusesDifferentHarness(t *testing.T) {
	codex := "codex"
	rec := Record{ID: "0000beef", Pane: "w1:p3", TerminalID: "term_a", Harness: &codex}
	if err := Verify(rec, running(details("w1:p3", "term_a"), "claude")); code(err) != "agent_identity_stale" {
		t.Fatalf("%v", err)
	}
}

func TestAttributedSkipsDifferentHarness(t *testing.T) {
	codex := "codex"
	records := map[string]Record{"term_a": {ID: "0000beef", TerminalID: "term_a", Harness: &codex}, "": {ID: "0000dead", Harness: &codex}}
	if _, ok := Attributed(records, running(details("w1:p3", "term_a"), "claude")); ok {
		t.Fatal("attributed a record of another harness")
	}
	if rec, ok := Attributed(records, running(details("w1:p3", "term_a"), "codex")); !ok || rec.ID != "0000beef" {
		t.Fatalf("%+v %v", rec, ok)
	}
	if _, ok := Attributed(records, running(details("w1:p3", ""), "codex")); ok {
		t.Fatal("attributed an agent without a terminal")
	}
}

// countScans counts full scans of the agent records until the test ends.
func countScans(t *testing.T) *int {
	t.Helper()
	n, real := 0, scan
	scan = func(r reader) (map[string]Record, []string, error) { n++; return real(r) }
	t.Cleanup(func() { scan = real })
	return &n
}

func TestRegisterScansRecordsOnce(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	caller := details("old:p1", "term_parent")
	// The caller has moved panes since registering, and the terminal being
	// registered has a live record left by a different harness: the parent
	// lookup, relocation, harness check, end, and create share one scan.
	c := client(t, call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: info(moved("new:p1", "new", "term_parent"))})
	s := store(t, c)
	parent := registered(t, c, caller)
	old := registered(t, c, running(details("w1:p3", "term_a"), "codex"))
	n := countScans(t)
	rec, err := Register(context.Background(), s, c, running(details("w1:p3", "term_a"), "claude"), "adopt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if *n != 1 {
		t.Fatalf("Register scanned the agent records %d times, want 1", *n)
	}
	if rec.Parent == nil || *rec.Parent != parent.ID || !ended(t, s, old.ID) {
		t.Fatalf("%+v; old ended %v", rec, ended(t, s, old.ID))
	}
	var moved Record
	if err := s.Get(Kind, parent.ID, &moved); err != nil || moved.Pane != "new:p1" || moved.WorkspaceID != "new" {
		t.Fatalf("%+v %v", moved, err)
	}
}

func TestConcurrentRegistrationsAcrossHarnessesLeaveOneLiveRecord(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	s := store(t, client(t))
	// Different harnesses replace each other's record, so without one lock
	// over check, end, and create, a registration can miss a concurrent one's
	// record and leave both live.
	for round := range 20 {
		terminal := fmt.Sprintf("term_%d", round)
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := range 8 {
			harness := []string{"claude", "codex"}[i%2]
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := Register(context.Background(), s, libagent.Client{}, running(details("w1:p3", terminal), harness), "adopt", nil)
				if err != nil && code(err) != "agent_already_registered" {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		ids, err := s.List(Kind)
		if err != nil {
			t.Fatal(err)
		}
		live := 0
		for _, id := range ids {
			var rec Record
			if err := s.Get(Kind, id, &rec); err != nil {
				t.Fatal(err)
			}
			if rec.TerminalID == terminal && rec.EndedAt == nil {
				live++
			}
		}
		if live != 1 {
			t.Fatalf("round %d: %d live records for %s, want 1", round, live, terminal)
		}
	}
}

func TestEndArchivesRecordKeepingItLoadable(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	s := store(t, c)
	rec := registered(t, c, details("w1:p3", "term_a"))
	if err := End(s, rec.ID); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(c.Cwd, ".fledge", "state", Kind)
	if _, err := os.Stat(filepath.Join(agents, "archive", rec.ID+".json")); err != nil {
		t.Fatalf("ended record not archived: %v", err)
	}
	n := countScans(t)
	if records, err := LiveByTerminal(s); err != nil || len(records) != 0 {
		t.Fatalf("%+v %v", records, err)
	}
	if ids, err := s.List(Kind); err != nil || len(ids) != 0 {
		t.Fatalf("live scan still lists %v %v", ids, err)
	}
	if *n != 1 || !ended(t, s, rec.ID) {
		t.Fatalf("scans %d", *n)
	}
	if _, _, err := Resolve(context.Background(), s, c, rec.ID); code(err) != "agent_identity_stale" || !strings.Contains(err.Error(), "ended at") {
		t.Fatalf("%v", err)
	}
}

func TestLegacyEndedRecordIsArchivedUnderTheLock(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	s := store(t, c)
	// An ended record written before archiving still sits with the live ones.
	at, dev, claude := "2026-01-01T00:00:00Z", "dev", "claude"
	legacy, err := s.Create(Kind, func(id string) any {
		return Record{ID: id, Pane: "w1:p3", TerminalID: "term_a", Session: &dev, Harness: &claude, EndedAt: &at}
	})
	if err != nil {
		t.Fatal(err)
	}
	if records, err := LiveByTerminal(s); err != nil || len(records) != 0 {
		t.Fatalf("%+v %v", records, err)
	}
	rec := registered(t, c, details("w1:p3", "term_a"))
	if ids, err := s.List(Kind); err != nil || len(ids) != 1 || ids[0] != rec.ID {
		t.Fatalf("live folder holds %v %v, want only %s", ids, err, rec.ID)
	}
	var stored Record
	if err := s.Get(Kind, legacy, &stored); err != nil || stored.EndedAt == nil || *stored.EndedAt != at {
		t.Fatalf("%+v %v", stored, err)
	}
	if live, err := Live(s, "term_a"); err != nil || live == nil || live.ID != rec.ID {
		t.Fatalf("%+v %v", live, err)
	}
}

func TestEndOnceReportsWhetherThisCallEnded(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	s := store(t, c)
	rec := registered(t, c, details("w1:p3", "term_a"))
	for i, want := range []bool{true, false} {
		ended, err := EndOnce(s, rec.ID)
		if err != nil || ended != want {
			t.Fatalf("call %d: ended %v, %v; want %v", i, ended, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(c.Cwd, ".fledge", "state", Kind, "archive", rec.ID+".json")); err != nil {
		t.Fatal(err)
	}
}

func TestReopenReturnsEndedRecordToLiveScans(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	s := store(t, c)
	rec := registered(t, c, details("w1:p3", "term_a"))
	if _, err := EndOnce(s, rec.ID); err != nil {
		t.Fatal(err)
	}
	if err := Reopen(s, rec.ID); err != nil {
		t.Fatal(err)
	}
	if live, err := LiveByTerminal(s); err != nil || live["term_a"].ID != rec.ID {
		t.Fatalf("%+v %v", live, err)
	}
	agents := filepath.Join(c.Cwd, ".fledge", "state", Kind)
	if _, err := os.Stat(filepath.Join(agents, rec.ID+".json")); err != nil {
		t.Fatalf("reopened record not live: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agents, "archive", rec.ID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reopened record still archived: %v", err)
	}
	if ended(t, s, rec.ID) {
		t.Fatal("reopened record still ended")
	}
}

func TestReopenRefusesTerminalRegisteredAgain(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	s := store(t, c)
	old := registered(t, c, details("w1:p3", "term_a"))
	if _, err := EndOnce(s, old.ID); err != nil {
		t.Fatal(err)
	}
	now := registered(t, c, details("w1:p3", "term_a"))
	err := Reopen(s, old.ID)
	if code(err) != "agent_already_registered" || !strings.Contains(err.Error(), now.ID) {
		t.Fatalf("%v", err)
	}
	if !ended(t, s, old.ID) {
		t.Fatal("refused Reopen un-ended the record")
	}
	if live, err := LiveByTerminal(s); err != nil || live["term_a"].ID != now.ID {
		t.Fatalf("%+v %v", live, err)
	}
	if ids, err := s.List(Kind); err != nil || len(ids) != 1 || ids[0] != now.ID {
		t.Fatalf("refused Reopen left %v %v in the live folder", ids, err)
	}
}

func TestReopenOfLiveRecordIsNoop(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	s := store(t, c)
	rec := registered(t, c, details("w1:p3", "term_a"))
	if err := Reopen(s, rec.ID); err != nil {
		t.Fatal(err)
	}
	var stored Record
	if err := s.Get(Kind, rec.ID, &stored); err != nil || !reflect.DeepEqual(stored, rec) {
		t.Fatalf("%+v %v", stored, err)
	}
}

func TestReopenUnknownOrInvalidRecord(t *testing.T) {
	c := client(t)
	s := store(t, c)
	if err := Reopen(s, "0123abcd"); code(err) != "agent_record_not_found" {
		t.Fatalf("%v", err)
	}
	var input *libagent.InputError
	if err := Reopen(s, "nope"); !errors.As(err, &input) {
		t.Fatalf("%v", err)
	}
}

func TestReopenLegacyEndedRecordInLiveFolder(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t)
	s := store(t, c)
	at, dev := "2026-01-01T00:00:00Z", "dev"
	id, err := s.Create(Kind, func(id string) any {
		return Record{ID: id, Pane: "w1:p3", TerminalID: "term_a", Session: &dev, EndedAt: &at}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Reopen(s, id); err != nil {
		t.Fatal(err)
	}
	if live, err := LiveByTerminal(s); err != nil || live["term_a"].ID != id {
		t.Fatalf("%+v %v", live, err)
	}
}
