package cmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSendCLI(t *testing.T) {
	l := newSocket(t)
	done := serveRPCs(l, waitedResult(), map[string]any{"type": "ok"})
	var b bytes.Buffer
	args := []string{"agent", "send", "--name", "worker", "--text", "/model x", "--key", "down", "--key", "enter"}
	if err := ExecuteWithArgs(args, &b); err != nil {
		t.Fatalf("%v: %s", err, b.String())
	}
	calls := waitCalls(t, l, done, 2)
	if calls[0].Method != "agent.get" || calls[1].Method != "pane.send_input" {
		t.Fatalf("%+v", calls)
	}
	var params map[string]any
	if err := json.Unmarshal(calls[1].Params, &params); err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"pane_id": "w1:p1", "text": "/model x", "keys": []any{"down", "enter"}}; !reflect.DeepEqual(params, want) {
		t.Fatalf("%#v", params)
	}
	if got := b.String(); got != "Sent input to - (claude) in w1:p1; it was idle before sending.\n" {
		t.Fatalf("%q", got)
	}
}

// Key values are never split on commas.
func TestSendCLIKeyKeepsCommas(t *testing.T) {
	l := newSocket(t)
	done := serveRPCs(l, waitedResult(), map[string]any{"type": "ok"})
	var b bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "send", "--name", "worker", "--key", "a,b"}, &b); err != nil {
		t.Fatalf("%v: %s", err, b.String())
	}
	calls := waitCalls(t, l, done, 2)
	if got := paramsField(t, calls[1], "keys"); !reflect.DeepEqual(got, []any{"a,b"}) {
		t.Fatalf("%#v", got)
	}
}

func TestSendCLIValidation(t *testing.T) {
	for _, flags := range [][]string{{"--name", "a"}, {"--name", "a", "--text", ""}, {"--key", "enter"}, {"--name", "a", "--pane", "p", "--key", "enter"}, {"--name", "a", "--key", "enter", "extra"}} {
		var b bytes.Buffer
		args := append([]string{"agent", "send", "--json"}, flags...)
		if err := ExecuteWithArgs(args, &b); err == nil {
			t.Fatalf("accepted %v", args)
		}
		if !strings.Contains(b.String(), `"operation":"agent.send"`) || !strings.Contains(b.String(), `"status":"rejected"`) {
			t.Fatalf("%v: %s", args, b.String())
		}
	}
}
