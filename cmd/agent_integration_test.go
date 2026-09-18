package cmd

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSpawnForwardsExactNativeTokens(t *testing.T) {
	dir, err := os.MkdirTemp("", "fc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SOCKET_PATH", path)
	done := make(chan []string, 1)
	go func() {
		var argv []string
		for i := 0; i < 2; i++ {
			conn, err := l.Accept()
			if err != nil {
				done <- nil
				return
			}
			var req struct {
				ID     string `json:"id"`
				Method string `json:"method"`
				Params struct {
					Args []string `json:"args"`
				} `json:"params"`
			}
			json.NewDecoder(conn).Decode(&req)
			var result any
			if i == 0 {
				result = map[string]any{"type": "session_snapshot", "snapshot": map[string]any{"protocol": 999, "version": "future", "workspaces": []any{}, "tabs": []any{}, "layouts": []any{}, "agents": []any{}, "panes": []any{map[string]any{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1"}}}}
			} else {
				argv = req.Params.Args
				result = map[string]any{"type": "agent_started", "agent": map[string]any{"pane_id": "w1:p1", "workspace_id": "w1", "tab_id": "w1:t1", "agent": "claude", "agent_status": "idle"}, "argv": append([]string{"claude"}, argv...)}
			}
			json.NewEncoder(conn).Encode(map[string]any{"id": req.ID, "result": result})
			conn.Close()
		}
		done <- argv
	}()
	var out bytes.Buffer
	err = ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "w1:p1", "--args=--setting=a,b", "--args", "two words", "--json", "--", "--native", "x,y"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	if got := <-done; !reflect.DeepEqual(got, []string{"--setting=a,b", "two words", "--native", "x,y"}) {
		t.Fatalf("%q", got)
	}
	var envelope map[string]any
	if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err, out.String())
	}
	if envelope["status"] != "success" {
		t.Fatal(out.String())
	}
}
