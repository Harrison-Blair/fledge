// Package herdrclient is a minimal client for the Herdr socket API:
// newline-delimited JSON (one request per line) over a Unix domain socket.
//
// Scope (Stage 0): transport, request/response correlation, session.snapshot,
// events.subscribe, and typed helpers for the methods the experiment
// harnesses use. This is the one component Stage 1 reuses as-is.
//
// The client makes no LLM calls and holds no durable state; it is an I/O
// adapter only (zero-inference invariant, docs/ARCHITECTURE.md).
//
// TRANSPORT (verified against live Herdr v0.7.4, protocol 16 — see
// docs/DECISIONS.md ADR-015): the server serves ONE request per connection.
// It reads a single request line, writes a single response line, and closes
// the socket. There is no multiplexing and no keep-alive on the RPC path, so
// Call dials a fresh connection for every request. Every request MUST carry a
// `params` field (an empty object `{}` when the method takes no arguments);
// omitting it yields `invalid_request: missing field params` and a dropped
// connection. Only events.subscribe holds a connection open, to stream
// subsequent event lines (Events()).
package herdrclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// request is one RPC line. Params is always emitted (never omitempty): the
// protocol-16 server requires the field on every method.
type request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

// Response is one reply line. Result and Error are kept raw so unknown fields
// pass through untouched (Herdr compatibility guidance: handle unknown fields
// gracefully).
type Response struct {
	ID     string          `json:"id"`
	OK     *bool           `json:"ok,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`

	// Raw is the complete line as received.
	Raw json.RawMessage `json:"-"`
}

// Err returns a non-nil error if the response carries one. Protocol-16 errors
// are `{"error":{"code":...,"message":...}}`; the code and message are
// surfaced so harness output is legible.
func (r *Response) Err() error {
	if len(r.Error) > 0 && string(r.Error) != "null" {
		var e struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(r.Error, &e) == nil && (e.Code != "" || e.Message != "") {
			return fmt.Errorf("herdr error: %s: %s", e.Code, e.Message)
		}
		return fmt.Errorf("herdr error: %s", r.Error)
	}
	if r.OK != nil && !*r.OK {
		return fmt.Errorf("herdr error: ok=false: %s", r.Raw)
	}
	return nil
}

// Decode unmarshals the result payload into v.
func (r *Response) Decode(v any) error {
	if len(r.Result) > 0 {
		return json.Unmarshal(r.Result, v)
	}
	// Some servers may inline result fields on the envelope; fall back to
	// decoding the whole line.
	return json.Unmarshal(r.Raw, v)
}

// Event is one asynchronous line delivered on a subscription connection.
type Event struct {
	Type string          `json:"event"`
	Data json.RawMessage `json:"data,omitempty"`

	// Raw is the complete line as received.
	Raw json.RawMessage `json:"-"`
}

// Client is a Herdr socket client. RPC calls (Call) are stateless and dial per
// request; only a subscription (EventsSubscribe) holds a connection open.
// Safe for concurrent use.
type Client struct {
	socketPath string

	// Streaming state for events.subscribe.
	mu     sync.Mutex
	stream net.Conn
	events chan Event

	readErrMu sync.Mutex
	readErr   error
}

// SocketPath resolves the socket to dial, mirroring Herdr's documented
// resolution order: explicit path > HERDR_SOCKET_PATH > HERDR_SESSION (named
// session socket under sessions/<name>/) > default session socket.
//
// The config-dir location for the two fallbacks is a documented assumption
// (docs/DECISIONS.md): HERDR_CONFIG_DIR if set, else <user config dir>/herdr.
func SocketPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if p := os.Getenv("HERDR_SOCKET_PATH"); p != "" {
		return p, nil
	}
	dir := os.Getenv("HERDR_CONFIG_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve herdr config dir: %w", err)
		}
		dir = filepath.Join(base, "herdr")
	}
	if s := os.Getenv("HERDR_SESSION"); s != "" {
		return filepath.Join(dir, "sessions", s, "herdr.sock"), nil
	}
	return filepath.Join(dir, "herdr.sock"), nil
}

// Dial resolves the socket path and verifies the session is reachable with a
// probe connection (the RPC path opens its own connection per call). Pass ""
// to use the environment-driven resolution order.
func Dial(ctx context.Context, socketPath string) (*Client, error) {
	path, err := SocketPath(socketPath)
	if err != nil {
		return nil, err
	}
	// Reachability probe: the server closes it immediately, which is fine.
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("dial herdr socket %s: %w", path, err)
	}
	_ = conn.Close()
	return &Client{
		socketPath: path,
		events:     make(chan Event, 64),
	}, nil
}

// Close tears down any open subscription stream. RPC calls hold no persistent
// connection, so on a client that only made Call requests this is a no-op.
func (c *Client) Close() error {
	c.mu.Lock()
	s := c.stream
	c.stream = nil
	c.mu.Unlock()
	if s != nil {
		return s.Close()
	}
	return nil
}

// Events returns the stream of asynchronous lines. Subscribe first via
// EventsSubscribe. The channel closes when the subscription connection ends;
// check Err() afterwards.
func (c *Client) Events() <-chan Event { return c.events }

// Err reports the terminal subscription read error, if any.
func (c *Client) Err() error {
	c.readErrMu.Lock()
	defer c.readErrMu.Unlock()
	return c.readErr
}

// Call sends one request on a fresh connection and returns its single
// response. Params is always serialized; a nil params becomes `{}` to satisfy
// the protocol-16 requirement that the field be present.
func (c *Client) Call(ctx context.Context, method string, params any) (*Response, error) {
	if params == nil {
		params = struct{}{}
	}
	line, err := json.Marshal(request{ID: "1", Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	line = append(line, '\n')

	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("%s: dial: %w", method, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if _, err := conn.Write(line); err != nil {
		return nil, fmt.Errorf("write %s: %w", method, err)
	}

	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("%s: read response: %w", method, err)
		}
		return nil, fmt.Errorf("%s: no response (connection closed)", method)
	}
	raw := append([]byte(nil), sc.Bytes()...)

	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("%s: decode response: %w", method, err)
	}
	resp.Raw = raw
	if err := resp.Err(); err != nil {
		return &resp, fmt.Errorf("%s: %w", method, err)
	}
	return &resp, nil
}

// subscribe opens the one long-lived connection Herdr keeps open: it sends the
// events.subscribe request, confirms the initial response, and streams every
// subsequent line into Events(). Called by EventsSubscribe (types.go).
func (c *Client) subscribe(ctx context.Context, method string, params any) error {
	if params == nil {
		params = struct{}{}
	}
	line, err := json.Marshal(request{ID: "1", Method: method, Params: params})
	if err != nil {
		return err
	}
	line = append(line, '\n')

	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("%s: dial: %w", method, err)
	}
	if _, err := conn.Write(line); err != nil {
		conn.Close()
		return fmt.Errorf("write %s: %w", method, err)
	}

	c.mu.Lock()
	c.stream = conn
	c.mu.Unlock()

	go c.streamLoop(conn)
	return nil
}

func (c *Client) streamLoop(conn net.Conn) {
	defer close(c.events)
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		raw := append([]byte(nil), sc.Bytes()...)
		var ev Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			ev = Event{}
		}
		ev.Raw = raw
		select {
		case c.events <- ev:
		default:
			// Drop rather than block; the consumer is behind.
		}
	}
	err := sc.Err()
	if err == nil {
		err = errors.New("herdrclient: subscription closed")
	}
	c.readErrMu.Lock()
	c.readErr = err
	c.readErrMu.Unlock()
}
