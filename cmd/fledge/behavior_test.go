package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

func TestHumanSizeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{9 * 1024, "9.0K"},
		{10 * 1024, "10K"},
		{1024*1024 - 1, "1024K"},
		{1024 * 1024, "1.0M"},
		{10 * 1024 * 1024, "10M"},
	} {
		if got := humanSize(tc.n); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestContextScanHumanOutputGroupsAndAlignsFiles(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	files := map[string]int{
		"a.txt":             1,
		"long-name.txt":     1024,
		"docs/readme.md":    10 * 1024,
		"docs/reference.md": 0,
	}
	for name, size := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)

	out, err := captureRun(t, "context", "scan")
	if err != nil {
		t.Fatalf("context scan: %v", err)
	}
	for _, want := range []string{"a.txt", "1B", "long-name.txt", "1.0K", "docs/", "readme.md", "10K", "reference.md", "0B"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "docs/") < strings.Index(out, "long-name.txt") {
		t.Errorf("root files should precede directory groups:\n%s", out)
	}
}

func TestFlockArgPrefersExplicitNameAndFallsBackToEnvironment(t *testing.T) {
	t.Setenv(flock.Env, "envflock")
	if got, err := flockArg("flock status", []string{"explicit"}); err != nil || got != "explicit" {
		t.Fatalf("explicit flock = %q, %v", got, err)
	}
	if got, err := flockArg("flock status", nil); err != nil || got != "envflock" {
		t.Fatalf("environment flock = %q, %v", got, err)
	}

	t.Setenv(flock.Env, "Bad")
	if _, err := flockArg("flock status", nil); err == nil || !strings.Contains(err.Error(), flock.Env) {
		t.Fatalf("bad environment error = %v", err)
	}
	if _, err := flockArg("flock status", []string{"alpha", "bravo"}); err == nil || !strings.Contains(err.Error(), "bravo") {
		t.Fatalf("extra argument error = %v", err)
	}
}

func TestFlockListReportsOutsideEmptyDownAndRunningWorkspaces(t *testing.T) {
	t.Run("outside", func(t *testing.T) {
		t.Chdir(t.TempDir())
		out, err := captureRun(t, "flock", "list")
		if err != nil || !strings.Contains(out, "no flocks") {
			t.Fatalf("list = %q, %v", out, err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		root, _ := scaffoldedWorkspace(t)
		t.Chdir(root)
		out, err := captureRun(t, "flock", "list")
		if err != nil || !strings.Contains(out, "no flocks") {
			t.Fatalf("list = %q, %v", out, err)
		}
	})
	t.Run("down and running", func(t *testing.T) {
		root, _ := scaffoldedWorkspace(t)
		t.Setenv("XDG_RUNTIME_DIR", shortRuntimeDir(t))
		if err := os.MkdirAll(flock.Dir(root, "alpha"), 0o755); err != nil {
			t.Fatal(err)
		}
		startDaemon(t, root, "bravo")
		t.Chdir(root)

		out, err := captureRun(t, "flock", "list")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"alpha  down", "bravo  up", "herdr:none", "agents:0/0"} {
			if !strings.Contains(out, want) {
				t.Errorf("list missing %q:\n%s", want, out)
			}
		}
	})
}

func TestFlockStatusShowsDownAndRunningDetails(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", shortRuntimeDir(t))
	startDaemon(t, root, "alpha")
	t.Chdir(root)

	down, err := captureRun(t, "flock", "status", "bravo")
	if err != nil || !strings.Contains(down, "daemon: down") || !strings.Contains(down, "herdr:  none") {
		t.Fatalf("down status = %q, %v", down, err)
	}
	up, err := captureRun(t, "flock", "status", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"flock:  alpha", "daemon: up", "socket:", "pid:", "version:", "herdr:  none", "no agents registered"} {
		if !strings.Contains(up, want) {
			t.Errorf("running status missing %q:\n%s", want, up)
		}
	}
}

func commandWorkspace(t *testing.T) string {
	t.Helper()
	root, _ := scaffoldedWorkspace(t)
	t.Setenv("XDG_RUNTIME_DIR", shortRuntimeDir(t))
	t.Setenv(flock.Env, "alpha")
	startDaemon(t, root, "alpha")
	t.Chdir(root)
	return root
}

func shortRuntimeDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "fr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func registerCLI(t *testing.T, typ string) string {
	t.Helper()
	out, err := captureRun(t, "agent", "register", "--pid", strconv.Itoa(os.Getpid()), typ)
	if err != nil {
		t.Fatalf("register %s: %v", typ, err)
	}
	return strings.TrimSpace(out)
}

