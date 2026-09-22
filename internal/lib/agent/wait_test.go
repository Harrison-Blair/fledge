package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestWaitFromEnvironmentTransportLimit(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SOCKET_PATH", "/run/herdr.sock")
	if client, ok := WaitFromEnvironment(0).API.(herdr.Client); !ok || !client.NoDeadline || client.Socket != "/run/herdr.sock" {
		t.Fatalf("indefinite wait kept a transport deadline: %+v", client)
	}
	if client, ok := WaitFromEnvironment(2 * time.Second).API.(herdr.Client); !ok || client.NoDeadline || client.Timeout != 17*time.Second {
		t.Fatalf("finite wait transport limit: %+v", client)
	}
}

func TestWaitSendsParamsAndDecodes(t *testing.T) {
	for _, tc := range []struct {
		until   []string
		timeout time.Duration
		want    map[string]any
		status  string
	}{
		{nil, 0, map[string]any{"target": "worker"}, "blocked"},
		{[]string{"working"}, 1500 * time.Millisecond, map[string]any{"target": "worker", "until": []string{"working"}, "timeout_ms": int64(1500)}, "working"},
	} {
		c := Client{API: apiFunc(func(method string, params any) (any, error) {
			if method != "agent.wait" || !reflect.DeepEqual(normalize(params), normalize(tc.want)) {
				t.Fatalf("%s %#v", method, params)
			}
			return herdr.AgentResult{Type: "agent_info", Agent: agentInfo(tc.status)}, nil
		})}
		a, err := c.Wait(context.Background(), "worker", tc.until, tc.timeout)
		if err != nil || a.AgentStatus != tc.status {
			t.Fatalf("%v %+v", err, a)
		}
	}
}

func TestWaitRejectsUnmatchedOrIncompleteResults(t *testing.T) {
	for _, tc := range []struct {
		until  []string
		result any
	}{
		{nil, herdr.AgentResult{Type: "wait_matched", Agent: agentInfo("idle")}},
		{nil, herdr.AgentResult{Type: "agent_info", Agent: herdr.AgentDetails{Pane: agentInfo("idle").Pane}}},
		{nil, herdr.AgentResult{Type: "agent_info", Agent: agentInfo("working")}},
		{[]string{"done"}, herdr.AgentResult{Type: "agent_info", Agent: agentInfo("idle")}},
	} {
		c := Client{API: apiFunc(func(string, any) (any, error) { return tc.result, nil })}
		_, err := c.Wait(context.Background(), "worker", tc.until, 0)
		var remote *herdr.Error
		if !errors.As(err, &remote) || remote.Code != "protocol_error" {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}
