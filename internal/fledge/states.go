package fledge

// Agent lifecycle state names. These values are wire- and CLI-visible through
// Herdr requests, status output, and JSON, so they must stay byte-stable.
const (
	StateIdle    = "idle"
	StateWorking = "working"
	StateBlocked = "blocked"
	StateDone    = "done"
	StateUnknown = "unknown"
	StateStopped = "stopped"
)

// ReportedStates is the vocabulary and ordering of the `fledge status` agent
// counters.
var ReportedStates = []string{StateIdle, StateWorking, StateBlocked, StateDone, StateUnknown, StateStopped}

// stopSettleStates bound graceful shutdown before Fledge sends Ctrl+D again.
var stopSettleStates = []string{StateIdle, StateDone, StateBlocked}
