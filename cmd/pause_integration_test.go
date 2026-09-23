package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPauseCLI(t *testing.T) {
	for _, mode := range []string{"wait", "no-wait", "idle", "done", "json", "custom"} {
		t.Run(mode, func(t *testing.T) {
			l := newSocket(t)
			initial := waitedResult().(map[string]any)
			a := initial["agent"].(map[string]any)
			if mode == "done" {
				a["agent_status"] = "done"
			} else if mode != "idle" {
				a["agent_status"] = "working"
			}
			results := []any{initial}
			args := []string{"agent", "pause", "--name", "worker"}
			wantText := "Paused"
			if mode == "idle" || mode == "done" {
				wantText = "Already idle or done"
			} else {
				results = append(results, map[string]any{"type": "ok"})
				if mode == "no-wait" {
					args = append(args, "--no-wait")
					wantText = "Pause requested"
				} else {
					results = append(results, waitedResult())
				}
			}
			if mode == "json" {
				args = append(args, "--json")
			}
			if mode == "custom" {
				args = append(args, "--timeout", "3s")
			}
			done := serveRPCs(l, results...)
			var b bytes.Buffer
			if err := ExecuteWithArgs(args, &b); err != nil {
				t.Fatalf("%v: %s", err, b.String())
			}
			calls := waitCalls(t, l, done, len(results))
			for i, c := range calls {
				want := []string{"agent.get", "agent.send_keys", "agent.wait"}[i]
				if c.Method != want {
					t.Fatalf("%+v", calls)
				}
			}
			if len(calls) > 1 && paramsField(t, calls[1], "target") != "w1:p1" {
				t.Fatalf("%+v", calls)
			}
			if len(calls) == 3 {
				ms := paramsField(t, calls[2], "timeout_ms").(float64)
				max := 10000.
				if mode == "custom" {
					max = 3000
				}
				if ms <= max-1000 || ms > max {
					t.Fatalf("timeout %v", ms)
				}
			}
			if mode == "json" {
				var out struct {
					Operation, Status string
					Result            struct{ Submitted, Settled bool }
				}
				if err := json.Unmarshal(b.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				if out.Operation != "agent.pause" || out.Status != "success" || !out.Result.Submitted || !out.Result.Settled {
					t.Fatalf("%s", b.String())
				}
			} else if !strings.HasPrefix(b.String(), wantText) {
				t.Fatalf("%s", b.String())
			}
		})
	}
}
func TestPauseCLIValidation(t *testing.T) {
	for _, flags := range [][]string{{}, {"--name", "a", "--pane", "p"}, {"--name", ""}, {"--pane", " "}, {"--name", "a", "extra"}, {"--name", "a", "--timeout", "0s"}, {"--name", "a", "--timeout", "-1s"}, {"--name", "a", "--no-wait", "--timeout", "0s"}, {"--name", "a", "--timeout", "bad"}} {
		var b bytes.Buffer
		args := append([]string{"agent", "pause", "--json"}, flags...)
		if err := ExecuteWithArgs(args, &b); err == nil {
			t.Fatalf("accepted %v", args)
		}
		if !strings.Contains(b.String(), `"operation":"agent.pause"`) || !strings.Contains(b.String(), `"status":"rejected"`) {
			t.Fatalf("%v: %s", args, b.String())
		}
	}
}

func TestPauseCLIFailureEnvelopes(t *testing.T) {
	for _, mode := range []string{"send-malformed", "wait-malformed", "blocked", "changed-terminal"} {
		t.Run(mode, func(t *testing.T) {
			l := newSocket(t)
			initial := waitedResult().(map[string]any)
			initial["agent"].(map[string]any)["agent_status"] = "working"
			results := []any{initial}
			wantStatus, wantPhase := "partial", "agent.wait"
			if mode == "send-malformed" {
				results = append(results, map[string]any{"type": "unexpected"})
				wantStatus, wantPhase = "unknown", "agent.send_keys"
			} else {
				final := waitedResult().(map[string]any)
				switch mode {
				case "wait-malformed":
					final["type"] = "unexpected"
				case "blocked":
					final["agent"].(map[string]any)["agent_status"] = "blocked"
				case "changed-terminal":
					final["agent"].(map[string]any)["terminal_id"] = "replacement"
				}
				results = append(results, map[string]any{"type": "ok"}, final)
			}
			done := serveRPCs(l, results...)
			var b bytes.Buffer
			if err := ExecuteWithArgs([]string{"agent", "pause", "--pane", "w1:p1", "--json"}, &b); err == nil {
				t.Fatal("expected failure")
			}
			calls := waitCalls(t, l, done, len(results))
			for i, c := range calls {
				if c.Method != []string{"agent.get", "agent.send_keys", "agent.wait"}[i] {
					t.Fatalf("%+v", calls)
				}
			}
			var out struct {
				Operation, Status string
				Result            struct{ Submitted, Settled bool }
				Error             struct{ Phase string }
			}
			if err := json.Unmarshal(b.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Operation != "agent.pause" || out.Status != wantStatus || out.Error.Phase != wantPhase || out.Result.Settled || out.Result.Submitted != (mode != "send-malformed") {
				t.Fatalf("%s", b.String())
			}
		})
	}
}
