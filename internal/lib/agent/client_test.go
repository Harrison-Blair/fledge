package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// apiFunc serves one scripted response per call through JSON, like the socket.
type apiFunc func(method string, params any) (any, error)

func (f apiFunc) Call(_ context.Context, method string, params, result any) error {
	response, err := f(method, params)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(response)
	return json.Unmarshal(data, result)
}

func ptr[T any](v T) *T { return &v }

func agentInfo(status string) herdr.AgentDetails {
	return herdr.AgentDetails{Pane: herdr.Pane{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1", AgentStatus: status, Agent: ptr("claude")}, TerminalID: "term", Focused: ptr(false), Revision: ptr(uint64(1))}
}

func TestFromEnvironmentRequiresHerdr(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_SOCKET_PATH", "/run/herdr.sock")
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	c := FromEnvironment(0)
	c.API = apiFunc(func(string, any) (any, error) { t.Fatal("called Herdr outside Herdr"); return nil, nil })
	err := c.Call(context.Background(), "agent.list", nil, &struct{}{})
	if err == nil || err.Error() != "run inside Herdr with HERDR_ENV=1 and HERDR_SOCKET_PATH set" || c.CallerPane != "w1:p1" {
		t.Fatalf("%v %+v", err, c)
	}
	t.Setenv("HERDR_ENV", "1")
	c = FromEnvironment(0)
	if client, ok := c.API.(herdr.Client); !ok || client.Socket != "/run/herdr.sock" || client.Timeout != 15e9 || c.Cwd == "" {
		t.Fatalf("%+v", c)
	}
}

func TestCallLocatesTransportFailures(t *testing.T) {
	remote := &herdr.Error{Code: "agent_not_found", Message: "missing"}
	c := Client{API: apiFunc(func(string, any) (any, error) { return nil, remote })}
	err := c.Call(context.Background(), "tab.create", nil, &struct{}{})
	o := Outcome{}
	o.Fail(err, "placement", false)
	if !errors.Is(err, remote) || o.Error.Phase != "tab.create" || o.Error.Code != "agent_not_found" {
		t.Fatalf("%v %+v", err, o.Error)
	}
}

func TestAtPhaseLocatesResultFailures(t *testing.T) {
	cause := Protocol("incomplete worktree.list result")
	err := AtPhase("worktree.list", cause)
	o := Outcome{}
	o.Fail(err, "placement", true)
	if !errors.Is(err, cause) || err.Error() != cause.Error() || *o.Error != (Failure{Code: "protocol_error", Message: cause.Error(), Phase: "worktree.list"}) || o.Status != "unknown" {
		t.Fatalf("%v %s %+v", err, o.Status, o.Error)
	}
}

func TestGetValidatesAgentInfo(t *testing.T) {
	session := &herdr.AgentSession{Source: ptr("hook"), Agent: ptr("claude"), Kind: ptr("id"), Value: ptr("abc")}
	good := agentInfo("idle")
	withSession := agentInfo("working")
	withSession.AgentSession = session
	badSession := agentInfo("idle")
	badSession.AgentSession = &herdr.AgentSession{Source: ptr("hook"), Agent: ptr("claude"), Kind: ptr("uuid"), Value: ptr("abc")}
	noTerminal := agentInfo("idle")
	noTerminal.TerminalID = ""
	for _, tc := range []struct {
		name     string
		response any
		ok       bool
	}{
		{"valid", herdr.AgentResult{Type: "agent_info", Agent: good}, true},
		{"valid session", herdr.AgentResult{Type: "agent_info", Agent: withSession}, true},
		{"wrong type", herdr.AgentResult{Type: "agent_prompted", Agent: good}, false},
		{"bad status", herdr.AgentResult{Type: "agent_info", Agent: agentInfo("sleeping")}, false},
		{"bad session", herdr.AgentResult{Type: "agent_info", Agent: badSession}, false},
		{"no terminal", herdr.AgentResult{Type: "agent_info", Agent: noTerminal}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Client{API: apiFunc(func(method string, params any) (any, error) {
				if method != "agent.get" || !reflect.DeepEqual(params, map[string]any{"target": "worker"}) {
					t.Fatalf("%s %#v", method, params)
				}
				return tc.response, nil
			})}
			a, err := c.Get(context.Background(), "worker")
			if tc.ok != (err == nil) {
				t.Fatalf("%v", err)
			}
			var remote *herdr.Error
			if !tc.ok && (!errors.As(err, &remote) || remote.Code != "protocol_error" || !remote.Uncertain || err.Error() != "protocol_error: incomplete agent.get result") {
				t.Fatalf("%v", err)
			}
			if tc.ok && a.PaneID != "w1:p2" {
				t.Fatalf("%+v", a)
			}
		})
	}
}

func TestPromptValidatesAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response herdr.AgentResult
		ok       bool
	}{
		{"valid", herdr.AgentResult{Type: "agent_prompted", Agent: agentInfo("working")}, true},
		{"wrong type", herdr.AgentResult{Type: "agent_info", Agent: agentInfo("working")}, false},
		{"missing pane", herdr.AgentResult{Type: "agent_prompted", Agent: herdr.AgentDetails{Pane: herdr.Pane{AgentStatus: "idle"}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Client{API: apiFunc(func(method string, params any) (any, error) {
				if method != "agent.prompt" || !reflect.DeepEqual(params, map[string]any{"target": "worker", "text": "hi"}) {
					t.Fatalf("%s %#v", method, params)
				}
				return tc.response, nil
			})}
			a, err := c.Prompt(context.Background(), "worker", "hi")
			if tc.ok != (err == nil) || tc.ok && a.PaneID != "w1:p2" || !tc.ok && a.PaneID != "" {
				t.Fatalf("%+v %v", a, err)
			}
			if !tc.ok && err.Error() != "protocol_error: incomplete agent.prompt result" {
				t.Fatal(err)
			}
		})
	}
	sent := errors.New("socket closed")
	_, err := Client{API: apiFunc(func(string, any) (any, error) { return nil, sent })}.Prompt(context.Background(), "worker", "hi")
	if !errors.Is(err, sent) {
		t.Fatal(err)
	}
}

func TestResolveTarget(t *testing.T) {
	for _, c := range []struct {
		name, pane, want string
		ok               bool
	}{{"worker", "", "worker", true}, {"", "w1:p3", "w1:p3", true}, {"", "", "", false}, {"worker", "w1:p3", "", false}} {
		got, err := ResolveTarget(c.name, c.pane)
		if got != c.want || (err == nil) != c.ok {
			t.Fatalf("%+v: got %q, %v", c, got, err)
		}
		var input *InputError
		if !c.ok && (err.Error() != "exactly one of --name or --pane is required" || !errors.As(err, &input)) {
			t.Fatalf("%+v: %v", c, err)
		}
	}
}

func TestValidPaneRequiresOwnership(t *testing.T) {
	for _, p := range []herdr.Pane{{WorkspaceID: "w1", TabID: "t"}, {PaneID: "p", TabID: "t"}, {PaneID: "p", WorkspaceID: "w1"}} {
		if ValidPane(p) || ValidAgent(p) {
			t.Fatalf("accepted %+v", p)
		}
	}
	for _, status := range []string{"idle", "working", "blocked", "done", "unknown"} {
		if !ValidAgent(herdr.Pane{PaneID: "p", WorkspaceID: "w", TabID: "t", AgentStatus: status}) {
			t.Fatal(status)
		}
	}
}
