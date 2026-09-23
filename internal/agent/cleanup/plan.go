package cleanup

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/worktree"
)

// planned is a cleanup plan and what executing it needs.
type planned struct {
	Result
	store *state.Store
	root  string
}

// plan selects the caller's workers and checkouts and decides each one. It
// never writes: the caller is found by terminal without relocating its
// record, and records, tasks, and checkouts are only read.
func plan(ctx context.Context, c libagent.Client, o Options) (planned, error) {
	s, err := identity.Existing(ctx, c.Cwd)
	if err != nil {
		return planned{}, libagent.AtPhase("state", err)
	}
	caller, err := callerRecord(ctx, c, s)
	if err != nil {
		return planned{}, err
	}
	children, err := identity.Children(s, caller.ID)
	var live map[string]identity.Record
	if err == nil {
		live, err = identity.LiveByTerminal(s)
	}
	var tasks []task.Record
	if err == nil {
		tasks, err = task.List(s)
	}
	if err != nil {
		return planned{}, libagent.AtPhase("state", err)
	}
	agents, err := c.List(ctx)
	if err != nil {
		return planned{}, err
	}
	p := planned{Result: Result{DryRun: o.DryRun, Caller: caller.ID, Checkouts: []Checkout{}}, store: s}
	p.Workers = selectWorkers(caller.ID, children, agents, live, tasks, o.ResultsCollected)
	if !slices.ContainsFunc(children, func(r identity.Record) bool { return spawnedBy(caller.ID, r) && r.WorktreePath != nil }) {
		return p, nil
	}
	listing, err := worktree.ListCheckouts(ctx, c, c.Cwd)
	if err != nil {
		return planned{}, err
	}
	p.root = listing.Root
	p.Checkouts = planCheckouts(ctx, caller.ID, children, p.Workers, agents, live, listing)
	return p, nil
}

// callerRecord returns the live record of the agent in the caller's pane,
// matched by terminal, or fails with caller_unregistered.
func callerRecord(ctx context.Context, c libagent.Client, s *state.Store) (identity.Record, error) {
	unregistered := libagent.AtPhase("identity", &herdr.Error{Code: "caller_unregistered", Message: "the caller has no live Fledge record; register with fledge agent adopt"})
	if c.CallerPane == "" {
		return identity.Record{}, unregistered
	}
	a, err := c.Get(ctx, c.CallerPane)
	var remote *herdr.Error
	if errors.As(err, &remote) && remote.Code == "agent_not_found" {
		return identity.Record{}, unregistered
	}
	if err != nil {
		return identity.Record{}, err
	}
	var rec *identity.Record
	if s != nil {
		if rec, err = identity.Registered(s, a); err != nil {
			return identity.Record{}, libagent.AtPhase("state", err)
		}
	}
	if rec == nil {
		return identity.Record{}, unregistered
	}
	return *rec, nil
}

// spawnedBy reports whether rec is a direct worker that caller spawned.
func spawnedBy(caller string, rec identity.Record) bool {
	return rec.ID != caller && rec.Parent != nil && *rec.Parent == caller && rec.RegisteredBy == "spawn"
}

// selectWorkers returns caller's direct spawned workers whose records are
// live, in records order, each planned for stopping or skipped with the
// reasons it is held. live is a LiveByTerminal map.
func selectWorkers(caller string, records []identity.Record, agents []herdr.AgentDetails, live map[string]identity.Record, tasks []task.Record, collected bool) []Worker {
	workers := []Worker{}
	for _, rec := range records {
		if !spawnedBy(caller, rec) || rec.EndedAt != nil {
			continue
		}
		w := Worker{ID: rec.ID, Name: rec.Name, Outcome: "planned"}
		var reasons []string
		if a, ok := running(agents, rec); !ok {
			reasons = append(reasons, "no live Herdr agent hosts its terminal")
		} else {
			if a.Name != nil {
				w.Name = a.Name
			}
			w.PaneID, w.AgentStatus = &a.PaneID, &a.AgentStatus
			if a.AgentStatus != "idle" && a.AgentStatus != "done" {
				reasons = append(reasons, "agent is "+a.AgentStatus)
			}
		}
		var descendants []string
		for _, d := range live {
			if d.Parent != nil && *d.Parent == rec.ID {
				descendants = append(descendants, "its worker "+d.ID+" is live")
			}
		}
		slices.Sort(descendants)
		reasons = append(reasons, descendants...)
		if r := taskHold(tasks, rec.ID, collected); r != "" {
			reasons = append(reasons, r)
		}
		if len(reasons) > 0 {
			w.Outcome, w.Reason = "skipped", libagent.Pointer(strings.Join(reasons, "; "))
		}
		workers = append(workers, w)
	}
	return workers
}

