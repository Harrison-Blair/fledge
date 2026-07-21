package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Harrison-Blair/fledge/internal/protocol"
)

// Journal event names.
const (
	evStarted    = "daemon.started"
	evRegistered = "agent.registered"
	evSpawned    = "agent.spawned"
	evSettled    = "agent.settled"
	evStopped    = "agent.stopped"
	evSent       = "msg.sent"
	evDelivered  = "msg.delivered"
)

// event is one journal line. The union of every event's fields; each event
// writes only the ones it needs.
type event struct {
	Event string `json:"event"`

	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Species string `json:"species,omitempty"`
	PID     int    `json:"pid,omitempty"`

	Integration string `json:"integration,omitempty"`
	Model       string `json:"model,omitempty"`
	Config      string `json:"config,omitempty"`
	PaneID      string `json:"pane_id,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Reason      string `json:"reason,omitempty"`
	MsgID       string `json:"msg_id,omitempty"`

	ID      string `json:"id,omitempty"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Body    string `json:"body,omitempty"`
	ReplyTo string `json:"reply_to,omitempty"`
}

// append writes one event as a JSON line. It must return before the operation
// that produced the event is acknowledged to the client.
func (d *Daemon) append(e event) error {
	return d.appendAll(e)
}

// appendAll writes several events in a single write, so a caller whose facts
// only make sense together cannot leave half the pair in the journal. One
// write of a few hundred bytes to an O_APPEND file is what makes the pair
// atomic; replay's torn-final-line rule covers the rest.
func (d *Daemon) appendAll(events ...event) error {
	var buf []byte
	for _, e := range events {
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	if _, err := d.journal.Write(buf); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	return nil
}

// state is the roster and pending set rebuilt from the journal.
type state struct {
	agents  map[string]protocol.Agent
	order   []string
	pending []protocol.Message
}

// replay reconstructs state from an existing journal. A missing journal
// replays as empty state. A message that was sent but never delivered is
// pending; delivery order is the order the messages were sent in.
func replay(path string) (*state, error) {
	s := &state{agents: make(map[string]protocol.Agent)}

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return s, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		if line := sc.Bytes(); len(line) > 0 {
			lines = append(lines, append([]byte(nil), line...))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	delivered := make(map[string]bool)
	var sent []protocol.Message

	for i, line := range lines {
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			// A torn final line means the daemon died mid-append; everything
			// before it is still authoritative. A bad line anywhere else is
			// corruption.
			if i == len(lines)-1 {
				break
			}
			return nil, fmt.Errorf("journal %s: %w", path, err)
		}

		switch e.Event {
		case evRegistered:
			if _, ok := s.agents[e.Name]; !ok {
				s.order = append(s.order, e.Name)
			}
			s.agents[e.Name] = protocol.Agent{
				Name:    e.Name,
				Type:    e.Type,
				Species: e.Species,
				PID:     e.PID,
			}
		case evSpawned:
			a := s.agents[e.Name]
			a.Integration = e.Integration
			a.Model = e.Model
			a.Config = e.Config
			a.PaneID = e.PaneID
			a.State = stateRunning
			s.agents[e.Name] = a
		case evSettled:
			if a, ok := s.agents[e.Name]; ok {
				a.State = stateSettled
				s.agents[e.Name] = a
			}
		case evStopped:
			if a, ok := s.agents[e.Name]; ok {
				a.State = stateStopped
				s.agents[e.Name] = a
			}
		case evSent:
			sent = append(sent, protocol.Message{
				ID:      e.ID,
				From:    e.From,
				To:      e.To,
				Body:    e.Body,
				ReplyTo: e.ReplyTo,
			})
		case evDelivered:
			delivered[e.ID] = true
		}
	}

	// A pi agent's pipes died with the daemon that owned them, so a replayed
	// one is unreachable however it looked when the journal was written. A
	// Claude agent lives in a pane that may well have outlived the daemon, so
	// its state stands and alive(pid) reports on it.
	for name, a := range s.agents {
		if a.Integration == "pi" && a.State != stateStopped {
			a.State = stateOrphaned
			s.agents[name] = a
		}
	}

	for _, m := range sent {
		if !delivered[m.ID] {
			s.pending = append(s.pending, m)
		}
	}
	return s, nil
}
