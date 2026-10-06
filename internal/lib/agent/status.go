package agent

import "slices"

// statuses are Herdr's live agent_status values; settled is Herdr's default
// agent.wait set.
var (
	statuses = []string{"idle", "working", "blocked", "done", "unknown"}
	settled  = []string{"idle", "done", "blocked"}
)

// IsStatus reports whether s is a live Herdr agent_status value.
func IsStatus(s string) bool { return slices.Contains(statuses, s) }

// SettledStatuses returns Herdr's default agent.wait set as a fresh slice.
func SettledStatuses() []string { return slices.Clone(settled) }
