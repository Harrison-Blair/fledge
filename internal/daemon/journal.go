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
	evLaunching  = "agent.launching"
	evSpawned    = "agent.spawned"
	evReady      = "agent.ready"
	// evSettled is a legacy event from the removed pi RPC subprocess shape:
	// recognized on replay so old journals still load, never emitted.
	evSettled   = "agent.settled"
	evStopped   = "agent.stopped"
	evSent      = "msg.sent"
	evDelivered = "msg.delivered"
)

// event is one journal line. The union of every event's fields; each event
// writes only the ones it needs.
type event struct {
	Event string `json:"event"`

	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Species string `json:"species,omitempty"`
	PID     int    `json:"pid,omitempty"`

	Integration     string `json:"integration,omitempty"`
	Model           string `json:"model,omitempty"`
	Config          string `json:"config,omitempty"`
	Agent           string `json:"agent,omitempty"`
	Profile         string `json:"profile,omitempty"`
	Source          string `json:"source,omitempty"`
	PaneID          string `json:"pane_id,omitempty"`
	WorkspaceID     string `json:"workspace_id,omitempty"`
	WorkspaceLabel  string `json:"workspace_label,omitempty"`
	Cwd             string `json:"cwd,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	Reason          string `json:"reason,omitempty"`
	MsgID           string `json:"msg_id,omitempty"`
	TokenHash       string `json:"token_hash,omitempty"`
	InstructionHash string `json:"instruction_hash,omitempty"`

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
	// Fsync so "journaled before ack" survives power loss, not just a clean
	// process exit. In production journal is always an *os.File; a test seam is
	// not.
	if f, ok := d.journal.(*os.File); ok {
		if err := f.Sync(); err != nil {
			return fmt.Errorf("journal sync: %w", err)
		}
	}
	return nil
}

// state is the roster and pending set rebuilt from the journal.
type state struct {
	agents  map[string]protocol.Agent
	order   []string
	pending []protocol.Message
	tokens  map[string]string
}

// replay reconstructs state from an existing journal. A missing journal
// replays as empty state. A message that was sent but never delivered is
// pending; delivery order is the order the messages were sent in.
func replay(path string) (*state, error) {
	s := &state{agents: make(map[string]protocol.Agent), tokens: make(map[string]string)}

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return s, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines [][]byte
	// ends[k] is the byte offset in the file just past the newline that
	// terminated lines[k], so a torn tail can be truncated to the end of the
	// last complete line before the journal is reopened for append.
	var ends []int64
	var pos int64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		raw := sc.Bytes()
		pos += int64(len(raw)) + 1 // +1 for the '\n' ScanLines stripped
		if len(raw) > 0 {
			lines = append(lines, append([]byte(nil), raw...))
			ends = append(ends, pos)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	delivered := make(map[string]bool)
	var sent []protocol.Message
	launching := make(map[string]bool)
	spawned := make(map[string]bool)

	for i, line := range lines {
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			// A torn final line means the daemon died mid-append; everything
			// before it is still authoritative. A bad line anywhere else is
			// corruption.
			if i == len(lines)-1 {
				// Truncate the torn tail. Left in place, the next O_APPEND write
				// fuses onto it, so it stops being the final line and every later
				// replay hard-fails on the now-interior malformed line.
				var keep int64
				if i > 0 {
					keep = ends[i-1]
				}
				if err := os.Truncate(path, keep); err != nil {
					return nil, fmt.Errorf("journal %s: truncate torn tail: %w", path, err)
				}
				break
			}
			return nil, fmt.Errorf("journal %s: %w", path, err)
		}

		// A final event that landed complete but without its trailing newline
		// (a short write that still wrote a whole line) parses fine, so the loop
		// above never truncates it. Left unterminated, the next O_APPEND write
		// fuses onto it and every later replay hard-fails. It is a complete,
		// authoritative event, so re-terminate rather than discard.
		if i == len(lines)-1 {
			if err := ensureTrailingNewline(path); err != nil {
				return nil, err
			}
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
				Agent:   e.Agent,
				Profile: e.Profile,
				Source:  e.Source,
			}
			delete(s.tokens, e.Name)
			delete(launching, e.Name)
			delete(spawned, e.Name)
		case evLaunching:
			a := s.agents[e.Name]
			a.Integration = e.Integration
			a.Model = e.Model
			a.Config = e.Config
			a.Agent = e.Agent
			a.Profile = e.Profile
			a.Source = e.Source
			a.WorkspaceLabel = e.WorkspaceLabel
			a.State = stateStarting
			s.agents[e.Name] = a
			launching[e.Name] = true
			if e.TokenHash != "" {
				s.tokens[e.Name] = e.TokenHash
			}
		case evSpawned:
			a := s.agents[e.Name]
			if e.PID != 0 || a.PID == reservedPID {
				a.PID = e.PID
			}
			if e.Integration != "" {
				a.Integration = e.Integration
				a.Model = e.Model
				a.Config = e.Config
				a.Agent = e.Agent
				a.Profile = e.Profile
				a.Source = e.Source
			}
			a.PaneID = e.PaneID
			a.WorkspaceID = e.WorkspaceID
			if e.WorkspaceLabel != "" {
				a.WorkspaceLabel = e.WorkspaceLabel
			}
			if e.TokenHash != "" {
				s.tokens[e.Name] = e.TokenHash
			}
			if s.tokens[e.Name] == "" {
				a.State = stateRunning
			} else {
				a.State = stateStarting
			}
			s.agents[e.Name] = a
			spawned[e.Name] = true
		case evReady:
			if a, ok := s.agents[e.Name]; ok {
				a.State = stateRunning
				s.agents[e.Name] = a
				delete(s.tokens, e.Name)
			}
		case evSettled:
			// Legacy pi RPC event; the pane-less rule below already sidelines
			// every agent that could have emitted one, so it changes nothing.
		case evStopped:
			if a, ok := s.agents[e.Name]; ok {
				a.State = stateStopped
				s.agents[e.Name] = a
				delete(s.tokens, e.Name)
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

	// A durable launch intent without a corresponding spawned event is an
	// incomplete attempt. No CLI is trusted to authenticate after restart and
	// the claimed species is immediately reusable.
	for name := range launching {
		if spawned[name] {
			continue
		}
		a := s.agents[name]
		if a.State == stateStopped {
			continue
		}
		a.State = stateOrphaned
		s.agents[name] = a
		delete(s.tokens, name)
	}

	// A spawned agent with no pane came from the removed subprocess shape (pi
	// before it was pane-hosted); its pipes died with the daemon that owned
	// them, so a replayed one is unreachable however it looked when the journal
	// was written. A pane-hosted agent's pane may well have outlived the
	// daemon, so its state stands and alive(pid) reports on it.
	for name, a := range s.agents {
		if a.Integration != "" && a.PaneID == "" && a.State != stateStopped {
			a.State = stateOrphaned
			s.agents[name] = a
			delete(s.tokens, name)
		}
	}

	for _, m := range sent {
		if !delivered[m.ID] {
			s.pending = append(s.pending, m)
		}
	}
	return s, nil
}

// ensureTrailingNewline appends a newline when the journal's last byte is not
// one, so a final event written without its terminator does not fuse with the
// next O_APPEND write. A missing or empty file needs nothing.
func ensureTrailingNewline(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if fi.Size() == 0 {
		return nil
	}
	var last [1]byte
	if _, err := f.ReadAt(last[:], fi.Size()-1); err != nil {
		return err
	}
	if last[0] == '\n' {
		return nil
	}
	if _, err := f.WriteAt([]byte{'\n'}, fi.Size()); err != nil {
		return err
	}
	return nil
}
