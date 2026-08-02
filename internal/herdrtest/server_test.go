package herdrtest

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSocketPathUsesTMPDIR(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	socket := socketPath(t)
	if got := filepath.Dir(filepath.Dir(socket)); got != tmp {
		t.Fatalf("socket parent = %q, want TMPDIR %q", got, tmp)
	}
	if got := filepath.Base(filepath.Dir(socket)); !strings.HasPrefix(got, "fht-") {
		t.Fatalf("socket directory = %q, want fht- prefix", got)
	}
	if got := filepath.Base(socket); got != "h.sock" {
		t.Fatalf("socket name = %q, want h.sock", got)
	}
}

func TestSocketPathDoesNotContainLongTestName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	namePart := "socket-path-must-not-leak"
	longName := strings.Repeat(namePart+"-", 12)

	t.Run(longName, func(t *testing.T) {
		socket := socketPath(t)
		if strings.Contains(socket, namePart) {
			t.Fatalf("socket path leaked test name: %q", socket)
		}
		if got := filepath.Base(socket); got != "h.sock" {
			t.Fatalf("socket name = %q, want h.sock", got)
		}
	})
}

func TestSocketPathCleanupRemovesDirectory(t *testing.T) {
	var dir string

	ok := t.Run("listen", func(t *testing.T) {
		dir = filepath.Dir(Listen(t, func(net.Conn, Call) {}))
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("listening directory %q: %v", dir, err)
		}
	})
	if !ok || dir == "" {
		return
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("socket directory remains after cleanup: %q: %v", dir, err)
	}
}

func TestListenBindsWithLongTestName(t *testing.T) {
	longName := strings.Repeat("long-descriptive-test-name-", 12)

	t.Run(longName, func(t *testing.T) {
		socket := Listen(t, func(conn net.Conn, call Call) {
			WriteResult(conn, call, map[string]any{"type": "ok"})
		})
		conn, err := net.Dial("unix", socket)
		if err != nil {
			t.Fatalf("dial Unix socket %q (%d bytes): %v", socket, len([]byte(socket)), err)
		}
		defer conn.Close()
		if err := json.NewEncoder(conn).Encode(Call{ID: "1", Method: "ping"}); err != nil {
			t.Fatal(err)
		}
		var response struct {
			ID     string         `json:"id"`
			Result map[string]any `json:"result"`
		}
		if err := json.NewDecoder(conn).Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response.ID != "1" || response.Result["type"] != "ok" {
			t.Fatalf("response = %#v", response)
		}
	})
}
