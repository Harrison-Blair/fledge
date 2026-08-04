//go:build !windows

package watchproc

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/watch"
)

func TestEventSetupResolvesAndDialsHerdrUnixSocket(t *testing.T) {
	directory, err := os.MkdirTemp("", "watchproc-sock-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	socketPath := filepath.Join(directory, "herdr.sock")
	listener, err := net.Listen("unix", socketPath)
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("sandbox does not permit Unix sockets: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	served := make(chan struct{})
	go func() {
		defer close(served)
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_, _ = bufio.NewReader(connection).ReadBytes('\n')
		_, _ = connection.Write([]byte(`{"result":{"type":"subscription_started"}}` + "\n"))
	}()

	config := testConfig()
	config.EventStream = true
	config.MinProtocol = 16
	client := &staticHerdr{
		protocol: 19,
		sessions: [][]herdr.Session{{{Name: testSession, Running: true, SocketPath: socketPath}}},
	}
	config, subscriber := configureEventStream(context.Background(), config, client, testSession, func(string) {})
	if !config.EventStream || subscriber == nil {
		t.Fatal("supported event stream was disabled")
	}
	ready := false
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = subscriber(ctx, []string{"pane-1"}, func() { ready = true }, func(watch.Event) {})
	<-served
	if !ready {
		t.Fatal("event subscriber did not reach its acknowledged ready state")
	}
}
