// Package trace turns durable coordination activity into a live diagnostic
// feed. One record type carries every event the dispatcher observes or causes,
// so the daemon can store the feed as JSON lines while readers choose between
// human and machine rendering.
package trace

import "time"

// Record is one line of the trace: what happened, between whom, and what the
// dispatcher did about it.
type Record struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Origin string    `json:"origin,omitempty"`
	Target string    `json:"target,omitempty"`
	Actor  string    `json:"actor,omitempty"`
	Pane   string    `json:"pane,omitempty"`
	Ref    string    `json:"ref,omitempty"`
	Rel    string    `json:"rel,omitempty"`
	Status string    `json:"status,omitempty"`
	Body   string    `json:"body,omitempty"`
	Note   string    `json:"note,omitempty"`
}
