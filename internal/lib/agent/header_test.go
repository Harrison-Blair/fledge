package agent

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestNewMessageIDIsSixLowercaseHex(t *testing.T) {
	re := regexp.MustCompile(`^m-[0-9a-f]{6}$`)
	if id := NewMessageID(); !re.MatchString(id) {
		t.Fatalf("%q", id)
	}
	if id := messageID(bytes.NewReader([]byte{0x0a, 0x1b, 0x2c, 0xff})); id != "m-0a1b2c" {
		t.Fatalf("%q", id)
	}
}

func TestResolveSender(t *testing.T) {
	named := agentInfo("working")
	named.Pane.Name = ptr("orchestrator")
	unnamed := agentInfo("idle")
	emptyName := agentInfo("idle")
	emptyName.Pane.Name = ptr("")
	for _, tc := range []struct {
		name, caller string
		response     any
		err          error
		want         Sender
	}{
		{"named agent", "w1:p2", herdr.AgentResult{Type: "agent_info", Agent: named}, nil, Sender{Name: ptr("orchestrator"), Pane: ptr("w1:p2"), Kind: "named"}},
		{"unnamed agent", "w1:p2", herdr.AgentResult{Type: "agent_info", Agent: unnamed}, nil, Sender{Pane: ptr("w1:p2"), Kind: "unnamed"}},
		{"empty name", "w1:p2", herdr.AgentResult{Type: "agent_info", Agent: emptyName}, nil, Sender{Pane: ptr("w1:p2"), Kind: "unnamed"}},
		{"not an agent", "w1:p9", nil, &herdr.Error{Code: "agent_not_found", Message: "no agent"}, Sender{Pane: ptr("w1:p9"), Kind: "pane"}},
		{"other error", "w1:p9", nil, &herdr.Error{Code: "timeout", Message: "slow"}, Sender{Pane: ptr("w1:p9"), Kind: "unknown", Error: ptr("timeout: slow")}},
		{"malformed result", "w1:p9", map[string]any{"type": "wrong"}, nil, Sender{Pane: ptr("w1:p9"), Kind: "unknown", Error: ptr("protocol_error: incomplete agent.get result")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Client{CallerPane: tc.caller, API: apiFunc(func(method string, params any) (any, error) {
				if method != "agent.get" || !reflect.DeepEqual(params, map[string]any{"target": tc.caller}) {
					t.Fatalf("%s %#v", method, params)
				}
				return tc.response, tc.err
			})}
			if got := ResolveSender(context.Background(), c); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveSenderWithoutCallerPaneSkipsHerdr(t *testing.T) {
	c := Client{API: apiFunc(func(string, any) (any, error) { return nil, errors.New("called Herdr") })}
	got := ResolveSender(context.Background(), c)
	if got.Kind != "unknown" || got.Pane != nil || got.Name != nil || got.Error == nil || *got.Error != "HERDR_PANE_ID is not set" {
		t.Fatalf("%+v", got)
	}
}

func TestWithHeader(t *testing.T) {
	for _, tc := range []struct {
		sender      Sender
		label, want string
	}{
		{Sender{Name: ptr("orchestrator"), Pane: ptr("wA:p1"), Kind: "named"}, "orchestrator (wA:p1)",
			"ᛉ fledge message from orchestrator (wA:p1) · id m-0a1b2c · reply: fledge agent message --name orchestrator\nhello\n"},
		{Sender{Pane: ptr("wA:p1"), Kind: "unnamed"}, "unnamed agent (wA:p1)",
			"ᛉ fledge message from unnamed agent (wA:p1) · id m-0a1b2c\nhello\n"},
		{Sender{Pane: ptr("wA:p1"), Kind: "pane"}, "pane wA:p1",
			"ᛉ fledge message from pane wA:p1 · id m-0a1b2c\nhello\n"},
		{Sender{Pane: ptr("wA:p1"), Kind: "unknown", Error: ptr("slow")}, "unknown sender",
			"ᛉ fledge message from unknown sender · id m-0a1b2c\nhello\n"},
	} {
		if got := WithHeader("m-0a1b2c", tc.sender, "hello\n"); got != tc.want {
			t.Errorf("got %q want %q", got, tc.want)
		}
		if got := tc.sender.String(); got != tc.label {
			t.Errorf("got %q want %q", got, tc.label)
		}
	}
}
