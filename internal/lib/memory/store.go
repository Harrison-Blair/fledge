package memory

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// IndexName is the generated index beside the memory files.
const IndexName = "MEMORY.md"

// Dir is the memories directory of the primary checkout of the repository
// containing cwd. It may not exist yet.
func Dir(ctx context.Context, cwd string) (string, error) {
	root, err := fledgedir.Root(ctx, cwd)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".fledge", "memories"), nil
}

// List reads every memory in dir, ordered by name. A missing dir has none.
// A malformed memory file fails the listing, naming the file.
func List(dir string) ([]Memory, error) {
	ms := []Memory{}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return ms, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok || e.Name() == IndexName || !e.Type().IsRegular() {
			continue
		}
		m, err := read(dir, name)
		if err != nil {
			return nil, err
		}
		ms = append(ms, m)
	}
	return ms, nil
}

// Get reads the memory named name from dir.
func Get(dir, name string) (Memory, error) {
	if err := ValidateName(name); err != nil {
		return Memory{}, err
	}
	return read(dir, name)
}

func read(dir, name string) (Memory, error) {
	path := filepath.Join(dir, name+".md")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Memory{}, notFound(name)
	}
	if err != nil {
		return Memory{}, err
	}
	m, err := Parse(name, data)
	if err != nil {
		return Memory{}, fmt.Errorf("memory file %s: %w", path, err)
	}
	return m, nil
}

// Add validates m and stores it as a new memory in the primary checkout,
// failing with memory_exists rather than replacing one. The write and the
// index regeneration share the state store lock, so concurrent adds never
// lose index lines.
func Add(ctx context.Context, cwd string, m Memory, out *libagent.Outcome) error {
	if err := Validate(m); err != nil {
		return err
	}
	return locked(ctx, cwd, out, func(dir string) error {
		path := filepath.Join(dir, m.Name+".md")
		err := state.WriteExclusive(path, Format(m))
		if errors.Is(err, fs.ErrExist) {
			return &herdr.Error{Code: "memory_exists", Message: fmt.Sprintf("memory %s already exists; remove it first to replace it", m.Name)}
		}
		if err != nil {
			return err
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "memory", Path: path})
		return writeIndex(dir, out)
	})
}

// Remove deletes the memory named name and regenerates the index under the
// state store lock.
func Remove(ctx context.Context, cwd, name string, out *libagent.Outcome) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	return locked(ctx, cwd, out, func(dir string) error {
		path := filepath.Join(dir, name+".md")
		err := os.Remove(path)
		if errors.Is(err, fs.ErrNotExist) {
			return notFound(name)
		}
		if err != nil {
			return err
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "removed", Kind: "memory", Path: path})
		return writeIndex(dir, out)
	})
}

// locked prepares the primary checkout's .fledge and memories directories and
// runs fn with the memories directory while holding the state store lock.
func locked(ctx context.Context, cwd string, out *libagent.Outcome, fn func(dir string) error) error {
	root, err := fledgedir.Root(ctx, cwd)
	if err != nil {
		return err
	}
	managed, err := fledgedir.Ensure(root, out)
	if err != nil {
		return err
	}
	s, err := state.Open(filepath.Join(managed, "state"))
	if err != nil {
		return err
	}
	dir := filepath.Join(managed, "memories")
	if err := fledgedir.MakeParents(root, dir, out); err != nil {
		return err
	}
	return s.Exclusive(func(*state.Tx) error { return fn(dir) })
}

// indexStep runs between writeIndex's listing and its write so tests can
// widen the window a concurrent writer would race through without the lock.
var indexStep = func() {}

// writeIndex regenerates the index from the memory files in dir.
func writeIndex(dir string, out *libagent.Outcome) error {
	ms, err := List(dir)
	if err != nil {
		return err
	}
	indexStep()
	path := filepath.Join(dir, IndexName)
	if err := state.WriteReplace(path, []byte(Index(ms))); err != nil {
		return err
	}
	out.Effects = append(out.Effects, libagent.Effect{Action: "updated", Kind: "file", Path: path})
	return nil
}

func notFound(name string) error {
	return &herdr.Error{Code: "memory_not_found", Message: fmt.Sprintf("no memory named %s", name)}
}

// Phase is the outcome phase of an error from this package: memory for a
// missing or existing memory, state otherwise.
func Phase(err error) string {
	var known *herdr.Error
	if errors.As(err, &known) {
		return "memory"
	}
	return "state"
}
