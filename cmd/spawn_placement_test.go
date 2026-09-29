package cmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Retired placement flags are unknown flags: rejected before any Herdr call.
func TestSpawnRetiredPlacementFlagsFailBeforeSocket(t *testing.T) {
	for _, flag := range [][]string{{"--tab-id", "w1:t1"}, {"--direction", "down"}, {"--ratio", "0.5"}} {
		t.Run(flag[0], func(t *testing.T) {
			l := newSocket(t)
			done := serveRPCs(l)
			var out bytes.Buffer
			err := ExecuteWithArgs(append([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--workspace", "main"}, flag...), &out)
			if err == nil || !strings.Contains(err.Error(), "unknown flag: "+flag[0]) {
				t.Fatalf("%v %s", err, out.String())
			}
			waitCalls(t, l, done, 0)
		})
	}
}

// The same tokens after -- are native arguments, forwarded exactly.
func TestSpawnNativeTokensMatchingRetiredFlagsPassThrough(t *testing.T) {
	native := []string{"--tab-id", "w1:t1", "--direction", "down", "--ratio", "0.5"}
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), labeledResult(), startedResult(native...), readyAs("claude"))
	var out bytes.Buffer
	if err := ExecuteWithArgs(append([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "w1:p1", "--json", "--"}, native...), &out); err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 4)
	var params struct{ Args []string }
	if err := json.Unmarshal(calls[2].Params, &params); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(params.Args, native) {
		t.Fatalf("%q", params.Args)
	}
	var envelope struct{ Result map[string]any }
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err, out.String())
	}
	if _, ok := envelope.Result["split"]; ok || envelope.Result["pane_id"] != "w1:p1" || envelope.Result["tab_id"] == nil || envelope.Result["workspace_id"] == nil {
		t.Fatal(out.String())
	}
}
