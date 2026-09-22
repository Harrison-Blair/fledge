package get

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

var fake = herdrscript.Client

func TestGetTargetsAndStates(t *testing.T) {
	for _, status := range []string{"idle", "working", "blocked", "done", "unknown"} {
		for _, byPane := range []bool{false, true} {
			p := herdrscript.LiveAgent(status)
			o, target := Options{Name: "worker"}, "worker"
			if byPane {
				p.Name = nil
				o = Options{Pane: p.PaneID}
				target = p.PaneID
			}
			out := Run(context.Background(), fake(t, call{Method: "agent.get", Params: map[string]any{"target": target}, Result: herdrscript.Info(p)}), o)
			if out.Operation != "agent.get" || out.Status != "success" || out.ExitCode() != 0 || out.Effects == nil || len(out.Effects) != 0 {
				t.Fatalf("%+v", out)
			}
			if !reflect.DeepEqual(out.Result.(Result).AgentRow, libagent.NewAgentRow(p)) {
				t.Fatalf("%+v", out.Result)
			}
		}
	}
}

func TestGetInvalidTargets(t *testing.T) {
	for _, o := range []Options{{}, {Name: "worker", Pane: "w1:p3"}} {
		out := Run(context.Background(), fake(t), o)
		if out.ExitCode() != 2 || out.Status != "rejected" || out.Error.Phase != "validation" {
			t.Fatalf("%+v", out)
		}
	}
}

func TestGetFailures(t *testing.T) {
	for _, code := range []string{"agent_not_found", "transport_error", "protocol_error"} {
		out := Run(context.Background(), fake(t, call{Method: "agent.get", Err: &herdr.Error{Code: code, Message: "failed", Uncertain: true}}), Options{Name: "worker"})
		if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != code || out.Error.Phase != "agent.get" || len(out.Effects) != 0 {
			t.Fatalf("%+v", out)
		}
	}
	for _, field := range []string{"type", "pane_id", "workspace_id", "tab_id", "agent_status", "terminal_id", "focused", "revision"} {
		p := map[string]any{"pane_id": "w1:p3", "workspace_id": "w1", "tab_id": "w1:t2", "agent_status": "idle", "terminal_id": "term_x", "focused": false, "revision": 0}
		r := map[string]any{"type": "agent_info", "agent": p}
		switch field {
		case "type":
			r[field] = "wrong"
		case "focused", "revision":
			delete(p, field)
		default:
			p[field] = ""
		}
		out := Run(context.Background(), fake(t, call{Method: "agent.get", Result: r}), Options{Name: "worker"})
		if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != "protocol_error" || out.Error.Phase != "agent.get" {
			t.Fatalf("%s: %+v", field, out)
		}
	}
}

func TestGetRejectsMalformedSession(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{"pane_id": "w1:p3", "workspace_id": "w1", "tab_id": "w1:t2", "agent_status": "idle", "terminal_id": "term_x", "focused": false, "revision": 0}
	}
	validSession := func() map[string]any {
		return map[string]any{"source": "s", "agent": "a", "kind": "id", "value": "v"}
	}
	rejects := []struct {
		name    string
		session map[string]any
	}{
		{"empty object", map[string]any{}},
		{"bad kind enum", map[string]any{"source": "s", "agent": "a", "kind": "url", "value": "v"}},
	}
	for _, field := range []string{"source", "agent", "kind", "value"} {
		omitted := validSession()
		delete(omitted, field)
		rejects = append(rejects, struct {
			name    string
			session map[string]any
		}{field + " omitted", omitted})
		null := validSession()
		null[field] = nil
		rejects = append(rejects, struct {
			name    string
			session map[string]any
		}{field + " null", null})
	}
	for _, tc := range rejects {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			p["agent_session"] = tc.session
			out := Run(context.Background(), fake(t, call{Method: "agent.get", Result: map[string]any{"type": "agent_info", "agent": p}}), Options{Name: "worker"})
			if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != "protocol_error" || out.Error.Phase != "agent.get" {
				t.Fatalf("%+v", out)
			}
		})
	}
	accepts := []struct {
		name    string
		session map[string]any
	}{
		{"kind id", validSession()},
		{"kind path", map[string]any{"source": "s", "agent": "a", "kind": "path", "value": "v"}},
		{"empty strings", map[string]any{"source": "", "agent": "", "kind": "id", "value": ""}},
	}
	for _, tc := range accepts {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			p["agent_session"] = tc.session
			out := Run(context.Background(), fake(t, call{Method: "agent.get", Result: map[string]any{"type": "agent_info", "agent": p}}), Options{Name: "worker"})
			if out.ExitCode() != 0 || out.Status != "success" {
				t.Fatalf("%+v", out)
			}
		})
	}
	t.Run("absent", func(t *testing.T) {
		out := Run(context.Background(), fake(t, call{Method: "agent.get", Result: map[string]any{"type": "agent_info", "agent": base()}}), Options{Name: "worker"})
		if out.ExitCode() != 0 || out.Status != "success" {
			t.Fatalf("%+v", out)
		}
	})
	t.Run("explicit null", func(t *testing.T) {
		p := base()
		p["agent_session"] = nil
		out := Run(context.Background(), fake(t, call{Method: "agent.get", Result: map[string]any{"type": "agent_info", "agent": p}}), Options{Name: "worker"})
		if out.ExitCode() != 0 || out.Status != "success" {
			t.Fatalf("%+v", out)
		}
	})
}

