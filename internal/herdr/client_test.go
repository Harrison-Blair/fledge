// The client's own tests live outside package herdr so that they can share the
// socket fake in herdrtest, which imports herdr.
package herdr_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrtest"
)

func TestClientCorrelatesAndHandlesPartialReads(t *testing.T) {
	socket := herdrtest.Listen(t, func(conn net.Conn, request herdrtest.Call) {
		payload := `{"id":"` + request.ID + `","result":{"type":"pong","version":"0.7.5","protocol":17,"future":true}}` + "\n"
		for _, part := range []string{payload[:7], payload[7:19], payload[19:]} {
			_, _ = conn.Write([]byte(part))
		}
	})
	client := herdr.Client{Socket: socket}
	pong, err := client.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pong.Protocol != 17 || pong.Version != "0.7.5" {
		t.Fatalf("unexpected pong: %#v", pong)
	}
}

func TestClientPreservesAPIErrors(t *testing.T) {
	socket := herdrtest.Listen(t, func(conn net.Conn, request herdrtest.Call) {
		_ = json.NewEncoder(conn).Encode(map[string]any{
			"id": request.ID, "error": map[string]string{"code": "agent_not_found", "message": "missing"},
		})
	})
	err := (&herdr.Client{Socket: socket}).Call(context.Background(), "agent.get", map[string]string{"target": "x"}, nil)
	var apiErr *herdr.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "agent_not_found" {
		t.Fatalf("error = %#v", err)
	}
}

func TestClientRejectsWrongCorrelationMalformedAndOversized(t *testing.T) {
	tests := []struct {
		name    string
		handler func(net.Conn, herdrtest.Call)
		match   string
	}{
		{"correlation", func(conn net.Conn, _ herdrtest.Call) {
			_, _ = conn.Write([]byte(`{"id":"wrong","result":{}}` + "\n"))
		}, "does not match"},
		{"malformed", func(conn net.Conn, _ herdrtest.Call) {
			_, _ = conn.Write([]byte("{what\n"))
		}, "decode"},
		{"oversized", func(conn net.Conn, request herdrtest.Call) {
			_, _ = conn.Write([]byte(`{"id":"` + request.ID + `","result":{"x":"`))
			_, _ = conn.Write([]byte(strings.Repeat("x", herdr.MaxResponseBytes)))
		}, "exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			socket := herdrtest.Listen(t, test.handler)
			err := (&herdr.Client{Socket: socket}).Call(context.Background(), "ping", nil, nil)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v, want containing %q", err, test.match)
			}
		})
	}
}

func TestClientTimeoutAndCancellationCloseConnection(t *testing.T) {
	closed := make(chan struct{})
	var once sync.Once
	socket := herdrtest.Listen(t, func(conn net.Conn, _ herdrtest.Call) {
		buffer := make([]byte, 1)
		_, _ = conn.Read(buffer)
		once.Do(func() { close(closed) })
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := (&herdr.Client{Socket: socket}).Call(ctx, "agent.wait", map[string]string{"target": "x"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("connection was not closed after cancellation")
	}
}
