package watch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

const (
	subscribeID    = "fledge-watch"
	subscribeCall  = "events.subscribe"
	agentStatusEvt = "pane.agent_status_changed"
	ackType        = "subscription_started"

	// defaultAckBudget bounds the wait for the subscription ack. Tests inject
	// a shorter one through subscribe.
	defaultAckBudget  = 5 * time.Second
	maxEventLineBytes = 64 * 1024
)

// Event is one Herdr pane.agent_status_changed notification.
type Event struct {
	PaneID      string
	AgentStatus string
	Agent       string
}

type subscription struct {
	Type   string `json:"type"`
	PaneID string `json:"pane_id"`
}

type subscribeParams struct {
	Subscriptions []subscription `json:"subscriptions"`
}

type subscribeMessage struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params subscribeParams `json:"params"`
}

// Subscribe opens one connection through dial, subscribes to the agent status
// stream for paneIDs, and hands every decoded event to onEvent until the
// stream ends. It always returns an error: ctx.Err() once ctx is done, and a
// wrapped transport error when the subscription fails or the server hangs up.
// Lines that are not decodable agent status events are ignored, so a newer
// Herdr adding fields or events cannot stall supervision.
//
// onReady runs once, after the subscription is acknowledged and before any
// event is delivered. Reconciling against a snapshot taken from there is what
// keeps an already-blocked worker from being missed: a snapshot read before
// the ack leaves a window in which a transition belongs to neither the
// snapshot nor the stream.
//
// Only the subscription ack is bounded by a deadline. Once subscribed the read
// blocks for as long as the server holds the connection open without sending,
// so callers MUST pass a bounded ctx: a Herdr that wedges with the socket open
// sends neither data nor EOF, and only ctx expiry turns that into the error
// that sends the caller back to polling.
func Subscribe(ctx context.Context, dial func(context.Context) (net.Conn, error), paneIDs []string, onReady func(), onEvent func(Event)) error {
	return subscribe(ctx, dial, paneIDs, onReady, onEvent, defaultAckBudget)
}

func subscribe(ctx context.Context, dial func(context.Context) (net.Conn, error), paneIDs []string, onReady func(), onEvent func(Event), ackBudget time.Duration) error {
	request, err := subscribeRequest(paneIDs)
	if err != nil {
		return err
	}

	conn, err := dial(ctx)
	if err != nil {
		return fmt.Errorf("dial Herdr event socket: %w", err)
	}
	defer conn.Close()

	// Closing the connection is what unblocks a read that ctx has outlived.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if _, err := conn.Write(request); err != nil {
		return streamError(ctx, "send Herdr subscription: %w", err)
	}

	reader := bufio.NewReaderSize(conn, maxEventLineBytes+1)

	_ = conn.SetReadDeadline(time.Now().Add(ackBudget))
	ack, err := readEventLine(reader)
	if err != nil {
		return streamError(ctx, "read Herdr subscription ack: %w", err)
	}
	if !acknowledged(ack) {
		return fmt.Errorf("read Herdr subscription ack: want %s", ackType)
	}
	_ = conn.SetReadDeadline(time.Time{})
	onReady()

	for {
		line, err := readEventLine(reader)
		if err != nil {
			return streamError(ctx, "read Herdr event stream: %w", err)
		}
		if event, ok := decodeEventLine(line); ok {
			onEvent(event)
		}
	}
}

func readEventLine(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) || len(line) > maxEventLineBytes {
		return nil, fmt.Errorf("Herdr event line exceeds %d bytes", maxEventLineBytes)
	}
	return line, err
}

// streamError reports a cancellation as such: closing the connection to unblock
// a read makes every pending operation fail, and that failure is not the story.
func streamError(ctx context.Context, format string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf(format, err)
}

func subscribeRequest(paneIDs []string) ([]byte, error) {
	if len(paneIDs) == 0 {
		return nil, errors.New("build Herdr subscription: no panes")
	}

	subscriptions := make([]subscription, 0, len(paneIDs))
	for _, paneID := range paneIDs {
		subscriptions = append(subscriptions, subscription{Type: agentStatusEvt, PaneID: paneID})
	}

	request, err := json.Marshal(subscribeMessage{
		ID:     subscribeID,
		Method: subscribeCall,
		Params: subscribeParams{Subscriptions: subscriptions},
	})
	if err != nil {
		return nil, fmt.Errorf("build Herdr subscription: %w", err)
	}

	return append(request, '\n'), nil
}

func acknowledged(line []byte) bool {
	var ack struct {
		Result struct {
			Type string `json:"type"`
		} `json:"result"`
	}
	if err := json.Unmarshal(line, &ack); err != nil {
		return false
	}

	return ack.Result.Type == ackType
}

func decodeEventLine(line []byte) (Event, bool) {
	var message struct {
		Event string `json:"event"`
		Data  struct {
			PaneID      string `json:"pane_id"`
			AgentStatus string `json:"agent_status"`
			Agent       string `json:"agent"`
		} `json:"data"`
	}
	if err := json.Unmarshal(line, &message); err != nil {
		return Event{}, false
	}
	if message.Event != agentStatusEvt {
		return Event{}, false
	}
	if message.Data.PaneID == "" || message.Data.AgentStatus == "" {
		return Event{}, false
	}

	return Event{
		PaneID:      message.Data.PaneID,
		AgentStatus: message.Data.AgentStatus,
		Agent:       message.Data.Agent,
	}, true
}