func TestGetResolvesTitle(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{"pane_id": "w1:p3", "workspace_id": "w1", "tab_id": "w1:t2", "agent_status": "idle", "terminal_id": "term_x", "focused": false, "revision": 0}
	}
	ptr := func(s string) *string { return &s }
	for _, tc := range []struct {
		name                                        string
		title, terminalTitle, terminalTitleStripped *string
		want                                        string
	}{
		{"stripped only", nil, nil, ptr("clean"), "clean"},
		{"raw only", nil, ptr("◐ raw"), nil, "◐ raw"},
		{"title wins", ptr("Review"), ptr("◐ raw"), ptr("clean"), "Review"},
		{"none present", nil, nil, nil, ""},
		{"stripped beats raw", nil, ptr("◐ raw"), ptr("clean"), "clean"},
		{"empty title falls through to stripped", ptr(""), nil, ptr("clean"), "clean"},
		{"empty stripped falls through to raw", nil, ptr("◐ raw"), ptr(""), "◐ raw"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			if tc.title != nil {
				p["title"] = *tc.title
			}
			if tc.terminalTitle != nil {
				p["terminal_title"] = *tc.terminalTitle
			}
			if tc.terminalTitleStripped != nil {
				p["terminal_title_stripped"] = *tc.terminalTitleStripped
			}
			out := Run(context.Background(), fake(t, call{Method: "agent.get", Result: map[string]any{"type": "agent_info", "agent": p}}), Options{Name: "worker"})
			if out.ExitCode() != 0 || out.Status != "success" {
				t.Fatalf("%+v", out)
			}
			got := out.Result.(Result).Title
			if tc.want == "" {
				if got != nil {
					t.Fatalf("title %q want nil", *got)
				}
			} else if got == nil || *got != tc.want {
				t.Fatalf("title %v want %q", got, tc.want)
			}
			var b bytes.Buffer
			if err := out.Write(&b, true, Render); err != nil {
				t.Fatal(err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(b.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			result := envelope["result"].(map[string]any)
			if tc.want == "" {
				if result["title"] != nil {
					t.Fatalf("json title %v want nil", result["title"])
				}
			} else if result["title"] != tc.want {
				t.Fatalf("json title %v want %q", result["title"], tc.want)
			}
		})
	}
}

func TestGetOutput(t *testing.T) {
	for _, detailed := range []bool{false, true} {
		p := map[string]any{"pane_id": "w1:p3", "workspace_id": "w1", "tab_id": "w1:t2", "agent_status": "idle", "terminal_id": "term_x", "focused": false, "revision": 0}
		want := "Name: -\nHarness: -\nStatus: idle\nWorkspace ID: w1\nTab ID: w1:t2\nPane ID: w1:p3\nWorking directory: -\nForeground working directory: -\nInteractive ready: -\nLaunch pending: -\nFocused: false\nTitle: -\nSession source: -\nSession harness: -\nSession reference kind: -\nSession reference value: -\n"
		if detailed {
			p["foreground_cwd"], p["interactive_ready"], p["launch_pending"], p["focused"], p["title"] = "/repo/sub", true, false, false, "Review"
			p["agent_session"] = map[string]any{"source": "herdr:claude", "agent": "claude", "kind": "id", "value": "session-1"}
			want = "Name: -\nHarness: -\nStatus: idle\nWorkspace ID: w1\nTab ID: w1:t2\nPane ID: w1:p3\nWorking directory: -\nForeground working directory: /repo/sub\nInteractive ready: true\nLaunch pending: false\nFocused: false\nTitle: Review\nSession source: herdr:claude\nSession harness: claude\nSession reference kind: id\nSession reference value: session-1\n"
		}
		out := Run(context.Background(), fake(t, call{Method: "agent.get", Result: map[string]any{"type": "agent_info", "agent": p}}), Options{Pane: "w1:p3"})
		var b bytes.Buffer
		if err := out.Write(&b, false, Render); err != nil {
			t.Fatal(err)
		}
		if b.String() != want {
			t.Fatalf("got %q want %q", b.String(), want)
		}
		b.Reset()
		if err := out.Write(&b, true, Render); err != nil {
			t.Fatal(err)
		}
		var envelope map[string]any
		if err := json.Unmarshal(b.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		result := envelope["result"].(map[string]any)
		if envelope["operation"] != "agent.get" || envelope["status"] != "success" || envelope["error"] != nil || len(envelope["effects"].([]any)) != 0 {
			t.Fatal(b.String())
		}
		for _, key := range []string{"foreground_cwd", "interactive_ready", "launch_pending", "title", "agent_session"} {
			value, ok := result[key]
			if !ok || (!detailed && value != nil) {
				t.Fatalf("%s: %s", key, b.String())
			}
		}
		if result["focused"] != false {
			t.Fatalf("focused: %s", b.String())
		}
		if detailed && (result["launch_pending"] != false || result["interactive_ready"] != true || result["agent_session"].(map[string]any)["harness"] != "claude") {
			t.Fatal(b.String())
		}
	}
}

func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: Result{}})
}

