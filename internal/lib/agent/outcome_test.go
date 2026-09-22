package agent

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestFailClassifiesStatusCodeAndPhase(t *testing.T) {
	remote := &herdr.Error{Code: "agent_not_found", Message: "missing"}
	uncertain := &herdr.Error{Code: "transport_error", Message: "lost", Uncertain: true}
	for _, tc := range []struct {
		name                string
		effects             []Effect
		err                 error
		phase               string
		mutating            bool
		status, code, where string
		exit                int
	}{
		{"input", nil, Invalid("bad %s", "flag"), "validation", false, "rejected", "invalid_input", "validation", 2},
		{"plain", nil, errors.New("boom"), "placement", false, "rejected", "operation_failed", "placement", 1},
		{"remote", nil, remote, "agent.get", false, "rejected", "agent_not_found", "agent.get", 1},
		{"uncertain mutation", nil, uncertain, "pane.close", true, "unknown", "transport_error", "pane.close", 1},
		{"uncertain read", nil, uncertain, "agent.get", false, "rejected", "transport_error", "agent.get", 1},
		{"start timeout", nil, &herdr.Error{Code: "timeout", Message: "slow"}, "agent.start", false, "partial", "timeout", "agent.start", 1},
		{"start not ready", nil, &herdr.Error{Code: "agent_not_ready", Message: "slow"}, "agent.start", false, "partial", "agent_not_ready", "agent.start", 1},
		{"reused only", []Effect{{Action: "reused", Kind: "worktree"}}, errors.New("boom"), "placement", false, "rejected", "operation_failed", "placement", 1},
		{"prior effect", []Effect{{Action: "created", Kind: "tab"}}, errors.New("boom"), "placement", false, "partial", "operation_failed", "placement", 1},
		{"located phase", nil, &phaseError{phase: "tab.create", cause: remote}, "placement", false, "rejected", "agent_not_found", "tab.create", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Outcome{Status: "success", Effects: tc.effects}
			o.Fail(tc.err, tc.phase, tc.mutating)
			want := Failure{Code: tc.code, Message: tc.err.Error(), Phase: tc.where}
			if o.Status != tc.status || o.Error == nil || *o.Error != want || o.ExitCode() != tc.exit {
				t.Fatalf("got %s %+v exit=%d", o.Status, o.Error, o.ExitCode())
			}
		})
	}
	if (Outcome{}).ExitCode() != 0 {
		t.Fatal("success must exit 0")
	}
}

func TestInvalidOutcome(t *testing.T) {
	o := InvalidOutcome("agent.spawn", errors.New("unknown flag: --bad"))
	var b bytes.Buffer
	if err := o.Write(&b, true, nil); err != nil {
		t.Fatal(err)
	}
	want := `{"operation":"agent.spawn","status":"rejected","result":null,"effects":[],"error":{"code":"invalid_input","message":"unknown flag: --bad","phase":"validation"}}` + "\n"
	if b.String() != want || o.ExitCode() != 2 {
		t.Fatalf("got %s exit=%d", b.String(), o.ExitCode())
	}
}

func TestWriteJSONNeverRendersAndKeepsEmptySlices(t *testing.T) {
	render := func(io.Writer, Outcome) error { t.Fatal("JSON output called the human renderer"); return nil }
	var b bytes.Buffer
	if err := (Outcome{Operation: "agent.list", Status: "success", Effects: []Effect{}}).Write(&b, true, render); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"effects":[]`) || !strings.Contains(b.String(), `"error":null`) {
		t.Fatal(b.String())
	}
}

func TestWriteHumanRendersFailureThenLeafHint(t *testing.T) {
	o := Outcome{Status: "partial", Effects: []Effect{{Action: "created", Kind: "tab", ID: "w1:t2"}, {Action: "created", Kind: "worktree", Path: "/repo/.worktrees/a"}}, Error: &Failure{Code: "timeout", Message: "slow", Phase: "agent.start"}}
	calls := 0
	render := func(w io.Writer, got Outcome) error {
		calls++
		if got.Error == nil {
			t.Fatal("renderer lost the failure")
		}
		_, err := fmt.Fprintln(w, "hint")
		return err
	}
	var b bytes.Buffer
	if err := o.Write(&b, false, render); err != nil {
		t.Fatal(err)
	}
	want := "partial: slow (agent.start)\n  created tab w1:t2\n  created worktree /repo/.worktrees/a\nhint\n"
	if b.String() != want || calls != 1 {
		t.Fatalf("calls=%d got %q", calls, b.String())
	}
}

func TestWriteHumanSuccessDelegatesToRenderer(t *testing.T) {
	var b bytes.Buffer
	render := func(w io.Writer, o Outcome) error { _, err := fmt.Fprintf(w, "ok %s\n", o.Operation); return err }
	if err := (Outcome{Operation: "agent.list", Status: "success"}).Write(&b, false, render); err != nil || b.String() != "ok agent.list\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
	// Without a leaf renderer only generic failure lines are written.
	b.Reset()
	if err := (Outcome{Status: "success"}).Write(&b, false, nil); err != nil || b.Len() != 0 {
		t.Fatalf("%q %v", b.String(), err)
	}
	failed := Outcome{Status: "rejected", Error: &Failure{Message: "bad", Phase: "validation"}}
	if err := failed.Write(&b, false, nil); err != nil || b.String() != "rejected: bad (validation)\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestFinish(t *testing.T) {
	var b bytes.Buffer
	if err := Finish(Outcome{Status: "success"}, &b, true, nil); err != nil {
		t.Fatal(err)
	}
	failed := InvalidOutcome("agent.get", errors.New("bad"))
	err := Finish(failed, &b, false, nil)
	var result *ResultError
	if !errors.As(err, &result) || result.ExitCode() != 2 || err.Error() != "bad" || !cli.IsRendered(err) {
		t.Fatalf("%v", err)
	}
	for _, asJSON := range []bool{false, true} {
		err = Finish(failed, failingWriter{}, asJSON, nil)
		var output *cli.OutputError
		if !errors.As(err, &output) || output.ExitCode() != 1 || !cli.IsRendered(err) {
			t.Fatalf("json=%v: %v", asJSON, err)
		}
	}
	renderErr := errors.New("render failed")
	err = Finish(Outcome{Status: "success"}, &b, false, func(io.Writer, Outcome) error { return renderErr })
	if !errors.Is(err, renderErr) {
		t.Fatalf("renderer failure lost: %v", err)
	}
}

func TestNewAgentRow(t *testing.T) {
	name, kind, cwd := "worker", "claude", "/repo"
	row := NewAgentRow(herdr.Pane{Name: &name, Agent: &kind, AgentStatus: "idle", WorkspaceID: "w1", TabID: "w1:t1", PaneID: "w1:p1", Cwd: &cwd})
	if *row.Name != name || *row.Harness != kind || *row.AgentStatus != "idle" || *row.WorkspaceID != "w1" || *row.TabID != "w1:t1" || *row.PaneID != "w1:p1" || *row.Cwd != cwd {
		t.Fatalf("%+v", row)
	}
	if empty := NewAgentRow(herdr.Pane{}); empty.AgentStatus != nil || empty.PaneID != nil || empty.WorkspaceID != nil || empty.TabID != nil {
		t.Fatalf("empty values must be null: %+v", empty)
	}
}
