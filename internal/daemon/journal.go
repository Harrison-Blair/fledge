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
	evPlaced     = "agent.placed"
	evSpawned    = "agent.spawned"
	evReady      = "agent.ready"
	// evSettled is a legacy event from the removed pi RPC subprocess shape:
	// recognized on replay so old journals still load, never emitted.
	evSettled           = "agent.settled"
	evStopped           = "agent.stopped"
	evTabCreateIntent   = "tab.create.intent"
	evTabCreateResolved = "tab.create.resolved"
	evTabCreated        = "tab.created"
	evTabClosing        = "tab.closing"
	evTabClosed         = "tab.closed"
	evWorkspaceClosing  = "workspace.closing"
	evWorkspaceClosed   = "workspace.closed"
	evSent              = "msg.sent"
	evDelivered         = "msg.delivered"
	evInboxNotified     = "inbox.notified"
)

type stopRecord struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type tabRecord struct {
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	TabLabel    string `json:"tab_label,omitempty"`
}

type pendingTabCreate struct {
	IntentID    string
	WorkspaceID string
	TabLabel    string
	CreateLabel string
	Cwd         string
}

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
	TabID           string `json:"tab_id,omitempty"`
	TabLabel        string `json:"tab_label,omitempty"`
	IntentID        string `json:"intent_id,omitempty"`
	CreateLabel     string `json:"create_label,omitempty"`
	OwnsWorkspace   bool   `json:"owns_workspace,omitempty"`
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
	// InboxNotifyArmed belongs to agent.ready. Keeping it on the same JSON
	// record makes readiness plus arming replay-atomic even after a torn write.
	InboxNotifyArmed bool     `json:"inbox_notify_armed,omitempty"`
	IDs              []string `json:"ids,omitempty"`

	Stops []stopRecord `json:"stops,omitempty"`
	Tabs  []tabRecord  `json:"tabs,omitempty"`
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

// state is the roster and message state rebuilt from the journal.
type state struct {
	agents            map[string]protocol.Agent
	order             []string
	pending           []protocol.Message
	notifyPending     []protocol.Message
	tokens            map[string]string
	credentials       map[string]string
	inboxNotifyArmed  map[string]bool
	inboxNotified     map[string]bool
	messages          map[string]protocol.Message
	messageOrder      []string
	messageDelivered  map[string]bool
	ownedTabs         map[string]ownedTab
	tabCreateIntents  map[string]pendingTabCreate
	tabClosures       map[string]tabRecord
	workspaceClosures map[string]event
}

// replay reconstructs state from an existing journal. A missing journal
// replays as empty state. A message that was sent but never delivered is
// pending. Real journals from the former pane-delivery implementation record
// msg.delivered with only id/to; those entries are final because no durable
// fact can prove whether replaying their body would duplicate handling.
func replay(path string) (*state, error) {
	s := &state{
		agents:            make(map[string]protocol.Agent),
		tokens:            make(map[string]string),
		credentials:       make(map[string]string),
		inboxNotifyArmed:  make(map[string]bool),
		inboxNotified:     make(map[string]bool),
		messages:          make(map[string]protocol.Message),
		messageDelivered:  make(map[string]bool),
		ownedTabs:         make(map[string]ownedTab),
		tabCreateIntents:  make(map[string]pendingTabCreate),
		tabClosures:       make(map[string]tabRecord),
		workspaceClosures: make(map[string]event),
	}

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
			delete(s.credentials, e.Name)
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
			a.Cwd = e.Cwd
			a.SessionID = e.SessionID
			a.State = stateStarting
			s.agents[e.Name] = a
			launching[e.Name] = true
			if e.TokenHash != "" {
				s.tokens[e.Name] = e.TokenHash
				s.credentials[e.Name] = e.TokenHash
			}
		case evPlaced:
			a := s.agents[e.Name]
			a.WorkspaceID = e.WorkspaceID
			a.WorkspaceLabel = e.WorkspaceLabel
			a.TabID = e.TabID
			a.TabLabel = e.TabLabel
			s.agents[e.Name] = a
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
			a.TabID = e.TabID
			a.TabLabel = e.TabLabel
			a.OwnsWorkspace = e.OwnsWorkspace
			// Journals predating explicit tab placement stored a dedicated
			// workspace id without an ownership bit or tab id.
			if a.WorkspaceID != "" && a.TabID == "" {
				a.OwnsWorkspace = true
			}
			if e.WorkspaceLabel != "" {
				a.WorkspaceLabel = e.WorkspaceLabel
			}
			if e.TokenHash != "" {
				s.tokens[e.Name] = e.TokenHash
				s.credentials[e.Name] = e.TokenHash
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
				if e.SessionID != "" {
					a.SessionID = e.SessionID
				}
				s.agents[e.Name] = a
				delete(s.tokens, e.Name)
			}
			if e.InboxNotifyArmed {
				s.inboxNotifyArmed[e.Name] = true
			}
		case evSettled:
		// Legacy pi RPC event; the pane-less rule below already sidelines
		// every agent that could have emitted one, so it changes nothing.
		case evStopped:
			if a, ok := s.agents[e.Name]; ok {
				a.State = stateStopped
				s.agents[e.Name] = a
				delete(s.tokens, e.Name)
				delete(s.inboxNotifyArmed, e.Name)
			}
		case evTabCreateIntent:
			s.tabCreateIntents[e.IntentID] = pendingTabCreate{
				IntentID:    e.IntentID,
				WorkspaceID: e.WorkspaceID,
				TabLabel:    e.TabLabel,
				CreateLabel: e.CreateLabel,
				Cwd:         e.Cwd,
			}
		case evTabCreateResolved:
			delete(s.tabCreateIntents, e.IntentID)
		case evTabCreated:
			if e.IntentID != "" {
				delete(s.tabCreateIntents, e.IntentID)
			}
			s.ownedTabs[e.TabID] = ownedTab{
				WorkspaceID: e.WorkspaceID,
				TabID:       e.TabID,
				Label:       e.TabLabel,
			}
		case evTabClosing:
			s.tabClosures[e.TabID] = tabRecord{
				WorkspaceID: e.WorkspaceID,
				TabID:       e.TabID,
				TabLabel:    e.TabLabel,
			}
		case evTabClosed:
			delete(s.ownedTabs, e.TabID)
			delete(s.tabClosures, e.TabID)
		case evWorkspaceClosing:
			s.workspaceClosures[e.WorkspaceID] = e
		case evWorkspaceClosed:
			delete(s.workspaceClosures, e.WorkspaceID)
		case evSent:
			msg := protocol.Message{
				ID:      e.ID,
				From:    e.From,
				To:      e.To,
				Body:    e.Body,
				ReplyTo: e.ReplyTo,
			}
			sent = append(sent, msg)
			s.messages[e.ID] = msg
			s.messageOrder = append(s.messageOrder, e.ID)
		case evDelivered:
			delivered[e.ID] = true
			s.messageDelivered[e.ID] = true
		case evInboxNotified:
			if e.ID != "" {
				s.inboxNotified[e.ID] = true
			}
			for _, id := range e.IDs {
				s.inboxNotified[id] = true
			}
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
		delete(s.credentials, name)
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
			delete(s.credentials, name)
		}
	}

	for _, m := range sent {
		if !delivered[m.ID] {
			s.pending = append(s.pending, m)
		}
	}
	for _, m := range sent {
		if s.inboxNotifyArmed[m.To] && !s.inboxNotified[m.ID] {
			s.notifyPending = append(s.notifyPending, m)
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
