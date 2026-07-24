package daemon_test

import (
	"bufio"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func TestNewRejectsMissingScaffold(t *testing.T) {
	_, err := daemon.New(t.TempDir(), "alpha")
	if err == nil || !strings.Contains(err.Error(), "run fledge init") {
		t.Fatalf("New error = %v", err)
	}
}

func TestNewRejectsInvalidFlockNames(t *testing.T) {
	root := workspace(t)
	for _, name := range []string{"", "Upper", "has-dash", strings.Repeat("a", 33)} {
		if _, err := daemon.New(root, name); err == nil {
			t.Errorf("New accepted flock name %q", name)
		}
	}
}

func TestNewRejectsOversizedSocketPath(t *testing.T) {
	root := workspace(t)
	runtime := filepath.Join(t.TempDir(), strings.Repeat("runtime", 16))
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	if len(daemon.SocketPath(root, "alpha")) <= 103 {
		t.Fatalf("test socket path is not oversized: %s", daemon.SocketPath(root, "alpha"))
	}
	_, err := daemon.New(root, "alpha")
	if err == nil || !strings.Contains(err.Error(), "over the 103 limit") {
		t.Fatalf("New error = %v", err)
	}
}

func TestUnknownOperationReturnsDaemonError(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	_, err := client.Do(root, testFlock, protocol.Request{Op: "does-not-exist"})
	if err == nil || !strings.Contains(err.Error(), `unknown op "does-not-exist"`) {
		t.Fatalf("unknown op error = %v", err)
	}
}

func TestMalformedClientRequestIsDroppedWithoutStoppingDaemon(t *testing.T) {
	root := workspace(t)
	defer start(t, root, testFlock)()

	conn, err := net.Dial("unix", daemon.SocketPath(root, testFlock))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("not-json\n")); err != nil {
		t.Fatal(err)
	}
	if unix, ok := conn.(*net.UnixConn); ok {
		unix.CloseWrite()
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = bufio.NewReader(conn).ReadByte()
	conn.Close()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("malformed request read error = %v, want EOF with no response", err)
	}

	resp, err := client.Do(root, testFlock, protocol.Request{Op: protocol.OpList})
	if err != nil || len(resp.Agents) != 0 {
		t.Fatalf("daemon after malformed request = %+v, %v", resp, err)
	}
}

func TestRunRequiresScaffoldingBeforeServing(t *testing.T) {
	err := daemon.Run(t.TempDir(), "alpha")
	if err == nil || !strings.Contains(err.Error(), scaffold.DirName) {
		t.Fatalf("Run error = %v", err)
	}
}
