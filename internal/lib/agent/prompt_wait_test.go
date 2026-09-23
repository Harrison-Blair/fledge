package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestPromptConfirmSendsWaitAndDecodes(t *testing.T) {
	want := map[string]any{"target": "worker", "text": "hi", "wait": map[string]any{"until": []string{"working", "done", "idle", "blocked"}, "timeout_ms": int64(2500)}}
	for _, status := range []string{"working", "done", "idle", "blocked"} {
		c := Client{API: apiFunc(func(method string, params any) (any, error) {
			if method != "agent.prompt" || !reflect.DeepEqual(normalize(params), normalize(want)) {
				t.Fatalf("%s %#v", method, params)
			}
			return herdr.AgentResult{Type: "agent_prompted", Agent: agentInfo(status)}, nil
		})}
		a, err := c.PromptConfirm(context.Background(), "worker", "hi", 2500*time.Millisecond)
		if err != nil || a.AgentStatus != status {
			t.Fatalf("%v %+v", err, a)
		}
	}
}

func TestPromptConfirmRejectsUnmatchedOrIncompleteResults(t *testing.T) {
	for _, result := range []any{
		herdr.AgentResult{Type: "agent_info", Agent: agentInfo("working")},
		herdr.AgentResult{Type: "agent_prompted", Agent: agentInfo("unknown")},
		herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: herdr.Pane{PaneID: "w1:p2", AgentStatus: "working"}}},
	} {
		c := Client{API: apiFunc(func(string, any) (any, error) { return result, nil })}
		_, err := c.PromptConfirm(context.Background(), "worker", "hi", time.Second)
		var remote *herdr.Error
		if !errors.As(err, &remote) || remote.Code != "protocol_error" || !remote.Uncertain {
			t.Fatalf("%+v: %v", result, err)
		}
	}
}
