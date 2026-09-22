package list

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

func TestListIncludesUnnamed(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.Pane{p}}}))
	if out.Status != "success" || len(out.Result.(Result).Agents) != 1 {
		t.Fatal(out)
	}
}
func TestRuntimeErrorExit(t *testing.T) {
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "agent.list", Err: errors.New("offline")}))
	if out.ExitCode() != 1 {
		t.Fatal(out)
	}
}
func TestHumanOperationResults(t *testing.T) {
	row := herdrscript.Row()
	id := "0000beef"
	for _, tc := range []struct {
		name   string
		result any
		want   string
	}{
		{"empty list", Result{}, "No live agents."},
		{"list", Result{Agents: []Row{{ID: &id, AgentRow: row}, {}}}, "ID NAME HARNESS STATUS WORKSPACE TAB PANE CWD 0000beef worker claude idle w1 w1:t1 w1:p1 /repo - - - - - - - -"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := (libagent.Outcome{Status: "success", Result: tc.result}).Write(&b, false, Render); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(strings.Fields(b.String()), " "); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
func TestOutputFailuresPropagate(t *testing.T) {
	name := "worker"
	herdrscript.CheckOutputFailures(t, Render,
		libagent.Outcome{Result: Result{}},
		libagent.Outcome{Result: Result{Agents: []Row{{AgentRow: libagent.AgentRow{Name: &name}}}}},
	)
}

func TestListMatchesRecordsByTerminal(t *testing.T) {
	registered := herdrscript.Info(herdrscript.LiveAgent("idle")).Agent
	other := herdrscript.Info(herdrscript.Pane("w1:p4", "w1", "w1:t2")).Agent
	other.AgentStatus, other.TerminalID = "idle", "term_other"
	c := herdrscript.Client(t, call{Method: "agent.list", Result: map[string]any{"type": "agent_list", "agents": []herdr.AgentDetails{registered, other}}})
	c.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, c.Cwd, registered)
	out := Run(context.Background(), c)
	rows := out.Result.(Result).Agents
	if out.Error != nil || len(rows) != 2 || rows[0].ID == nil || *rows[0].ID != rec.ID || rows[1].ID != nil {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil || !strings.Contains(b.String(), `"id":"`+rec.ID+`"`) || !strings.Contains(b.String(), `"id":null`) {
		t.Fatalf("%s %v", b.String(), err)
	}
}
