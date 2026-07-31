package fledge

// Agent lifecycle state names. These values are wire- and CLI-visible: they
// appear in Herdr requests, JSON output, and `--until` arguments, so they must
// stay byte-stable.
const (
	StateIdle    = "idle"
	StateWorking = "working"
	StateBlocked = "blocked"
	StateDone    = "done"
	StateUnknown = "unknown"
	StateStopped = "stopped"
)

// WaitStates is the vocabulary accepted by `--until`, in the order its help
// text lists them. A stopped agent cannot be waited on, so it is absent.
var WaitStates = []string{StateIdle, StateDone, StateBlocked, StateWorking, StateUnknown}

// ReportedStates is the vocabulary and ordering of the `fledge status` agent
// counters.
var ReportedStates = []string{StateIdle, StateWorking, StateBlocked, StateDone, StateUnknown, StateStopped}

// stopSettleStates are the states gracefullyStopPane waits for before sending
// Ctrl+D; the no---until default stays server-owned.
var stopSettleStates = []string{StateIdle, StateDone, StateBlocked}