func TestAgentRegisterAndListJSON(t *testing.T) {
	commandWorkspace(t)
	name := registerCLI(t, "worker")
	if name != "worker-emperor" {
		t.Fatalf("registered name = %q", name)
	}

	out, err := captureRun(t, "agent", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var agents []protocol.Agent
	if err := json.Unmarshal([]byte(out), &agents); err != nil {
		t.Fatalf("list JSON: %v\n%s", err, out)
	}
	if len(agents) != 1 || agents[0].Name != name || !agents[0].Alive || agents[0].Type != "worker" {
		t.Fatalf("agents = %+v", agents)
	}
}

func decimalPID(pid int) string {
	return strconv.Itoa(pid)
}

func TestAgentStopPropagatesDaemonError(t *testing.T) {
	commandWorkspace(t)
	// PID zero makes a self-registration dead but still present for stop's
	// ownership check; daemon-owned stop must reject it either way.
	out, err := captureRun(t, "agent", "register", "--pid", "99999999", "worker")
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimSpace(out)
	_, err = captureRun(t, "agent", "stop", name)
	if err == nil || !strings.Contains(err.Error(), "not spawned") {
		t.Fatalf("stop error = %v", err)
	}
}

func TestAgentListJSONUsesEmptyArray(t *testing.T) {
	commandWorkspace(t)
	out, err := captureRun(t, "agent", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty JSON roster = %q", out)
	}
}

func TestAgentMessageSendWaitAndReplyCorrelation(t *testing.T) {
	commandWorkspace(t)
	pid := decimalPID(os.Getpid())
	senderOut, err := captureRun(t, "agent", "register", "--pid", pid, "sender")
	if err != nil {
		t.Fatal(err)
	}
	receiverOut, err := captureRun(t, "agent", "register", "--pid", pid, "receiver")
	if err != nil {
		t.Fatal(err)
	}
	sender, receiver := strings.TrimSpace(senderOut), strings.TrimSpace(receiverOut)

	t.Setenv(protocol.AgentNameEnv, sender)
	askOut, err := captureRun(t, "agent", "msg", "send", receiver, "question")
	if err != nil {
		t.Fatal(err)
	}
	askID := strings.TrimSpace(askOut)
	if askID == "" {
		t.Fatal("send printed no message id")
	}
	t.Setenv(protocol.AgentNameEnv, receiver)
	if _, err := captureRun(t, "agent", "msg", "send", sender, "unrelated"); err != nil {
		t.Fatal(err)
	}
	if _, err := captureRun(t, "agent", "msg", "send", "--reply-to", askID, sender, "answer"); err != nil {
		t.Fatal(err)
	}

	t.Setenv(protocol.AgentNameEnv, sender)
	replyOut, err := captureRun(t, "agent", "msg", "wait", "--reply-to", askID, "--timeout", "1s")
	if err != nil {
		t.Fatal(err)
	}
	var reply protocol.Message
	if err := json.Unmarshal([]byte(replyOut), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Body != "answer" || reply.ReplyTo != askID {
		t.Fatalf("correlated reply = %+v", reply)
	}

	restOut, err := captureRun(t, "agent", "msg", "wait", "--timeout", "1s")
	if err != nil {
		t.Fatal(err)
	}
	var rest protocol.Message
	if err := json.Unmarshal([]byte(restOut), &rest); err != nil {
		t.Fatal(err)
	}
	if rest.Body != "unrelated" || rest.ReplyTo != "" {
		t.Fatalf("remaining message = %+v", rest)
	}
}

func TestAgentMessageDurationParsingAndRequiredFlags(t *testing.T) {
	commandWorkspace(t)
	pid := decimalPID(os.Getpid())
	out, err := captureRun(t, "agent", "register", "--pid", pid, "worker")
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimSpace(out)
	t.Setenv(protocol.AgentNameEnv, name)

	started := time.Now()
	_, err = captureRun(t, "agent", "msg", "wait", "--timeout", "25ms")
	if err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(started) < 15*time.Millisecond {
		t.Fatalf("duration wait = %v after %v", err, time.Since(started))
	}
	_, err = captureRun(t, "agent", "msg", "wait", "--timeout", "soon")
	if err == nil || !strings.Contains(err.Error(), "invalid duration") || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("invalid duration error = %v", err)
	}

	t.Setenv(protocol.AgentNameEnv, "")
	for _, args := range [][]string{
		{"agent", "msg", "send", name, "body"},
		{"agent", "msg", "wait", "--timeout", "1ms"},
		{"agent", "msg", "send", "--from", name, name, "body"},
		{"agent", "msg", "wait", "--as", name},
	} {
		if _, err := captureRun(t, args...); err == nil {
			t.Errorf("run(%q) succeeded", args)
		}
	}
	t.Setenv(protocol.AgentNameEnv, name)
	for _, args := range [][]string{
		{"agent", "msg", "send", "--from", name, name, "body"},
		{"agent", "msg", "wait", "--as", name},
	} {
		if _, err := captureRun(t, args...); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Errorf("run(%q) error = %v, want usage error", args, err)
		}
	}
}
