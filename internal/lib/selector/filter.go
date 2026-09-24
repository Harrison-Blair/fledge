package selector

import (
	"slices"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// states are the live agent statuses a filter may select.
var states = []string{"idle", "working", "blocked", "done", "unknown"}

// Filter selects live agents. Fields AND together; values within one slice OR
// together. States and Harnesses compare live Herdr values; every other field
// is record-backed and matches only agents with a live record in this
// repository. Agents of other repositories share Herdr's global list and
// never have one here, so Registered keeps only this repository's agents.
type Filter struct {
	Mine       bool     // children of the caller's live record
	Parent     string   // agent record id
	States     []string // live agent_status values
	Harnesses  []string // live agent field, a documented harness kind
	Profiles   []string // recorded profile names
	Tasks      []string // task ids; matches the owner of any of them
	Worktrees  []string // paths, cleaned absolute against the process cwd
	Registered bool     // only agents with a live record
}

// Empty reports whether f selects every live agent.
func (f Filter) Empty() bool {
	return len(f.States) == 0 && len(f.Harnesses) == 0 && !f.NeedsRecords()
}

// NeedsRecords reports whether any record-backed field is set.
func (f Filter) NeedsRecords() bool {
	return f.Mine || f.Parent != "" || len(f.Profiles) > 0 || len(f.Tasks) > 0 || len(f.Worktrees) > 0 || f.Registered
}

// Validate checks f without contacting Herdr or the store.
func (f Filter) Validate() error {
	switch {
	case f.Mine && f.Parent != "":
		return libagent.Invalid("--mine and --parent are mutually exclusive")
	case f.Parent != "" && !state.ValidID(f.Parent):
		return libagent.Invalid("--parent must be an 8 lowercase hexadecimal agent id")
	case slices.ContainsFunc(f.Tasks, func(id string) bool { return !state.ValidID(id) }):
		return libagent.Invalid("--task must be an 8 lowercase hexadecimal task id")
	case slices.ContainsFunc(f.States, func(s string) bool { return !slices.Contains(states, s) }):
		return libagent.Invalid("--state must be idle, working, blocked, done, or unknown")
	case slices.ContainsFunc(f.Harnesses, func(h string) bool { return !libagent.IsHarness(h) }):
		return libagent.Invalid("--harness must be a documented Herdr harness kind")
	case slices.ContainsFunc(f.Profiles, blank):
		return libagent.Invalid("--profile must be nonempty")
	case slices.ContainsFunc(f.Worktrees, blank):
		return libagent.Invalid("--worktree must be nonempty")
	}
	return nil
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }
