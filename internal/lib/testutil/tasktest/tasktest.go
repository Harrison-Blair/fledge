// Package tasktest builds agents and task records in throwaway repositories
// for task command tests. Production packages must not import it.
package tasktest

import (
	"context"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/brief"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

// Agent is a live agent_info result for the named agent in pane with terminal.
func Agent(pane, terminal, name string) herdr.AgentResult {
	p := herdrscript.LiveAgent("idle")
	p.PaneID, p.Name = pane, &name
	r := herdrscript.Info(p)
	r.Agent.TerminalID = terminal
	return r
}

// Get is the scripted agent.get of target answered by a.
func Get(target string, a herdr.AgentResult) herdrscript.Call {
	return herdrscript.Call{Method: "agent.get", Params: map[string]any{"target": target}, Result: a}
}

// Register records a as an agent in the repository at cwd.
func Register(t *testing.T, cwd string, a herdr.AgentResult) identity.Record {
	t.Helper()
	return identitytest.Register(t, cwd, a.Agent)
}

// Client serves calls for a caller in callerPane working in cwd.
func Client(t *testing.T, cwd, callerPane string, calls ...herdrscript.Call) libagent.Client {
	t.Helper()
	c := herdrscript.Client(t, calls...)
	c.Cwd, c.CallerPane = cwd, callerPane
	return c
}

// Seed stores r as a new task in the repository at cwd and returns its id.
func Seed(t *testing.T, cwd string, r task.Record) string {
	t.Helper()
	s, err := identity.OpenStore(context.Background(), cwd, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Create(task.Kind, func(id string) any { r.ID = id; return r })
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Load reads task id from the repository at cwd.
func Load(t *testing.T, cwd, id string) task.Record {
	t.Helper()
	s, err := task.Existing(context.Background(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	r, err := task.Get(s, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Brief is a brief that follows the template, with one line under each
// section.
func Brief() string {
	text := "Preamble line.\n"
	for _, s := range brief.Sections {
		text += "\n## " + s + "\n" + s + " text.\n"
	}
	return text
}

// Ptr returns a pointer to v.
func Ptr[T any](v T) *T { return &v }