// running returns the live agent hosting rec's terminal with rec's harness.
func running(agents []herdr.AgentDetails, rec identity.Record) (herdr.AgentDetails, bool) {
	for _, a := range agents {
		if a.TerminalID == rec.TerminalID && !identity.Mismatched(rec, a) {
			return a, true
		}
	}
	return herdr.AgentDetails{}, false
}

// taskHold explains why the tasks owned by owner hold it, or returns "": any
// task not verified or cancelled holds it, whoever created the task, and
// owning none holds it unless the caller collected its results.
func taskHold(tasks []task.Record, owner string, collected bool) string {
	var owned bool
	var open []string
	for _, t := range tasks {
		if t.Owner == nil || *t.Owner != owner {
			continue
		}
		owned = true
		if t.Status != task.Verified && t.Status != task.Cancelled {
			open = append(open, "task "+t.ID+" is "+t.Status)
		}
	}
	switch {
	case len(open) > 0:
		return strings.Join(open, "; ")
	case !owned && !collected:
		return "it owns no task; pass --results-collected once its results are read"
	}
	return ""
}

// planCheckouts decides each checkout still listed that one of caller's
// spawned workers recorded. Only a managed checkout the worker's spawn created
// from a recorded base is planned for removal, and only when its worker is
// ended or being stopped, it is clean and merged into that base, and no live
// agent other than the workers being stopped uses it. A checkout recorded by
// several workers is decided once, under the worker that created it.
func planCheckouts(ctx context.Context, caller string, records []identity.Record, workers []Worker, agents []herdr.AgentDetails, live map[string]identity.Record, listing worktree.Checkouts) []Checkout {
	outcomes := map[string]string{}
	for _, w := range workers {
		outcomes[w.ID] = w.Outcome
	}
	var others []herdr.AgentDetails
	for _, a := range agents {
		if rec, ok := identity.Attributed(live, a); !ok || outcomes[rec.ID] != "planned" {
			others = append(others, a)
		}
	}
	creatorsFirst := slices.Clone(records)
	slices.SortStableFunc(creatorsFirst, func(a, b identity.Record) int {
		switch {
		case a.WorktreeCreated == b.WorktreeCreated:
			return 0
		case a.WorktreeCreated:
			return -1
		}
		return 1
	})
	managed := worktree.Canonical(filepath.Join(listing.Root, ".fledge", "worktrees"))
	checkouts, seen := []Checkout{}, map[string]bool{}
	for _, rec := range creatorsFirst {
		if !spawnedBy(caller, rec) || rec.WorktreePath == nil {
			continue
		}
		path := worktree.Canonical(*rec.WorktreePath)
		i := slices.IndexFunc(listing.Worktrees, func(w herdr.Worktree) bool { return w.Path == path })
		if i < 0 || seen[path] {
			continue
		}
		seen[path] = true
		row := listing.Worktrees[i]
		c := Checkout{Path: row.Path, Branch: row.Branch, Base: rec.WorktreeBase, Worker: rec.ID, Outcome: "planned"}
		if reasons := checkoutHold(ctx, rec, row, listing.Root, managed, outcomes, others, live); len(reasons) > 0 {
			c.Outcome, c.Reason = "skipped", libagent.Pointer(strings.Join(reasons, "; "))
		}
		checkouts = append(checkouts, c)
	}
	return checkouts
}

// checkoutHold explains why checkout row, recorded by rec, is kept, or
// returns none. outcomes maps each worker with a live record to its planned
// outcome; others are the live agents that are not being stopped.
func checkoutHold(ctx context.Context, rec identity.Record, row herdr.Worktree, root, managed string, outcomes map[string]string, others []herdr.AgentDetails, live map[string]identity.Record) []string {
	rel, err := filepath.Rel(managed, row.Path)
	switch {
	case !rec.WorktreeCreated:
		return []string{"not created by its worker's spawn; remove it with fledge worktree remove once it is no longer needed"}
	case row.Path == root:
		return []string{"primary checkout"}
	case err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)):
		return []string{"not a managed checkout under .fledge/worktrees; remove it with fledge worktree remove once it is no longer needed"}
	case rec.WorktreeBase == nil:
		return []string{"no recorded base branch; check it and remove it with fledge worktree remove"}
	}
	var reasons []string
	if outcome, ok := outcomes[rec.ID]; ok && outcome != "planned" {
		reasons = append(reasons, "its worker "+rec.ID+" is held")
	}
	dirty, merged := worktree.State(ctx, root, *rec.WorktreeBase, row)
	if dirty != "no" {
		reasons = append(reasons, "dirty: "+dirty)
	}
	if merged != "yes" {
		reasons = append(reasons, "merged into "+*rec.WorktreeBase+": "+merged)
	}
	if user := worktree.User(others, live, row); user != "" {
		reasons = append(reasons, user)
	}
	return reasons
}