func TestGetByIDShowsRecord(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	c := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live})
	c.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, c.Cwd, live.Agent)
	out := Run(context.Background(), c, Options{ID: rec.ID})
	r, ok := out.Result.(Result)
	if out.Error != nil || !ok || r.Record == nil || !reflect.DeepEqual(*r.Record, rec) {
		t.Fatalf("%+v", out)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("Fledge ID: %s\nParent: -\nRegistered at: %s\nRegistered by: spawn\n", rec.ID, rec.RegisteredAt)
	if !strings.HasSuffix(b.String(), want) {
		t.Fatalf("%q", b.String())
	}
}

func TestGetByNameShowsLiveRecord(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	c := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "worker"}, Result: live})
	c.Cwd = identitytest.Repository(t)
	rec := identitytest.Register(t, c.Cwd, live.Agent)
	out := Run(context.Background(), c, Options{Name: "worker"})
	if r := out.Result.(Result); r.Record == nil || r.Record.ID != rec.ID {
		t.Fatalf("%+v", out)
	}
}

func TestGetByIDFailsClosedOnStaleTerminal(t *testing.T) {
	live := herdrscript.Info(herdrscript.LiveAgent("idle"))
	c := fake(t, call{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Result: live})
	c.Cwd = identitytest.Repository(t)
	recorded := live.Agent
	recorded.TerminalID = "term_old"
	rec := identitytest.Register(t, c.Cwd, recorded)
	out := Run(context.Background(), c, Options{ID: rec.ID})
	if out.Error == nil || out.Error.Code != "agent_identity_stale" || out.Error.Phase != "identity" || out.Result != nil {
		t.Fatalf("%+v", out)
	}
}

func TestGetIDExcludesOtherSelectors(t *testing.T) {
	for _, o := range []Options{{Name: "worker", ID: "0000beef"}, {Pane: "w1:p3", ID: "0000beef"}} {
		if out := Run(context.Background(), fake(t), o); out.ExitCode() != 2 || out.Error.Phase != "validation" {
			t.Fatalf("%+v", out)
		}
	}
}
