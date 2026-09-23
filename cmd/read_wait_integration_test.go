package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestReadCLITranslatesSourceAndLabelsSnapshot(t *testing.T) {
	l := newSocket(t)
	read := map[string]any{"type": "pane_read", "read": map[string]any{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1", "source": "recent_unwrapped", "format": "text", "text": "line one\nline two\n", "revision": 4, "truncated": true}}
	done := serveRPCs(l, waitedResult(), read)
	var b bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "read", "--pane", "w1:p1", "--lines", "2"}, &b); err != nil {
		t.Fatalf("%v: %s", err, b.String())
	}
	calls := waitCalls(t, l, done, 2)
	if calls[1].Method != "agent.read" || paramsField(t, calls[1], "source") != "recent_unwrapped" || paramsField(t, calls[1], "lines") != 2. || paramsField(t, calls[1], "format") != "text" {
		t.Fatalf("%+v", calls)
	}
	if want := "Terminal snapshot of w1:p1 (recent-unwrapped, 2 rows, truncated: yes)\nline one\nline two\n"; b.String() != want {
		t.Fatalf("%q", b.String())
	}
}

func TestWaitCLISingleAndFanOut(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		calls   int
		timeout any
	}{
		{[]string{"agent", "wait", "--name", "worker"}, 1, nil},
		{[]string{"agent", "wait", "--name", "worker", "--timeout", "2s", "--until", "idle", "--json"}, 1, 2000.},
		{[]string{"agent", "wait", "--name", "worker", "--pane", "w1:p2", "--all", "--json"}, 2, nil},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			l := newSocket(t)
			results := []any{waitedResult(), waitedResult()}[:tc.calls]
			done := serveRPCs(l, results...)
			var b bytes.Buffer
			if err := ExecuteWithArgs(tc.args, &b); err != nil {
				t.Fatalf("%v: %s", err, b.String())
			}
			for _, c := range waitCalls(t, l, done, tc.calls) {
				if c.Method != "agent.wait" || paramsField(t, c, "timeout_ms") != tc.timeout {
					t.Fatalf("%+v", c)
				}
			}
			if tc.calls == 1 && !strings.Contains(tc.args[len(tc.args)-1], "json") && b.String() != "w1:p1 is idle.\n" {
				t.Fatalf("%q", b.String())
			}
			if tc.calls == 2 {
				var out struct {
					Status string
					Result struct {
						Mode    string
						Targets []struct{ Target, Outcome string }
					}
				}
				if err := json.Unmarshal(b.Bytes(), &out); err != nil || out.Status != "success" || out.Result.Mode != "all" || len(out.Result.Targets) != 2 || out.Result.Targets[1].Outcome != "matched" {
					t.Fatalf("%v %s", err, b.String())
				}
			}
		})
	}
}
