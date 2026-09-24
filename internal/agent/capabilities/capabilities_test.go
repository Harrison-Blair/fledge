package capabilities

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/harness"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

type call = herdrscript.Call

// failingAPI fails the test on any socket call.
type failingAPI struct{ t *testing.T }

func (f failingAPI) Call(_ context.Context, method string, _ any, _ any) error {
	f.t.Fatalf("unexpected Herdr call %s", method)
	return errors.New("unreachable")
}

func offline(t *testing.T) libagent.Client { return libagent.Client{API: failingAPI{t}} }

func TestAllKindsInOrderWithoutSocket(t *testing.T) {
	out := Run(context.Background(), offline(t), Options{})
	if out.Status != "success" || out.Error != nil || out.Operation != "agent.capabilities" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
	r := out.Result.(Result)
	var kinds []string
	for _, h := range r.Harnesses {
		kinds = append(kinds, h.Kind)
		if !reflect.DeepEqual(h.Capabilities, harness.Capabilities(h.Kind)) || h.Live != nil {
			t.Errorf("%s: %+v", h.Kind, h)
		}
	}
	if !reflect.DeepEqual(kinds, harness.Kinds()) {
		t.Fatalf("kinds %q", kinds)
	}
}

func TestHarnessFilterAndValidation(t *testing.T) {
	out := Run(context.Background(), offline(t), Options{Harness: "claude"})
	r := out.Result.(Result)
	if len(r.Harnesses) != 1 || r.Harnesses[0].Kind != "claude" {
		t.Fatalf("%+v", r)
	}
	out = Run(context.Background(), offline(t), Options{Harness: "nope", Live: true})
	if out.Status != "rejected" || out.ExitCode() != 2 || out.Error.Phase != "validation" || out.Error.Message != "--harness must be a documented Herdr harness kind" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func TestLiveAddsIntegrationFacts(t *testing.T) {
	list := herdr.IntegrationListResult{Type: "integration_list", Integrations: []herdr.IntegrationInfo{
		{Target: "claude", Available: true, State: "current"},
		{Target: "codex", Available: false, State: "not_installed"},
	}}
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "integration.list", Result: list}), Options{Live: true})
	if out.Status != "success" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	live := map[string]*Live{}
	for _, h := range out.Result.(Result).Harnesses {
		live[h.Kind] = h.Live
	}
	if *live["claude"] != (Live{Available: true, HookState: "current"}) || *live["codex"] != (Live{HookState: "not_installed"}) || live["gemini"] != nil {
		t.Fatalf("%+v %+v %+v", live["claude"], live["codex"], live["gemini"])
	}
}

func TestLiveFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    call
		code string
	}{
		{"socket", call{Method: "integration.list", Err: errors.New("offline")}, "operation_failed"},
		{"protocol", call{Method: "integration.list", Result: map[string]any{"type": "ok"}}, "protocol_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := Run(context.Background(), herdrscript.Client(t, tc.c), Options{Live: true})
			if out.Status != "rejected" || out.Result != nil || out.ExitCode() != 1 || out.Error.Phase != "integration.list" || out.Error.Code != tc.code {
				t.Fatalf("%+v %+v", out, out.Error)
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	list := herdr.IntegrationListResult{Type: "integration_list", Integrations: []herdr.IntegrationInfo{{Target: "pi", Available: true, State: "outdated"}}}
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "integration.list", Result: list}), Options{Harness: "pi", Live: true})
	var b bytes.Buffer
	if err := out.Write(&b, true, Render); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Result Result `json:"result"`
	}
	if err := json.Unmarshal(b.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Result.Harnesses, out.Result.(Result).Harnesses) {
		t.Fatalf("%+v\n%+v", decoded.Result, out.Result)
	}
	for _, key := range []string{`"harnesses":[{"kind":"pi","capabilities":[{"name":"interrupt","level":"fledge","evidence":"esc"}`, `"live":{"available":true,"hook_state":"outdated"}`} {
		if !strings.Contains(b.String(), key) {
			t.Errorf("missing %s in %s", key, b.String())
		}
	}
	b.Reset()
	if err := Run(context.Background(), offline(t), Options{Harness: "pi"}).Write(&b, true, Render); err != nil || !strings.Contains(b.String(), `"live":null`) {
		t.Fatalf("%v %s", err, b.String())
	}
}

func TestHumanTable(t *testing.T) {
	var b bytes.Buffer
	if err := Run(context.Background(), offline(t), Options{Harness: "cursor"}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if len(lines) != 9 || strings.Join(strings.Fields(lines[0]), " ") != "HARNESS CAPABILITY LEVEL EVIDENCE" ||
		strings.Join(strings.Fields(lines[2]), " ") != "cursor model_select fledge spawn passes --model" {
		t.Fatalf("%q", lines)
	}
	list := herdr.IntegrationListResult{Type: "integration_list", Integrations: []herdr.IntegrationInfo{{Target: "claude", Available: true, State: "current"}}}
	b.Reset()
	if err := Run(context.Background(), herdrscript.Client(t, call{Method: "integration.list", Result: list}), Options{Live: true}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"HARNESS AVAILABLE HOOK_STATE", "claude yes current", "gemini - -"} {
		if !strings.Contains(squash(b.String()), want) {
			t.Errorf("missing %q in\n%s", want, b.String())
		}
	}
}

func squash(s string) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		lines = append(lines, strings.Join(strings.Fields(l), " "))
	}
	return strings.Join(lines, "\n")
}

func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render,
		libagent.Outcome{Result: Result{Harnesses: []Harness{{Kind: "pi", Capabilities: harness.Capabilities("pi")}}}},
		libagent.Outcome{Result: Result{live: true, Harnesses: []Harness{{Kind: "pi", Capabilities: harness.Capabilities("pi")}}}},
	)
}
