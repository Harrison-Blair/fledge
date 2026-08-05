package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/agentcontext"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/statedir"
)

// Context builds the deterministic context-usage report for the project's live
// session, persists it, and returns it. When name is non-empty the returned
// report is narrowed to that single agent; an unknown name is an error. The
// full report is always persisted before narrowing, so a per-agent query still
// refreshes the shared snapshot on disk.
func (m *Manager) Context(ctx context.Context, dir, name string) (agentcontext.Report, error) {
	root, err := project.Find(dir)
	if err != nil {
		return agentcontext.Report{}, err
	}
	if err := m.herdr.Check(); err != nil {
		return agentcontext.Report{}, err
	}
	value, found, err := readRecord(root)
	if err != nil {
		return agentcontext.Report{}, err
	}
	if !found {
		return agentcontext.Report{}, errors.New("project has no Fledge session; run fledge start first")
	}
	sessions, err := m.herdr.List(ctx)
	if err != nil {
		return agentcontext.Report{}, err
	}
	session, exists := sessionByName(sessions, value.SessionName)
	if !exists || !session.Running {
		return agentcontext.Report{}, errors.New("project's Fledge session is not running; run fledge start first")
	}
	snapshot, err := m.herdr.Snapshot(ctx, value.SessionName)
	if err != nil {
		return agentcontext.Report{}, err
	}

	report, err := m.buildAndPersistContext(ctx, root, value.SessionName, snapshot)
	if err != nil {
		return agentcontext.Report{}, err
	}
	if name == "" {
		return report, nil
	}
	for _, agent := range report.Agents {
		if agent.Name == name {
			report.Agents = []agentcontext.AgentContext{agent}
			return report, nil
		}
	}
	return agentcontext.Report{}, fmt.Errorf("no live agent named %q in the project's Fledge session", name)
}

// buildAndPersistContext derives the report from a snapshot and writes it to the
// session's context directory. Persistence failure is fatal to the explicit
// command so the caller learns the report was not saved.
func (m *Manager) buildAndPersistContext(ctx context.Context, root, session string, snapshot herdr.Snapshot) (agentcontext.Report, error) {
	home, err := m.homeDir()
	if err != nil {
		return agentcontext.Report{}, fmt.Errorf("resolve home directory: %w", err)
	}
	deps := m.contextDeps(ctx, home)
	report := agentcontext.Build(agentcontext.LiveAgents(snapshot), deps)
	if err := agentcontext.Persist(statedir.Context(root, session), report); err != nil {
		return agentcontext.Report{}, err
	}
	return report, nil
}

// refreshContext rebuilds and persists the report after a lifecycle change. It
// is best-effort: a failure warns but never fails the spawn or stop that
// triggered it, and it takes a fresh snapshot so the persisted report reflects
// the post-change layout.
func (m *Manager) refreshContext(ctx context.Context, root, session string) {
	snapshot, err := m.herdr.Snapshot(ctx, session)
	if err != nil {
		fmt.Fprintf(m.output, "Warning: could not refresh agent context usage: %v\n", err)
		return
	}
	if _, err := m.buildAndPersistContext(ctx, root, session, snapshot); err != nil {
		fmt.Fprintf(m.output, "Warning: could not refresh agent context usage: %v\n", err)
	}
}
