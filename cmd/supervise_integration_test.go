package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/sockettest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
)

// fakeHerdr answers every connection on l: agent.get and agent.list with a
// blocked worker in w1:p3, agent.prompt with agent_blocked, and
// events.subscribe with an ack, holding the connection open.
func fakeHerdr(t *testing.T, l net.Listener, worker herdr.AgentDetails) {
	t.Helper()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadBytes('\n')
				if err != nil {
					return
				}
				var req struct {
					ID     string         `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				json.Unmarshal(line, &req)
				reply := map[string]any{"id": req.ID}
				switch req.Method {
				case "agent.get":
					if req.Params["target"] != "worker" && req.Params["target"] != "w1:p3" {
						reply["error"] = map[string]any{"code": "agent_not_found", "message": "none"}
						break
					}
					reply["result"] = herdr.AgentResult{Type: "agent_info", Agent: worker}
				case "agent.list":
					reply["result"] = herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{worker}}
				case "agent.prompt":
					reply["error"] = map[string]any{"code": "agent_blocked", "message": "dialog"}
				case "events.subscribe":
					reply["result"] = map[string]any{"type": "subscription_started"}
					data, _ := json.Marshal(reply)
					conn.Write(append(data, '\n'))
					buf := make([]byte, 1)
					conn.Read(buf)
					return
				default:
					reply["error"] = map[string]any{"code": "invalid_request", "message": req.Method}
				}
				data, _ := json.Marshal(reply)
				conn.Write(append(data, '\n'))
			}(conn)
		}
	}()
}

// fledgeBinary builds this module's binary once per test.
func fledgeBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fledge")
	build := exec.Command("go", "build", "-o", bin, "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// A message the recipient cannot take yet starts the supervision daemon: a
// detached fledge supervise run in its own session, without the caller's
// pane, holding the lock. A second daemon exits at once; stop ends the first.
func TestDeferredMessageLaunchesRealDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the binary")
	}
	bin := fledgeBinary(t)
	l, socket := sockettest.Listen(t)
	t.Setenv("HERDR_SESSION", "itest")
	repo := identitytest.Repository(t)
	w := tasktest.Agent("w1:p3", "term_worker", "worker").Agent
	w.AgentStatus = "blocked"
	rec := identitytest.Register(t, repo, w)
	fakeHerdr(t, l, w)
	env := append(os.Environ(), "HERDR_ENV=1", "HERDR_SOCKET_PATH="+socket, "HERDR_SESSION=itest", "HERDR_PANE_ID=w9:p9")
	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir, cmd.Env = repo, env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	dir := supervise.Dir(repo, "itest")
	t.Cleanup(func() { supervise.Stop(context.Background(), dir) })

	out, err := run("agent", "message", "--name", "worker", "--body", "hello", "--json")
	var outcome cli.Outcome
	json.Unmarshal([]byte(out), &outcome)
	if err == nil || outcome.Status != "partial" || !strings.Contains(out, `"state":"queued"`) || !strings.Contains(out, `"action":"started","kind":"supervisor","id":"itest"`) {
		t.Fatalf("%v\n%s", err, out)
	}
	var st supervise.Status
	deadline := time.Now().Add(15 * time.Second)
	for {
		var live bool
		st, live, _ = supervise.Read(dir)
		if live && st.PID > 0 && len(st.Watched) == 1 {
			break
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(dir, "log"))
			t.Fatalf("daemon not watching: %+v\n%s", st, log)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if st.Watched[0] != rec.ID || st.Version == "" {
		t.Fatalf("%+v", st)
	}
	if runtime.GOOS == "linux" {
		environ, _ := os.ReadFile("/proc/" + strconv.Itoa(st.PID) + "/environ")
		if bytes.Contains(environ, []byte("HERDR_PANE_ID")) || !bytes.Contains(environ, []byte("HERDR_SESSION=itest")) {
			t.Fatalf("child environment: %q", environ)
		}
		stat, _ := os.ReadFile("/proc/" + strconv.Itoa(st.PID) + "/stat")
		fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
		if fields[3] != strconv.Itoa(st.PID) {
			t.Fatalf("not a session leader: sid %s pid %d", fields[3], st.PID)
		}
	}
	// A concurrent second daemon loses the lock and exits 0 without output.
	second := exec.Command(bin, "supervise", "run")
	second.Dir, second.Env = repo, env
	if out, err := second.CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("second daemon: %v %q", err, out)
	}
	if out, err := run("supervise", "status"); err != nil || !strings.Contains(out, "running (pid "+strconv.Itoa(st.PID)) || !strings.Contains(out, rec.ID) {
		t.Fatalf("%v %s", err, out)
	}
	if out, err := run("supervise", "stop"); err != nil || out != "Supervisor for session itest stopped.\n" {
		t.Fatalf("%v %q", err, out)
	}
	if _, live, _ := supervise.Read(dir); live {
		t.Fatal("still live after stop")
	}
	log, _ := os.ReadFile(filepath.Join(dir, "log"))
	if !strings.Contains(string(log), "start pid") || !strings.Contains(string(log), "exit: stopped") {
		t.Fatalf("log:\n%s", log)
	}
	s, _ := identity.Existing(context.Background(), repo)
	if all, _ := inbox.ForRecipient(s, rec.ID); len(all) != 1 || inbox.State(all[0]) != "queued" {
		t.Fatalf("%+v", all)
	}
}
