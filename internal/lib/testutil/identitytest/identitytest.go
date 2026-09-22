// Package identitytest registers agent records in throwaway repositories for
// command tests. Production packages must not import it.
package identitytest

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// Repository returns a fresh Git repository with no .fledge directory.
func Repository(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, b)
	}
	return root
}

// Register records a as a spawned agent in the repository at cwd, with no
// parent, and returns its record.
func Register(t *testing.T, cwd string, a herdr.AgentDetails) identity.Record {
	t.Helper()
	s, err := identity.OpenStore(context.Background(), cwd, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := identity.Register(context.Background(), s, libagent.Client{}, a, "spawn", nil)
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// RegisterChild records a like Register, then sets its parent to parent.
func RegisterChild(t *testing.T, cwd string, a herdr.AgentDetails, parent string) identity.Record {
	t.Helper()
	rec := Register(t, cwd, a)
	s, err := identity.Existing(context.Background(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(identity.Kind, rec.ID, &rec, func() error { rec.Parent = &parent; return nil }); err != nil {
		t.Fatal(err)
	}
	return rec
}
