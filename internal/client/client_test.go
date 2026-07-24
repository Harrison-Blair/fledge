package client

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func serveOnce(t *testing.T, root, flockName string, handler func(net.Conn)) {
	t.Helper()
	sock := daemon.SocketPath(root, flockName)
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			handler(conn)
			conn.Close()
		}
	}()
}

func TestDoExchangesOneJSONRequestAndResponse(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	root := t.TempDir()
	got := make(chan protocol.Request, 1)
	serveOnce(t, root, "alpha", func(conn net.Conn) {
		var req protocol.Request
		if err := json.NewDecoder(conn).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		got <- req
		json.NewEncoder(conn).Encode(protocol.Response{Name: "worker-emperor", ID: "m1"})
	})

	resp, err := Do(root, "alpha", protocol.Request{Op: protocol.OpSend, From: "ops", To: "worker", Body: "hello"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.Name != "worker-emperor" || resp.ID != "m1" {
		t.Fatalf("response = %+v", resp)
	}
	req := <-got
	if req.Op != protocol.OpSend || req.From != "ops" || req.To != "worker" || req.Body != "hello" {
		t.Fatalf("request = %+v", req)
	}
}

func TestDoReturnsDaemonError(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	root := t.TempDir()
	serveOnce(t, root, "alpha", func(conn net.Conn) {
		bufio.NewReader(conn).ReadString('\n')
		json.NewEncoder(conn).Encode(protocol.Response{Error: "agent missing"})
	})

	_, err := Do(root, "alpha", protocol.Request{Op: protocol.OpList})
	if err == nil || err.Error() != "agent missing" {
		t.Fatalf("err = %v", err)
	}
}

func TestDoReportsMalformedAndClosedResponses(t *testing.T) {
	for _, tc := range []struct {
		name, response string
	}{
		{name: "malformed", response: "not-json\n"},
		{name: "closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
			root := t.TempDir()
			serveOnce(t, root, "alpha", func(conn net.Conn) {
				bufio.NewReader(conn).ReadString('\n')
				if tc.response != "" {
					conn.Write([]byte(tc.response))
				}
			})

			_, err := Do(root, "alpha", protocol.Request{Op: protocol.OpList})
			if err == nil {
				t.Fatal("Do succeeded without a valid response")
			}
			if tc.name == "malformed" && !strings.Contains(err.Error(), "invalid character") {
				t.Fatalf("malformed error = %v", err)
			}
		})
	}
}

func TestRunningDetectsListeningAndDownSockets(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	root := t.TempDir()
	if Running(root, "alpha") {
		t.Fatal("Running reported an absent daemon")
	}

	sock := daemon.SocketPath(root, "alpha")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	if !Running(root, "alpha") {
		t.Fatal("Running missed a listener")
	}
	ln.Close()
	if Running(root, "alpha") {
		t.Fatal("Running reported a closed listener")
	}
}

func TestDoMapsDialFailureToErrNotRunning(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	_, err := Do(t.TempDir(), "alpha", protocol.Request{Op: protocol.OpList})
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("err = %v, want ErrNotRunning", err)
	}
}

func TestDoFallsBackToWorkspaceFilesWhenSocketIsInaccessible(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	runtimeA, err := os.MkdirTemp("/tmp", "fledge-client-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(runtimeA) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeA)
	d, err := daemon.New(root, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		d.Serve()
		close(done)
	}()
	t.Cleanup(func() {
		d.Close()
		<-done
	})

	// A sandbox can see and write the workspace but cannot see or connect to
	// the runtime-directory socket. A different runtime dir reproduces the
	// dial failure while leaving the daemon itself alive.
	runtimeB, err := os.MkdirTemp("/tmp", "fledge-client-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(runtimeB) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeB)
	resp, err := Do(root, "alpha", protocol.Request{
		Op: protocol.OpRegister, Type: "orchestrator-child", PID: os.Getpid(),
	})
	if err != nil {
		t.Fatalf("file fallback register: %v", err)
	}
	if resp.Name != "orchestrator-child-emperor" {
		t.Fatalf("registered name = %q", resp.Name)
	}
	if !Running(root, "alpha") {
		t.Fatal("Running missed a daemon reachable through the file fallback")
	}
}
