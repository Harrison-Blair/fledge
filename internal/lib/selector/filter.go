package selector

import (
	"slices"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/harness"
)

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
	if f.Mine && f.Parent != "" {
		return cli.Invalid("--mine and --parent are mutually exclusive")
	}
	if f.Parent != "" {
		if err := libagent.ValidateID("parent", "agent", f.Parent); err != nil {
			return err
		}
	}
	for _, id := range f.Tasks {
		if err := libagent.ValidateID("task", "task", id); err != nil {
			return err
		}
	}
	switch {
	case slices.ContainsFunc(f.States, func(s string) bool { return !libagent.IsStatus(s) }):
		return cli.Invalid("--state must be idle, working, blocked, done, or unknown")
	case slices.ContainsFunc(f.Harnesses, func(h string) bool { return !harness.IsKind(h) }):
		return cli.Invalid("--harness must be a documented Herdr harness kind")
	case slices.ContainsFunc(f.Profiles, blank):
		return cli.Invalid("--profile must be nonempty")
	case slices.ContainsFunc(f.Worktrees, blank):
		return cli.Invalid("--worktree must be nonempty")
	}
	return nil
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }
