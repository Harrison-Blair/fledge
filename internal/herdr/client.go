package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Error preserves server codes and distinguishes an unconfirmed request.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Uncertain bool   `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Client opens one connection for each request. Timeout is a transport limit.
type Client struct {
	Socket  string
	Timeout time.Duration
}

func (c Client) Call(ctx context.Context, method string, params any, result any) error {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return &Error{Code: "connection_error", Message: err.Error()}
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	if params == nil {
		params = struct{}{}
	}
	request := struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{"fledge", method, params}
	if err = json.NewEncoder(conn).Encode(request); err != nil {
		return &Error{Code: "transport_error", Message: err.Error(), Uncertain: true}
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return &Error{Code: "transport_error", Message: err.Error(), Uncertain: true}
	}
	var response struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
	}
	invalid := func(message string) error { return &Error{Code: "protocol_error", Message: message, Uncertain: true} }
	if err = json.Unmarshal(line, &response); err != nil {
		return invalid(err.Error())
	}
	if response.ID != "fledge" && !(response.ID == "" && response.Error != nil && response.Error.Code == "invalid_request") {
		return invalid("response ID does not match request")
	}
	hasResult := len(response.Result) > 0 && string(response.Result) != "null"
	if hasResult == (response.Error != nil) {
		return invalid("expected exactly one result or error")
	}
	if response.Error != nil {
		if response.Error.Code == "" || response.Error.Message == "" {
			return invalid("invalid server error")
		}
		return response.Error
	}
	var tag struct {
		Type string `json:"type"`
	}
	if err = json.Unmarshal(response.Result, &tag); err != nil || tag.Type == "" {
		return invalid("missing result discriminator")
	}
	if err = json.Unmarshal(response.Result, result); err != nil {
		return invalid(fmt.Sprintf("invalid %s result: %v", method, err))
	}
	return nil
}
