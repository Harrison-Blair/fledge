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
)

type call = herdrscript.Call

func TestListIncludesUnnamed(t *testing.T) {
	p := herdrscript.Pane("w1:p1", "w1", "w1:t1")
	p.AgentStatus = "idle"
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "agent.list", Result: herdr.AgentListResult{Type: "agent_list", Agents: []herdr.Pane{p}}}))
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
	for _, tc := range []struct {
		name   string
		result any
		want   string
	}{
		{"empty list", Result{}, "No live agents."},
		{"list", Result{Agents: []libagent.AgentRow{row, {}}}, "NAME HARNESS STATUS WORKSPACE TAB PANE CWD worker claude idle w1 w1:t1 w1:p1 /repo - - - - - - -"},
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
		libagent.Outcome{Result: Result{Agents: []libagent.AgentRow{{Name: &name}}}},
	)
}
