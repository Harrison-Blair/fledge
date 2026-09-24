package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func stopAgent(status string) any {
	return map[string]any{"type": "agent_info", "agent": map[string]any{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1", "name": "worker", "agent": "claude", "agent_status": status, "terminal_id": "term_x", "focused": false, "revision": 0}}
}

// A worker that reports and then finishes its turn within the grace is
// stopped without --force.
func TestStopWorkingAgentSettlingWithinGraceExitsZero(t *testing.T) {
	l := newSocket(t)
	done := serveRPCs(l, stopAgent("working"), stopAgent("done"), map[string]any{"type": "ok"})
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "stop", "--name", "worker"}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 3)
	if calls[1].Method != "agent.wait" || paramsField(t, calls[1], "timeout_ms") != float64(5000) || calls[2].Method != "pane.close" {
		t.Fatalf("%+v", calls)
	}
	if out.String() != "Stopped worker (claude) in w1:p1.\n" {
		t.Fatalf("%q", out.String())
	}
}

func TestStopGraceFlagSetsSettleWait(t *testing.T) {
	l := newSocket(t)
	done := serveRPCs(l, stopAgent("working"), stopAgent("idle"), map[string]any{"type": "ok"})
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "stop", "--name", "worker", "--grace", "2s"}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	if calls := waitCalls(t, l, done, 3); paramsField(t, calls[1], "timeout_ms") != float64(2000) {
		t.Fatalf("%+v", calls)
	}
}

// --grace 0 restores the immediate refusal: no agent.wait is sent.
func TestStopGraceZeroRefusesWithoutWaiting(t *testing.T) {
	l := newSocket(t)
	// A second reply lets a stray agent.wait be served and counted.
	done := serveRPCs(l, stopAgent("working"), stopAgent("idle"))
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "stop", "--name", "worker", "--grace", "0", "--json"}, &out)
	waitCalls(t, l, done, 1)
	var status interface{ ExitCode() int }
	if !errors.As(err, &status) || status.ExitCode() != 2 || !strings.Contains(out.String(), `"phase":"guard"`) {
		t.Fatalf("%v %s", err, out.String())
	}
}

func TestStopInvalidGraceIsRejectedBeforeHerdr(t *testing.T) {
	for _, args := range [][]string{{"--grace", "-1s"}, {"--grace", "61s"}, {"--grace", "1s", "--force"}, {"--grace", "0", "--force"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			l := newSocket(t)
			done := serveRPCs(l)
			var out bytes.Buffer
			err := ExecuteWithArgs(append([]string{"agent", "stop", "--name", "worker", "--json"}, args...), &out)
			waitCalls(t, l, done, 0)
			var status interface{ ExitCode() int }
			if !errors.As(err, &status) || status.ExitCode() != 2 || !strings.Contains(out.String(), `"phase":"validation"`) || !strings.Contains(out.String(), "--grace") {
				t.Fatalf("%v %s", err, out.String())
			}
		})
	}
}

// Repeated --name flags stop each agent in turn; one refusal makes the
// outcome partial with exit status 1.
func TestStopSeveralNamesExitsPartial(t *testing.T) {
	l := newSocket(t)
	done := serveRPCs(l, stopAgent("idle"), map[string]any{"type": "ok"}, stopAgent("blocked"))
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "stop", "--name", "a", "--name", "b"}, &out)
	calls := waitCalls(t, l, done, 3)
	var status interface{ ExitCode() int }
	if !errors.As(err, &status) || status.ExitCode() != 1 || calls[1].Method != "pane.close" || calls[2].Method != "agent.get" || paramsField(t, calls[2], "target") != "b" {
		t.Fatalf("%v %+v", err, calls)
	}
	for _, want := range []string{"Stopped 1 of 2 agents.", "  stopped  a (w1:p1)", "  refused  b (w1:p1): agent b is blocked"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}

// --dry-run only looks targets up and exits zero when every target resolves.
func TestStopDryRunOnlyLooksUp(t *testing.T) {
	l := newSocket(t)
	// A spare reply lets a stray mutating call be served and counted.
	done := serveRPCs(l, stopAgent("idle"), stopAgent("working"), map[string]any{"type": "ok"})
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "stop", "--name", "a", "--pane", "w1:p1", "--dry-run"}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 2)
	if calls[0].Method != "agent.get" || calls[1].Method != "agent.get" {
		t.Fatalf("%+v", calls)
	}
	if !strings.HasPrefix(out.String(), "Dry run: would stop 1 of 2 agents.\n  stop    a (w1:p1, idle)\n  refuse  w1:p1 (w1:p1, working): agent w1:p1 is working") {
		t.Fatalf("%q", out.String())
	}
}
