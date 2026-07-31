package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

const MaxResponseBytes = 4 << 20

var correlation atomic.Uint64

type Request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *APIError       `json:"error"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

type TransportError struct {
	Operation string
	Err       error
}

func (e *TransportError) Error() string { return fmt.Sprintf("Herdr %s: %v", e.Operation, e.Err) }
func (e *TransportError) Unwrap() error { return e.Err }

type Client struct {
	Socket string
	Dialer net.Dialer
}

func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	if params == nil {
		params = struct{}{}
	}
	id := strconv.FormatUint(correlation.Add(1), 10)
	conn, err := c.Dialer.DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return &TransportError{Operation: "connect", Err: err}
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := json.NewEncoder(conn).Encode(Request{ID: id, Method: method, Params: params}); err != nil {
		return &TransportError{Operation: "write", Err: contextError(ctx, err)}
	}
	line, err := readLine(conn, MaxResponseBytes)
	if err != nil {
		return &TransportError{Operation: "read", Err: contextError(ctx, err)}
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return &TransportError{Operation: "decode", Err: err}
	}
	if resp.ID != id {
		return &TransportError{Operation: "correlate", Err: fmt.Errorf("response id %q does not match request id %q", resp.ID, id)}
	}
	if resp.Error != nil {
		return resp.Error
	}
	if len(resp.Result) == 0 {
		return &TransportError{Operation: "decode", Err: errors.New("response has neither result nor error")}
	}
	if out != nil {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return &TransportError{Operation: "decode result", Err: err}
		}
	}
	return nil
}

func readLine(r io.Reader, limit int64) ([]byte, error) {
	br := bufio.NewReader(io.LimitReader(r, limit+1))
	line, err := br.ReadBytes('\n')
	if int64(len(line)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(line) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	return line, nil
}

func contextError(ctx context.Context, fallback error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var netErr net.Error
	if deadline, ok := ctx.Deadline(); ok && errors.As(fallback, &netErr) && netErr.Timeout() && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return fallback
}
