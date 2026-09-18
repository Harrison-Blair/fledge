package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func socket(t *testing.T, handle func(net.Conn)) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fh-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "s")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close(); os.RemoveAll(dir) })
	go func() {
		c, e := l.Accept()
		if e == nil {
			defer c.Close()
			handle(c)
		}
	}()
	return path
}
func TestCallLargeLineAndEmptyParams(t *testing.T) {
	path := socket(t, func(c net.Conn) {
		var req struct {
			ID     string         `json:"id"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(c).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Params == nil {
			t.Error("params must be an object")
		}
		json.NewEncoder(c).Encode(map[string]any{"id": req.ID, "result": map[string]any{"type": "large", "text": strings.Repeat("x", 100000)}})
	})
	var got struct{ Text string }
	if err := (Client{Socket: path}).Call(context.Background(), "test", nil, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Text) != 100000 {
		t.Fatal(len(got.Text))
	}
}
func TestCallErrors(t *testing.T) {
	for _, tc := range []struct {
		name, line, code string
		uncertain        bool
	}{
		{"empty invalid request", `{"id":"","error":{"code":"invalid_request","message":"unknown method"}}`, "invalid_request", false},
		{"server", `{"id":"fledge","error":{"code":"agent_blocked","message":"approval"}}`, "agent_blocked", false},
		{"bad json", `{`, "protocol_error", true},
		{"mismatch", `{"id":"other","result":{"type":"ok"}}`, "protocol_error", true},
		{"missing result", `{"id":"fledge"}`, "protocol_error", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := socket(t, func(c net.Conn) { bufio.NewReader(c).ReadString('\n'); c.Write([]byte(tc.line + "\n")) })
			var out any
			err := (Client{Socket: path}).Call(context.Background(), "test", nil, &out)
			var e *Error
			if !errors.As(err, &e) || e.Code != tc.code || e.Uncertain != tc.uncertain {
				t.Fatalf("%#v", err)
			}
		})
	}
}
func TestCallCancellation(t *testing.T) {
	path := socket(t, func(c net.Conn) { bufio.NewReader(c).ReadString('\n'); buf := make([]byte, 1); c.Read(buf) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var out any
	start := time.Now()
	err := (Client{Socket: path}).Call(ctx, "test", nil, &out)
	var e *Error
	if !errors.As(err, &e) || !e.Uncertain {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation did not interrupt read")
	}
}
