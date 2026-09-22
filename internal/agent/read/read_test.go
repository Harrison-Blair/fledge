package read

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

var fake = herdrscript.Client

func readResult(source, text string, truncated bool) map[string]any {
	return map[string]any{"type": "pane_read", "read": map[string]any{"pane_id": "w1:p3", "workspace_id": "w1", "tab_id": "w1:t2", "source": source, "format": "text", "text": text, "revision": 9, "truncated": truncated}}
}

func getCall(target string) call {
	return call{Method: "agent.get", Params: map[string]any{"target": target}, Result: herdrscript.Info(herdrscript.LiveAgent("working"))}
}

func TestReadTranslatesSourceSpelling(t *testing.T) {
	for cli, wire := range map[string]string{"visible": "visible", "recent": "recent", "recent-unwrapped": "recent_unwrapped", "detection": "detection"} {
		out := Run(context.Background(), fake(t, getCall("worker"),
			call{Method: "agent.read", Params: map[string]any{"target": "w1:p3", "source": wire, "format": "text"}, Result: readResult(wire, "a\nb\n", false)}),
			Options{Name: "worker", Source: cli})
		if out.Operation != "agent.read" || out.Status != "success" || out.ExitCode() != 0 || len(out.Effects) != 0 {
			t.Fatalf("%s: %+v", cli, out)
		}
		r := out.Result.(Result)
		if r.Source != cli || r.Text != "a\nb\n" || r.Lines != 2 || r.Revision != 9 || r.Truncated || !reflect.DeepEqual(r.AgentRow, libagent.NewAgentRow(herdrscript.LiveAgent("working"))) {
			t.Fatalf("%s: %+v", cli, r)
		}
	}
}

func TestReadRejectsInvalidInputWithoutCalls(t *testing.T) {
	for _, o := range []Options{
		{Source: "recent"},
		{Name: "worker", Pane: "w1:p3", Source: "recent"},
		{Name: "worker", Source: "recent_unwrapped"},
		{Name: "worker", Source: ""},
		{Name: "worker", Source: "ansi"},
		{Name: "worker", Source: "recent", Lines: -1, LinesSet: true},
		{Name: "worker", Source: "recent", Lines: 1 << 32, LinesSet: true},
	} {
		out := Run(context.Background(), fake(t), o)
		if out.ExitCode() != 2 || out.Status != "rejected" || out.Error.Phase != "validation" || out.Error.Code != "invalid_input" {
			t.Fatalf("%+v: %+v", o, out)
		}
	}
}

func TestReadForwardsLines(t *testing.T) {
	for _, lines := range []int64{0, 1000, 1<<32 - 1} {
		out := Run(context.Background(), fake(t, getCall("w1:p3"),
			call{Method: "agent.read", Params: map[string]any{"target": "w1:p3", "source": "recent", "format": "text", "lines": lines}, Result: readResult("recent", "", true)}),
			Options{Pane: "w1:p3", Source: "recent", Lines: lines, LinesSet: true})
		if out.Error != nil || out.Result.(Result).Lines != 0 || !out.Result.(Result).Truncated {
			t.Fatalf("%d: %+v", lines, out)
		}
	}
}

func TestReadFailures(t *testing.T) {
	out := Run(context.Background(), fake(t, call{Method: "agent.get", Err: &herdr.Error{Code: "agent_not_found", Message: "missing"}}), Options{Name: "worker", Source: "recent"})
	if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != "agent_not_found" || out.Error.Phase != "agent.get" || out.Result != nil {
		t.Fatalf("%+v", out)
	}
	out = Run(context.Background(), fake(t, getCall("worker"), call{Method: "agent.read", Err: &herdr.Error{Code: "agent_not_found", Message: "gone"}}), Options{Name: "worker", Source: "recent"})
	if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != "agent_not_found" || out.Error.Phase != "agent.read" {
		t.Fatalf("%+v", out)
	}
	moved := readResult("recent", "x", false)
	moved["read"].(map[string]any)["pane_id"] = "w1:p9"
	out = Run(context.Background(), fake(t, getCall("worker"), call{Method: "agent.read", Result: moved}), Options{Name: "worker", Source: "recent"})
	if out.ExitCode() != 1 || out.Error.Code != "protocol_error" || out.Error.Phase != "agent.read" {
		t.Fatalf("%+v", out)
	}
}

func TestRender(t *testing.T) {
	named := Result{AgentRow: herdrscript.Row(), Source: "recent-unwrapped", Lines: 2, Text: "one\ntwo", Truncated: true}
	unnamed := named
	unnamed.Name, unnamed.Text, unnamed.Lines, unnamed.Truncated = nil, "", 0, false
	for _, tc := range []struct {
		r    Result
		want string
	}{
		{named, "Terminal snapshot of worker (recent-unwrapped, 2 rows, truncated: yes)\none\ntwo\n"},
		{unnamed, "Terminal snapshot of w1:p1 (recent-unwrapped, 0 rows, truncated: no)\n"},
	} {
		var b bytes.Buffer
		if err := Render(&b, libagent.Outcome{Result: tc.r}); err != nil || b.String() != tc.want {
			t.Fatalf("%v %q", err, b.String())
		}
	}
	var b bytes.Buffer
	if err := (libagent.Outcome{Operation: "agent.read", Status: "success", Result: named, Effects: []libagent.Effect{}}).Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	json.Unmarshal(b.Bytes(), &decoded)
	result := decoded["result"].(map[string]any)
	if result["text"] != "one\ntwo" {
		t.Fatalf("JSON text not byte-exact: %q", result["text"])
	}
	for _, key := range []string{"name", "harness", "agent_status", "workspace_id", "tab_id", "pane_id", "cwd", "source", "lines", "text", "revision", "truncated"} {
		if _, ok := result[key]; !ok {
			t.Fatalf("missing %s: %s", key, b.String())
		}
	}
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Operation: "agent.read", Status: "success", Result: named, Effects: []libagent.Effect{}})
}

// TestReadJSONPreservesSnapshotBytes checks that only human output gains a
// final newline; JSON keeps a snapshot without one unchanged.
func TestReadJSONPreservesSnapshotBytes(t *testing.T) {
	out := Run(context.Background(), fake(t, getCall("worker"), call{Method: "agent.read", Result: readResult("recent", "a\nb", false)}), Options{Name: "worker", Source: "recent"})
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	var decoded struct{ Result struct{ Text string } }
	if err := json.Unmarshal(b.Bytes(), &decoded); err != nil || decoded.Result.Text != "a\nb" {
		t.Fatalf("%v %q", err, b.String())
	}
}

func TestReadByIDReadsVerifiedPane(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("working"))
	c := fake(t, getCall("w1:p3"), call{Method: "agent.read", Params: map[string]any{"target": "w1:p3", "source": "recent_unwrapped", "format": "text"}, Result: readResult("recent_unwrapped", "x\n", false)})
	c.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, c.Cwd, live.Agent)
	if out := Run(context.Background(), c, Options{ID: rec.ID, Source: "recent-unwrapped"}); out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	stale := live.Agent
	stale.TerminalID = "term_old"
	c = fake(t, getCall("w1:p3"), call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []any{}}})
	c.Cwd = identitytest.Repository(t)
	rec = identitytest.Register(t, c.Cwd, stale)
	if out := Run(context.Background(), c, Options{ID: rec.ID, Source: "recent-unwrapped"}); out.Error == nil || out.Error.Code != "agent_identity_stale" {
		t.Fatalf("%+v", out)
	}
}
