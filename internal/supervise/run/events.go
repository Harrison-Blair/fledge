package run

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// event is one Herdr push the daemon acts on, or news from a subscription:
// subscribed after a connection's ack, drop for a pane that no longer exists.
type event struct {
	kind, pane, previous, status string
	// retry marks a subscribed event after a lost connection.
	retry bool
}

// subscriber subscribes to subs on one connection, sends a subscribed event
// once Herdr acknowledges, then forwards each push to out until ctx ends or
// the connection fails.
type subscriber func(ctx context.Context, subs []map[string]any, out chan<- event) error

// globalSubscriptions are the session-wide events on the global connection.
var globalSubscriptions = []map[string]any{{"type": "pane.exited"}, {"type": "pane.closed"}, {"type": "pane.moved"}, {"type": "pane.agent_detected"}}

// paneSubscriptions watch one pane's agent status.
func paneSubscriptions(pane string) []map[string]any {
	return []map[string]any{{"type": "pane.agent_status_changed", "pane_id": pane}}
}

// socketSubscriber streams events from the Herdr socket at path.
func socketSubscriber(path string) subscriber {
	return func(ctx context.Context, subs []map[string]any, out chan<- event) error {
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
		if err != nil {
			return &herdr.Error{Code: "connection_error", Message: err.Error()}
		}
		defer conn.Close()
		stop := context.AfterFunc(ctx, func() { conn.Close() })
		defer stop()
		request := map[string]any{"id": "fledge-supervise", "method": "events.subscribe", "params": map[string]any{"subscriptions": subs}}
		if err := json.NewEncoder(conn).Encode(request); err != nil {
			return err
		}
		r := bufio.NewReader(conn)
		line, err := r.ReadBytes('\n')
		if err != nil {
			return err
		}
		var ack struct {
			Result *struct {
				Type string `json:"type"`
			} `json:"result"`
			Error *herdr.Error `json:"error"`
		}
		if err := json.Unmarshal(line, &ack); err != nil {
			return libagent.Protocol("invalid events.subscribe reply: " + err.Error())
		}
		if ack.Error != nil {
			return ack.Error
		}
		if ack.Result == nil || ack.Result.Type != "subscription_started" {
			return libagent.Protocol("events.subscribe was not acknowledged")
		}
		if !send(ctx, out, event{kind: "subscribed"}) {
			return ctx.Err()
		}
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return err
			}
			if e, ok := decode(line); ok && !send(ctx, out, e) {
				return ctx.Err()
			}
		}
	}
}

// decode reads one pushed line; lines of other kinds are ignored.
func decode(line []byte) (event, bool) {
	var push struct {
		Event string `json:"event"`
		Data  struct {
			PaneID         string `json:"pane_id"`
			AgentStatus    string `json:"agent_status"`
			PreviousPaneID string `json:"previous_pane_id"`
			Pane           struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"data"`
	}
	if json.Unmarshal(line, &push) != nil {
		return event{}, false
	}
	d := push.Data
	switch push.Event {
	case "pane.agent_status_changed":
		return event{kind: "status", pane: d.PaneID, status: d.AgentStatus}, d.PaneID != "" && libagent.IsStatus(d.AgentStatus)
	case "pane_exited", "pane_closed":
		return event{kind: "exited", pane: d.PaneID}, d.PaneID != ""
	case "pane_moved":
		return event{kind: "moved", previous: d.PreviousPaneID, pane: d.Pane.PaneID}, d.PreviousPaneID != "" && d.Pane.PaneID != ""
	case "pane_agent_detected":
		return event{kind: "detected", pane: d.PaneID}, true
	}
	return event{}, false
}

func send(ctx context.Context, out chan<- event, e event) bool {
	select {
	case out <- e:
		return true
	case <-ctx.Done():
		return false
	}
}

// Reconnect backoff bounds.
const (
	firstBackoff = time.Second
	maxBackoff   = 30 * time.Second
)

// keep holds one subscription open until ctx ends, reconnecting after a lost
// connection with a backoff from 1 second doubling to 30 seconds. A pane
// subscription whose pane is gone sends drop and ends.
func keep(ctx context.Context, sub subscriber, subs []map[string]any, out chan<- event, pane string, logger *log.Logger) {
	wait, retry := firstBackoff, false
	for {
		relay := make(chan event)
		done := make(chan error, 1)
		go func() { done <- sub(ctx, subs, relay) }()
		var err error
	stream:
		for {
			select {
			case e := <-relay:
				if e.kind == "subscribed" {
					e.retry, retry, wait = retry, false, firstBackoff
				}
				if !send(ctx, out, e) {
					return
				}
			case err = <-done:
				break stream
			}
		}
		if ctx.Err() != nil {
			return
		}
		var remote *herdr.Error
		if pane != "" && errors.As(err, &remote) && remote.Code == "pane_not_found" {
			send(ctx, out, event{kind: "drop", pane: pane})
			return
		}
		label := "global"
		if pane != "" {
			label = pane
		}
		logger.Printf("reconnect %s in %s: %v", label, wait, cli.AtPhase("events.subscribe", err))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait, retry = min(2*wait, maxBackoff), true
	}
}
