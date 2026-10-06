// Package herdrscript scripts Herdr requests and supplies fixtures for agent
// leaf tests. Production packages must not import it.
package herdrscript

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Call is one expected request and its scripted response.
type Call struct {
	Method string
	Params map[string]any
	Result any
	Err    error
	// Before, if set, runs as this call is served, so a test can simulate a
	// side effect (e.g. time passing) tied to the RPC itself rather than to
	// how many times something else was called.
	Before func()
}
type script struct {
	t     *testing.T
	calls []Call
	index int
}

func (s *script) Call(_ context.Context, method string, params any, result any) error {
	s.t.Helper()
	if s.index >= len(s.calls) {
		s.t.Fatalf("unexpected call %s %#v", method, params)
	}
	want := s.calls[s.index]
	s.index++
	if method != want.Method {
		s.t.Fatalf("call %d: got %s want %s", s.index, method, want.Method)
	}
	if want.Before != nil {
		want.Before()
	}
	data, _ := json.Marshal(params)
	var got map[string]any
	json.Unmarshal(data, &got)
	if want.Params != nil {
		b, _ := json.Marshal(want.Params)
		var normalized map[string]any
		json.Unmarshal(b, &normalized)
		if !reflect.DeepEqual(got, normalized) {
			s.t.Fatalf("%s params %#v want %#v", method, got, normalized)
		}
	}
	if want.Err != nil {
		return want.Err
	}
	b, _ := json.Marshal(want.Result)
	return json.Unmarshal(b, result)
}

// Client serves exactly calls, in order, and fails t if any go unused.
func Client(t *testing.T, calls ...Call) libagent.Client {
	t.Helper()
	s := &script{t: t, calls: calls}
	t.Cleanup(func() {
		if s.index != len(s.calls) {
			t.Errorf("used %d of %d calls", s.index, len(s.calls))
		}
	})
	return libagent.Client{API: s, CallerPane: "old:p1", Cwd: t.TempDir()}
}

func Pane(id, ws, tab string) herdr.Pane { return herdr.Pane{PaneID: id, WorkspaceID: ws, TabID: tab} }

// LiveAgent is a named claude agent in w1:p3 with the given status.
func LiveAgent(status string) herdr.Pane {
	p := Pane("w1:p3", "w1", "w1:t2")
	p.AgentStatus = status
	name, harness, cwd := "worker", "claude", "/repo"
	p.Name, p.Agent, p.Cwd = &name, &harness, &cwd
	return p
}

// Row is a fully populated agent row for renderer tests.
func Row() libagent.AgentRow {
	s := func(v string) *string { return &v }
	return libagent.AgentRow{Name: s("worker"), Harness: s("claude"), AgentStatus: s("idle"), WorkspaceID: s("w1"), TabID: s("w1:t1"), PaneID: s("w1:p1"), Cwd: s("/repo")}
}

// Info is a complete agent_info result for p.
func Info(p herdr.Pane) herdr.AgentResult {
	f := false
	var rev uint64
	return herdr.AgentResult{Type: "agent_info", Agent: herdr.AgentDetails{Pane: p, TerminalID: "term_x", Focused: &f, Revision: &rev}}
}

// List is an agent.list call that returns agents, or an empty list when none
// are given.
func List(agents ...herdr.AgentDetails) Call {
	if agents == nil {
		agents = []herdr.AgentDetails{}
	}
	return Call{Method: "agent.list", Result: herdr.AgentListResult{Type: "agent_list", Agents: agents}}
}

// Get is an agent.get call for target that returns a.
func Get(target string, a herdr.AgentDetails) Call {
	return Call{Method: "agent.get", Params: map[string]any{"target": target}, Result: herdr.AgentResult{Type: "agent_info", Agent: a}}
}

// Waited builds the agent.wait settled-state result for a pane already hosting an agent.
func Waited(p herdr.Pane, status string) herdr.AgentResult {
	p.AgentStatus = status
	return Info(p)
}

// OK is the bare acknowledgement returned by pane.close and agent.send_keys.
func OK() map[string]any { return map[string]any{"type": "ok"} }

// countWriter counts the writes of a successful render.
type countWriter struct{ writes int }

func (w *countWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

// failAfterWriter checks errors on later writes as well as the initial write.
type failAfterWriter struct {
	remaining int
	err       error
	failed    bool
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		w.failed = true
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

// CheckOutputFailures fails at every write boundary of each outcome, in human
// and JSON form, and requires Finish to report the write error unchanged.
func CheckOutputFailures(t *testing.T, render libagent.HumanRenderer, outcomes ...libagent.Outcome) {
	t.Helper()
	for i, out := range outcomes {
		for _, asJSON := range []bool{false, true} {
			var count countWriter
			if err := out.Write(&count, asJSON, render); err != nil {
				t.Fatal(err)
			}
			// Fail at each write of the successful render, then confirm that
			// the render makes no more writes than counted.
			for after := 0; after <= count.writes; after++ {
				sentinel := errors.New("output unavailable")
				w := &failAfterWriter{remaining: after, err: sentinel}
				err := libagent.Finish(out, w, asJSON, render)
				if after == count.writes {
					if w.failed {
						t.Fatalf("case %d json=%v: more than %d writes", i, asJSON, count.writes)
					}
					break
				}
				var outputErr *cli.OutputError
				if !w.failed {
					t.Fatalf("case %d json=%v: %d writes, want %d", i, asJSON, after, count.writes)
				}
				if !errors.As(err, &outputErr) {
					t.Fatalf("output failure lost: %v", err)
				}
				if !errors.Is(err, sentinel) || outputErr.ExitCode() != 1 || outputErr.Error() != sentinel.Error() {
					t.Fatalf("case %d json=%v write=%d: %v", i, asJSON, after, err)
				}
			}
		}
	}
}
