// Package client exchanges requests with a workspace's Fledge daemon. It uses
// the Unix socket when available and a workspace-local file bridge when an
// agent sandbox denies socket access.
package client

import (
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/filebridge"
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
		return doFile(root, flock, req, 750*time.Millisecond, 0)
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
	if err == nil {
		conn.Close()
		return true
	}
	_, err = doFile(root, flock, protocol.Request{Op: protocol.OpStatus}, 250*time.Millisecond, 250*time.Millisecond)
	return err == nil
}

func doFile(root, flock string, req protocol.Request, acceptTimeout, responseTimeout time.Duration) (protocol.Response, error) {
	if !filebridge.Available(root, flock) {
		return protocol.Response{}, ErrNotRunning
	}
	id, err := filebridge.Submit(root, flock, req)
	if err != nil {
		return protocol.Response{}, ErrNotRunning
	}
	resp, err := filebridge.Await(root, flock, id, acceptTimeout, responseTimeout)
	if err != nil {
		return protocol.Response{}, ErrNotRunning
	}
	if resp.Error != "" {
		return protocol.Response{}, errors.New(resp.Error)
	}
	return resp, nil
}
