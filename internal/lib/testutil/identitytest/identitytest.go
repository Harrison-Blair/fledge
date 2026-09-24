// Package identitytest registers agent records in throwaway repositories for
// command tests. Production packages must not import it.
package identitytest

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

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
	return register(t, cwd, a, nil)
}

// RegisterProfile records a like Register, as spawned with profile.
func RegisterProfile(t *testing.T, cwd string, a herdr.AgentDetails, profile string) identity.Record {
	t.Helper()
	return register(t, cwd, a, &profile)
}

func register(t *testing.T, cwd string, a herdr.AgentDetails, profile *string) identity.Record {
	t.Helper()
	s, err := identity.OpenStore(context.Background(), cwd, &libagent.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := identity.Register(context.Background(), s, libagent.Client{}, a, "spawn", nil, profile)
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

// WithSession returns a carrying a Herdr-reported claude session ref value.
func WithSession(a herdr.AgentDetails, value string) herdr.AgentDetails {
	source, harness, kind := "herdr:claude", "claude", "id"
	a.AgentSession = &herdr.AgentSession{Source: &source, Agent: &harness, Kind: &kind, Value: &value}
	return a
}

// ReadOnly makes the state store of the repository at cwd refuse every write
// and returns a check that fails t unless the store is byte for byte as it
// was. It confirms the store refuses a session write to record id, so it
// skips when permissions are not enforced, as for root.
func ReadOnly(t *testing.T, cwd, id string) func() {
	t.Helper()
	root := filepath.Join(cwd, ".fledge", "state")
	snapshot := func() map[string]string {
		files := map[string]string{}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(path)
			files[path] = string(b)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	chmod := func(dirMode, fileMode fs.FileMode) {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return os.Chmod(path, dirMode)
			}
			return os.Chmod(path, fileMode)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	before := snapshot()
	chmod(0o555, 0o444)
	t.Cleanup(func() { chmod(0o755, 0o644) })
	s, err := identity.Existing(context.Background(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := identity.ObserveSession(s, id, *WithSession(herdr.AgentDetails{}, "probe").AgentSession, time.Now()); err == nil {
		t.Skip("the state store accepted a write despite read-only permissions")
	}
	return func() {
		t.Helper()
		if after := snapshot(); !reflect.DeepEqual(before, after) {
			t.Fatalf("the state store changed:\nbefore %v\nafter  %v", before, after)
		}
	}
}
