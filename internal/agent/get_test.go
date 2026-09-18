package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

func TestGetTargetsAndStates(t *testing.T) {
	for _, status := range []string{"idle", "working", "blocked", "done", "unknown"} {
		for _, byPane := range []bool{false, true} {
			p := liveAgent(status)
			o, target := GetOptions{Name: "worker"}, "worker"
			if byPane {
				p.Name = nil
				o = GetOptions{Pane: p.PaneID}
				target = p.PaneID
			}
			out := fake(t, call{method: "agent.get", params: map[string]any{"target": target}, result: info(p)}).Get(context.Background(), o)
			if out.Operation != "agent.get" || out.Status != "success" || out.ExitCode() != 0 || out.Effects == nil || len(out.Effects) != 0 {
				t.Fatalf("%+v", out)
			}
			if !reflect.DeepEqual(out.Result.(GetResult).AgentRow, row(p)) {
				t.Fatalf("%+v", out.Result)
			}
		}
	}
}

func TestGetInvalidTargets(t *testing.T) {
	for _, o := range []GetOptions{{}, {Name: "worker", Pane: "w1:p3"}} {
		out := fake(t).Get(context.Background(), o)
		if out.ExitCode() != 2 || out.Status != "rejected" || out.Error.Phase != "validation" {
			t.Fatalf("%+v", out)
		}
	}
}

func TestGetFailures(t *testing.T) {
	for _, code := range []string{"agent_not_found", "transport_error", "protocol_error"} {
		out := fake(t, call{method: "agent.get", err: &herdr.Error{Code: code, Message: "failed", Uncertain: true}}).Get(context.Background(), GetOptions{Name: "worker"})
		if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != code || out.Error.Phase != "agent.get" || len(out.Effects) != 0 {
			t.Fatalf("%+v", out)
		}
	}
	for _, field := range []string{"type", "pane_id", "workspace_id", "tab_id", "agent_status"} {
		p := map[string]any{"pane_id": "w1:p3", "workspace_id": "w1", "tab_id": "w1:t2", "agent_status": "idle"}
		r := map[string]any{"type": "agent_info", "agent": p}
		if field == "type" {
			r[field] = "wrong"
		} else {
			p[field] = ""
		}
		out := fake(t, call{method: "agent.get", result: r}).Get(context.Background(), GetOptions{Name: "worker"})
		if out.ExitCode() != 1 || out.Status != "rejected" || out.Error.Code != "protocol_error" || out.Error.Phase != "agent.get" {
			t.Fatalf("%s: %+v", field, out)
		}
	}
}

func TestGetOutput(t *testing.T) {
	for _, detailed := range []bool{false, true} {
		p := map[string]any{"pane_id": "w1:p3", "workspace_id": "w1", "tab_id": "w1:t2", "agent_status": "idle"}
		want := "Name: -\nHarness: -\nStatus: idle\nWorkspace ID: w1\nTab ID: w1:t2\nPane ID: w1:p3\nWorking directory: -\nForeground working directory: -\nInteractive ready: -\nLaunch pending: -\nFocused: -\nTitle: -\nSession source: -\nSession harness: -\nSession reference kind: -\nSession reference value: -\n"
		if detailed {
			p["foreground_cwd"], p["interactive_ready"], p["launch_pending"], p["focused"], p["title"] = "/repo/sub", true, false, false, "Review"
			p["agent_session"] = map[string]any{"source": "herdr:claude", "agent": "claude", "kind": "id", "value": "session-1"}
			want = "Name: -\nHarness: -\nStatus: idle\nWorkspace ID: w1\nTab ID: w1:t2\nPane ID: w1:p3\nWorking directory: -\nForeground working directory: /repo/sub\nInteractive ready: true\nLaunch pending: false\nFocused: false\nTitle: Review\nSession source: herdr:claude\nSession harness: claude\nSession reference kind: id\nSession reference value: session-1\n"
		}
		out := fake(t, call{method: "agent.get", result: map[string]any{"type": "agent_info", "agent": p}}).Get(context.Background(), GetOptions{Pane: "w1:p3"})
		var b bytes.Buffer
		if err := out.Write(&b, false); err != nil {
			t.Fatal(err)
		}
		if b.String() != want {
			t.Fatalf("got %q want %q", b.String(), want)
		}
		b.Reset()
		if err := out.Write(&b, true); err != nil {
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
		for _, key := range []string{"foreground_cwd", "interactive_ready", "launch_pending", "focused", "title", "agent_session"} {
			value, ok := result[key]
			if !ok || (!detailed && value != nil) {
				t.Fatalf("%s: %s", key, b.String())
			}
		}
		if detailed && (result["launch_pending"] != false || result["focused"] != false || result["interactive_ready"] != true || result["agent_session"].(map[string]any)["harness"] != "claude") {
			t.Fatal(b.String())
		}
	}
}
