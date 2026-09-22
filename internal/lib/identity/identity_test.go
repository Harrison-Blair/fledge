package identity

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestResolveFailsClosed(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	for name, tc := range map[string]struct {
		get  call
		want string
	}{
		"different terminal": {call{Method: "agent.get", Result: info(details("w1:p3", "term_b"))}, "agent_identity_stale"},
		"pane without agent": {call{Method: "agent.get", Err: notFound()}, "agent_identity_stale"},
		"transport failure":  {call{Method: "agent.get", Err: &herdr.Error{Code: "connection_error", Message: "down"}}, "connection_error"},
	} {
		t.Run(name, func(t *testing.T) {
			c := client(t, tc.get)
			rec := registered(t, c, details("w1:p3", "term_a"))
			_, _, err := Resolve(context.Background(), store(t, c), c, rec.ID)
			if code(err) != tc.want {
				t.Fatalf("%v", err)
			}
		})
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

func TestRegisterParentRequiresRecordedPane(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, call{Method: "agent.get", Err: notFound()},
		call{Method: "agent.get", Params: map[string]any{"target": "old:p1"}, Result: info(details("old:p1", "term_parent"))})
	s := store(t, c)
	if _, err := Register(context.Background(), s, c, details("old:p9", "term_parent"), "adopt", nil); err != nil {
		t.Fatal(err)
	}
	child, err := Register(context.Background(), s, c, details("w1:p3", "term_child"), "spawn", nil)
	if err != nil || child.Parent != nil {
		t.Fatalf("%+v %v", child, err)
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
