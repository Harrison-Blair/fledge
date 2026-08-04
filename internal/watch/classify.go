// Package watch supervises Fledge workers: it classifies the status lines
// workers append to their status files, decodes Herdr's agent status events,
// and reads the watcher's project-local configuration.
package watch

import (
	"fmt"
	"strings"
)

// Status verbs workers are taught to prefix their status lines with. Matching
// is exact and lower case: the spawn prompt dictates the vocabulary, so a
// differently spelled verb is treated as prose rather than a signal.
const (
	verbWorking       = "working"
	verbDone          = "done"
	verbNeedsDecision = "needs-decision"
	verbBlocked       = "blocked"
	verbFailed        = "failed"
	verbPaused        = "paused"
)

// Herdr agent statuses the watcher reacts to.
const (
	statusBlocked = "blocked"
	statusWorking = "working"
)

const (
	maxWakeReasons   = 20
	maxWakeBodyBytes = 4096
)

// Action is the watcher's verdict for one worker status line.
type Action int

const (
	// ActionIgnore leaves the line alone; it carries no supervision signal.
	ActionIgnore Action = iota
	// ActionWake escalates to the orchestrator immediately.
	ActionWake
	// ActionWakeAfterGrace escalates only if the worker's own completion
	// message never arrives within the done grace window.
	ActionWakeAfterGrace
	// ActionAbsorb records progress without waking anybody.
	ActionAbsorb
)

// TransitionAction is the watcher's verdict for one Herdr agent status edge.
type TransitionAction int

const (
	// TransitionIgnore leaves the escalation state untouched.
	TransitionIgnore TransitionAction = iota
	// TransitionWake escalates to the orchestrator immediately.
	TransitionWake
	// TransitionClear re-arms the pane's escalation state.
	TransitionClear
)

// ParseStatusLine splits a worker status line into its leading verb and the
// detail that follows. A line qualifies only when a single whitespace-free
// token precedes the first colon, so prose that merely mentions a verb
// ("I am working: yes") is not a status line.
func ParseStatusLine(line string) (verb, detail string, ok bool) {
	verb, detail, found := strings.Cut(strings.TrimSpace(line), ":")
	if !found || verb == "" || strings.ContainsAny(verb, " \t") {
		return "", "", false
	}

	return verb, strings.TrimSpace(detail), true
}

// ClassifyStatus maps a worker status verb to the watcher's action. Unknown
// verbs are ignored so an unfamiliar vocabulary never wakes the orchestrator.
func ClassifyStatus(verb string) Action {
	switch verb {
	case verbBlocked, verbNeedsDecision, verbFailed:
		return ActionWake
	case verbWorking, verbPaused:
		return ActionAbsorb
	case verbDone:
		return ActionWakeAfterGrace
	default:
		return ActionIgnore
	}
}

// ClassifyTransition maps a Herdr agent status edge to the watcher's action.
// Anything but blocked and working — including idle, done and statuses newer
// Herdr releases may add — leaves the pane's escalation state alone.
func ClassifyTransition(status string) TransitionAction {
	switch status {
	case statusBlocked:
		return TransitionWake
	case statusWorking:
		return TransitionClear
	default:
		return TransitionIgnore
	}
}

// ComposeWakeBody renders batched wake reasons into one orchestrator message
// body. The batch is capped at 20 reasons and 4096 bytes; whatever does not
// fit is counted in a trailing notice and stays readable in the wake ledger.
func ComposeWakeBody(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}

	kept := min(len(reasons), maxWakeReasons)
	for ; kept > 0; kept-- {
		if body := renderWakeBody(reasons, kept); len(body) <= maxWakeBodyBytes {
			return body
		}
	}

	return renderWakeBody(reasons, 0)
}

func renderWakeBody(reasons []string, kept int) string {
	var body strings.Builder

	if len(reasons) == 1 {
		body.WriteString("Watcher: 1 worker event needs attention:\n")
	} else {
		fmt.Fprintf(&body, "Watcher: %d worker events need attention:\n", len(reasons))
	}
	for _, reason := range reasons[:kept] {
		fmt.Fprintf(&body, "- %s\n", reason)
	}
	if remaining := len(reasons) - kept; remaining > 0 {
		fmt.Fprintf(&body, "+%d more in the watch ledger\n", remaining)
	}
	body.WriteString("Check each worker (fledge agent message send <name> <text>) or inspect its pane.\n")
	body.WriteString("Automated watcher notification — do not reply to this message ID.")

	return body.String()
}
