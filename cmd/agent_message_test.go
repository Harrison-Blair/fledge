package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestMessageConfirmStalledExitsPartial answers a --confirm agent.prompt with
// agent_prompt_stalled: the message was typed, so the CLI must report a
// partial submission with exit status 1 and never send it again.
func TestMessageConfirmStalledExitsPartial(t *testing.T) {
	l := newSocket(t)
	t.Setenv("HERDR_PANE_ID", "")
	done := make(chan []rpcCall, 1)
	go func() {
		var calls []rpcCall
		for _, reply := range []map[string]any{
			{"result": waitedResult()},
			{"error": map[string]any{"code": "agent_prompt_stalled", "message": "no activity observed"}},
		} {
			conn, err := l.Accept()
			if err != nil {
				break
			}
			var req struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			json.NewDecoder(conn).Decode(&req)
			calls = append(calls, rpcCall{Method: req.Method, Params: req.Params})
			reply["id"] = "fledge"
			json.NewEncoder(conn).Encode(reply)
			conn.Close()
		}
		l.Close()
		done <- calls
	}()
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "message", "--name", "worker", "--body", "hi", "--confirm", "--timeout", "3s"}, &out)
	var exit interface{ ExitCode() int }
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("%v\n%s", err, out.String())
	}
	calls := waitCalls(t, l, done, 2)
	if calls[1].Method != "agent.prompt" || !reflect.DeepEqual(paramsField(t, calls[1], "wait"), map[string]any{"until": []any{"working", "done", "idle", "blocked"}, "timeout_ms": 3000.0}) {
		t.Fatalf("%+v", calls[1])
	}
	for _, want := range []string{"partial: agent_prompt_stalled: no activity observed (agent.prompt)", "submitted message w1:p1", "do not resend it"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}

// TestMessageFanOutDeliversToEachTarget messages two named agents: each is
// resolved first, then prompted in flag order with the same text, and the
// JSON result carries one row per target.
func TestMessageFanOutDeliversToEachTarget(t *testing.T) {
	l := newSocket(t)
	t.Setenv("HERDR_PANE_ID", "")
	done := make(chan []rpcCall, 1)
	go func() {
		var calls []rpcCall
		for i := range 4 {
			result := waitedResult().(map[string]any)
			if i >= 2 {
				result["type"] = "agent_prompted"
			}
			conn, err := l.Accept()
			if err != nil {
				break
			}
			var req struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			json.NewDecoder(conn).Decode(&req)
			calls = append(calls, rpcCall{Method: req.Method, Params: req.Params})
			json.NewEncoder(conn).Encode(map[string]any{"id": "fledge", "result": result})
			conn.Close()
		}
		l.Close()
		done <- calls
	}()
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "message", "--name", "a", "--name", "b", "--body", "hi", "--json"}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	calls := waitCalls(t, l, done, 4)
	var methods, targets []string
	for _, c := range calls {
		methods = append(methods, c.Method)
		targets = append(targets, paramsField(t, c, "target").(string))
	}
	if strings.Join(methods, " ") != "agent.get agent.get agent.prompt agent.prompt" || strings.Join(targets, " ") != "a b a b" ||
		paramsField(t, calls[2], "text") != paramsField(t, calls[3], "text") {
		t.Fatalf("%+v", calls)
	}
	var envelope struct {
		Status string `json:"status"`
		Result struct {
			Mode    string `json:"mode"`
			Targets []struct {
				Target, Outcome string
			} `json:"targets"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Status != "success" || envelope.Result.Mode != "fan-out" || len(envelope.Result.Targets) != 2 ||
		envelope.Result.Targets[1].Target != "b" || envelope.Result.Targets[1].Outcome != "submitted" {
		t.Fatalf("%v %s", err, out.String())
	}
}
