// Package protocol defines the newline-delimited JSON wire format spoken
// between the fledge CLI and the fledge daemon over the workspace unix socket.
package protocol

const (
	AgentNameEnv  = "FLEDGE_AGENT_NAME"
	ReadyTokenEnv = "FLEDGE_READY_TOKEN"
)

// JournalName is the append-only event log, relative to a flock's directory.
const JournalName = "journal.jsonl"

// LogName is the daemon's human-readable debug log, relative to a flock's
// directory.
const LogName = "daemon.log"

// Operations a client may request.
const (
	OpRegister = "register"
	OpList     = "list"
	OpStatus   = "status"
	OpSend     = "send"
	OpWait     = "wait"
	OpSpawn    = "spawn"
	OpReady    = "ready"
	OpStop     = "stop"
)

// Request is one client command. Fields not relevant to Op are omitted.
type Request struct {
	Op string `json:"op"`

	// register
	Type    string `json:"type,omitempty"`
	Species string `json:"species,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Profile string `json:"profile,omitempty"`
	Source  string `json:"source,omitempty"`

	// send
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Body string `json:"body,omitempty"`

	// wait
	As        string `json:"as,omitempty"`
	TimeoutMS int64  `json:"timeout_ms,omitempty"`

	// send and wait
	ReplyTo string `json:"reply_to,omitempty"`

	// spawn: exactly one of Config (a name in .fledge/agents.json) or Model
	// (routed to an integration by prefix) selects what to launch.
	Config   string `json:"config,omitempty"`
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
	// Integration overrides the routed integration for a Model spawn — the
	// same model id can run under pi or codex. Never set with Config, which
	// names its integration itself.
	Integration string `json:"integration,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	// Split places a pane-hosted agent by splitting the focused pane
	// ("right" or "down"). Ignored by pi agents, which have no pane.
	Split string `json:"split,omitempty"`
	// AnchorPane asks the daemon to swap a newly created pane with this pane
	// and focus the new pane immediately after launch. Interactive start uses
	// it to place the managed orchestrator before readiness begins.
	AnchorPane string `json:"anchor_pane,omitempty"`
	// Orchestrator runs Config as the reserved orchestrator: it is resolved as
	// itself, but reserved under the bare orchestrator name. Only the fallback
	// pick of `fledge start` sets it; `fledge agent spawn` never does.
	Orchestrator bool `json:"orchestrator,omitempty"`

	// ready
	Token string `json:"token,omitempty"`

	// stop
	Name string `json:"name,omitempty"`
}

// Response is the daemon's single reply to a Request. A non-empty Error means
// the operation failed and every other field is unset.
type Response struct {
	Error string `json:"error,omitempty"`

	Name    string   `json:"name,omitempty"`
	ID      string   `json:"id,omitempty"`
	Agents  []Agent  `json:"agents,omitempty"`
	Message *Message `json:"message,omitempty"`

	// spawn: the Herdr pane hosting the agent, empty for pi subprocesses.
	PaneID string `json:"pane_id,omitempty"`

	// status: the Herdr session the daemon is bound to, empty when unbound.
	Session       string `json:"session,omitempty"`
	SessionSocket string `json:"session_socket,omitempty"`
}

// Agent is a registered agent as reported by list.
type Agent struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Species string `json:"species"`
	PID     int    `json:"pid"`
	Alive   bool   `json:"alive"`

	// Spawned agents only; all empty for self-registered agents.
	Integration string `json:"integration,omitempty"` // "claude" | "pi" | "codex"
	Model       string `json:"model,omitempty"`
	Config      string `json:"config,omitempty"` // agents.json entry it came from
	Agent       string `json:"agent,omitempty"`
	Profile     string `json:"profile,omitempty"`
	Source      string `json:"source,omitempty"`
	PaneID      string `json:"pane_id,omitempty"`
	State       string `json:"state,omitempty"` // starting | running | busy | settled | stopped | orphaned
}

// Message is a point-to-point message. ReplyTo correlates it with the message
// it answers.
type Message struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Body    string `json:"body"`
	ReplyTo string `json:"reply_to,omitempty"`
}
