package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func fakeSocket(t *testing.T, handler func(net.Conn, Request)) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "herdr.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox does not permit Unix-domain listeners")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				var request Request
				if json.NewDecoder(conn).Decode(&request) == nil {
					handler(conn, request)
				}
			}()
		}
	}()
	return socket
}

func TestClientCorrelatesAndHandlesPartialReads(t *testing.T) {
	socket := fakeSocket(t, func(conn net.Conn, request Request) {
		payload := `{"id":"` + request.ID + `","result":{"type":"pong","version":"0.7.5","protocol":17,"future":true}}` + "\n"
		for _, part := range []string{payload[:7], payload[7:19], payload[19:]} {
			_, _ = conn.Write([]byte(part))
		}
	})
	client := Client{Socket: socket}
	pong, err := client.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pong.Protocol != 17 || pong.Version != "0.7.5" {
		t.Fatalf("unexpected pong: %#v", pong)
	}
}

func TestClientPreservesAPIErrors(t *testing.T) {
	socket := fakeSocket(t, func(conn net.Conn, request Request) {
		_ = json.NewEncoder(conn).Encode(map[string]any{
			"id": request.ID, "error": map[string]string{"code": "agent_not_found", "message": "missing"},
		})
	})
	err := (&Client{Socket: socket}).Call(context.Background(), "agent.get", map[string]string{"target": "x"}, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "agent_not_found" {
		t.Fatalf("error = %#v", err)
	}
}

func TestClientRejectsWrongCorrelationMalformedAndOversized(t *testing.T) {
	tests := []struct {
		name    string
		handler func(net.Conn, Request)
		match   string
	}{
		{"correlation", func(conn net.Conn, _ Request) {
			_, _ = conn.Write([]byte(`{"id":"wrong","result":{}}` + "\n"))
		}, "does not match"},
		{"malformed", func(conn net.Conn, _ Request) {
			_, _ = conn.Write([]byte("{what\n"))
		}, "decode"},
		{"oversized", func(conn net.Conn, request Request) {
			_, _ = conn.Write([]byte(`{"id":"` + request.ID + `","result":{"x":"`))
			_, _ = conn.Write([]byte(strings.Repeat("x", MaxResponseBytes)))
		}, "exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			socket := fakeSocket(t, test.handler)
			err := (&Client{Socket: socket}).Call(context.Background(), "ping", nil, nil)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v, want containing %q", err, test.match)
			}
		})
	}
}

func TestClientTimeoutAndCancellationCloseConnection(t *testing.T) {
	closed := make(chan struct{})
	var once sync.Once
	socket := fakeSocket(t, func(conn net.Conn, _ Request) {
		buffer := make([]byte, 1)
		_, _ = conn.Read(buffer)
		once.Do(func() { close(closed) })
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := (&Client{Socket: socket}).Call(ctx, "agent.wait", map[string]string{"target": "x"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("connection was not closed after cancellation")
	}
}
