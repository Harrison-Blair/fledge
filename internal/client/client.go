// Package client dials the fledge daemon socket for a workspace and performs
// one request/response exchange per connection.
package client

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

// ErrNotRunning is returned when the workspace has no daemon listening. Its
// text is the operator-facing instruction, printed as-is by the CLI.
var ErrNotRunning = errors.New("daemon not running; run fledge start")

// Do sends one request to the daemon serving root's flock and returns its
// response. It
// blocks for as long as the daemon holds the connection open, which is how
// wait blocks.
func Do(root, flock string, req protocol.Request) (protocol.Response, error) {
	conn, err := net.Dial("unix", daemon.SocketPath(root, flock))
	if err != nil {
		return protocol.Response{}, ErrNotRunning
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return protocol.Response{}, err
	}

	var resp protocol.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return protocol.Response{}, err
	}
	if resp.Error != "" {
		return protocol.Response{}, errors.New(resp.Error)
	}
	return resp, nil
}

// Running reports whether a daemon is listening for root's flock.
func Running(root, flock string) bool {
	conn, err := net.Dial("unix", daemon.SocketPath(root, flock))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
