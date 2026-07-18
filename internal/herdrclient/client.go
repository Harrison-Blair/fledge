// Package herdrclient is a minimal client for the Herdr socket API:
// newline-delimited JSON (one request per line) over a Unix domain socket.
//
// Scope (Stage 0): transport, request/response correlation, session.snapshot,
// events.subscribe, and typed helpers for the methods the experiment
// harnesses use. This is the one component Stage 1 reuses as-is.
//
// The client makes no LLM calls and holds no durable state; it is an I/O
// adapter only (zero-inference invariant, docs/ARCHITECTURE.md).
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
	"strconv"
	"sync"
	"time"
)

// Envelope shapes. NOT yet verified against a live server: the reference
// snapshot documents the transport (NDJSON, one request per line, dot-notation
// methods) but not the exact envelope field names. Verify against
// `herdr api schema --json` (scripts/gen-herdr-types.sh) and update here.
// Tracked in docs/DECISIONS.md.
type request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// Response is one reply line, matched to its request by ID. Result and Error
// are kept raw so unknown fields pass through untouched (Herdr compatibility
// guidance: handle unknown fields gracefully).
type Response struct {
	ID     string          `json:"id"`
	OK     *bool           `json:"ok,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`

	// Raw is the complete line as received.
	Raw json.RawMessage `json:"-"`
}

// Err returns a non-nil error if the response carries one.
func (r *Response) Err() error {
	if len(r.Error) > 0 && string(r.Error) != "null" {
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

// Event is one asynchronous line (anything that does not correlate to a
// pending request ID), e.g. pane.agent_status_changed after events.subscribe.
type Event struct {
	Type string          `json:"event"`
	Data json.RawMessage `json:"data,omitempty"`

	// Raw is the complete line as received.
	Raw json.RawMessage `json:"-"`
}

// Client is a single-connection Herdr socket client. Safe for concurrent use.
type Client struct {
	conn net.Conn

	mu      sync.Mutex
	nextID  int
	pending map[string]chan *Response
	closed  bool

	events    chan Event
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

// Dial connects to the Herdr socket. Pass "" to use the environment-driven
// resolution order.
func Dial(ctx context.Context, socketPath string) (*Client, error) {
	path, err := SocketPath(socketPath)
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("dial herdr socket %s: %w", path, err)
	}
	c := &Client{
		conn:    conn,
		pending: make(map[string]chan *Response),
		events:  make(chan Event, 64),
	}
	go c.readLoop()
	return c, nil
}

// Close closes the connection; pending calls fail and Events() is closed.
func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return c.conn.Close()
}

// Events returns the stream of asynchronous lines. Subscribe first via
// EventsSubscribe. The channel closes when the connection ends; check Err()
// afterwards.
func (c *Client) Events() <-chan Event { return c.events }

// Err reports the terminal read-loop error, if any.
func (c *Client) Err() error {
	c.readErrMu.Lock()
	defer c.readErrMu.Unlock()
	return c.readErr
}

// Call sends one request and waits for its matching response (or ctx done).
func (c *Client) Call(ctx context.Context, method string, params any) (*Response, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("herdrclient: closed")
	}
	c.nextID++
	id := strconv.Itoa(c.nextID)
	ch := make(chan *Response, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	line, err := json.Marshal(request{ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	line = append(line, '\n')

	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
	} else {
		_ = c.conn.SetWriteDeadline(time.Time{})
	}
	if _, err := c.conn.Write(line); err != nil {
		return nil, fmt.Errorf("write %s: %w", method, err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("%s: %w", method, c.Err())
		}
		if err := resp.Err(); err != nil {
			return resp, fmt.Errorf("%s: %w", method, err)
		}
		return resp, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

func (c *Client) readLoop() {
	defer close(c.events)
	sc := bufio.NewScanner(c.conn)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		raw := append([]byte(nil), sc.Bytes()...)
		var resp Response
		if err := json.Unmarshal(raw, &resp); err == nil && resp.ID != "" {
			resp.Raw = raw
			c.mu.Lock()
			ch, ok := c.pending[resp.ID]
			c.mu.Unlock()
			if ok {
				ch <- &resp
				continue
			}
		}
		var ev Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			ev = Event{}
		}
		ev.Raw = raw
		select {
		case c.events <- ev:
		default:
			// Drop rather than block the read loop; the consumer is behind.
		}
	}
	err := sc.Err()
	if err == nil {
		err = errors.New("herdrclient: connection closed")
	}
	c.readErrMu.Lock()
	c.readErr = err
	c.readErrMu.Unlock()

	c.mu.Lock()
	c.closed = true
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()
}
