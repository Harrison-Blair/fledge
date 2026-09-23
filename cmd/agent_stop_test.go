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
