package herdrscript

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestListServesAgentListWithEmptySlice(t *testing.T) {
	c := Client(t, List(), List(herdr.AgentDetails{Pane: LiveAgent("idle")}))
	for _, want := range []string{
		`{"type":"agent_list","agents":[]}`,
		`{"type":"agent_list","agents":[` + mustJSON(t, herdr.AgentDetails{Pane: LiveAgent("idle")}) + `]}`,
	} {
		var got json.RawMessage
		if err := c.API.Call(context.Background(), "agent.list", nil, &got); err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("got %s want %s", got, want)
		}
	}
}

func TestGetServesAgentInfoForTarget(t *testing.T) {
	a := herdr.AgentDetails{Pane: LiveAgent("idle"), TerminalID: "term_a"}
	call := Get("worker", a)
	if call.Method != "agent.get" || !reflect.DeepEqual(call.Params, map[string]any{"target": "worker"}) {
		t.Fatalf("%+v", call)
	}
	var got herdr.AgentResult
	if err := Client(t, call).API.Call(context.Background(), "agent.get", map[string]any{"target": "worker"}, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, herdr.AgentResult{Type: "agent_info", Agent: a}) {
		t.Fatalf("%+v", got)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
