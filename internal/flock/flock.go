// Package flock names and locates flocks. A flock is one isolated
// orchestration session: its own daemon, journal and agent roster, living
// under .fledge/flocks/<name>. Several flocks run concurrently in one
// workspace without seeing each other. Its socket lives outside the workspace
// (see internal/daemon).
package flock

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
	"github.com/Harrison-Blair/fledge/internal/workspace"
)

// Env carries the active flock into every flock-scoped command. It is the
// only way to select a flock: there is deliberately no override flag, so a
// pane started in one flock cannot address another.
const Env = "FLEDGE_FLOCK"

// DirName is the directory holding all flock state, relative to .fledge.
const DirName = "flocks"

// MaxName bounds a flock name. The name is the socket's filename, and the
// whole socket path must fit the platform's sun_path limit.
const MaxName = 32

// Dir is the state directory for one flock.
func Dir(root, name string) string {
	return filepath.Join(root, scaffold.DirName, DirName, name)
}

// SessionName and WindowTitle are how a fledge-managed herdr session announces
// itself: without them a flock's session is indistinguishable from one an
// operator started by hand. Both are derived here so the session list and the
// window title can never disagree about them.

// SessionName is the herdr session a flock starts by default, overridable with
// fledge start --session. Herdr's session namespace is global to the server, so
// the name carries the workspace's identity as well as the flock's: without
// it, every workspace's flock1 would resolve to one shared session, and
// stopping one flock would tear down every workspace bound to it.
func SessionName(root, name string) string {
	return SessionPrefix(root) + name
}

// SessionPrefix identifies every default Fledge-managed Herdr session for one
// workspace. It lets cleanup distinguish this project's orphan session records
// from managed sessions belonging to another workspace.
func SessionPrefix(root string) string {
	return "fledge-" + workspace.Slug(root) + "-"
}

// WindowTitle is the terminal window title a flock's session carries.
func WindowTitle(name string) string {
	return "fledge · " + name
}

// Validate reports whether name is usable as a flock name. It has to work as
// both a path segment and an environment value, so it is kept to lowercase
// alphanumerics.
func Validate(name string) error {
	if name == "" {
		return fmt.Errorf("flock name is empty")
	}
	if len(name) > MaxName {
		return fmt.Errorf("flock name %q is longer than %d characters", name, MaxName)
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return fmt.Errorf("flock name %q: only lowercase letters and digits are allowed", name)
		}
	}
	return nil
}

// List returns the names of every flock with state in the workspace, sorted.
func List(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, scaffold.DirName, DirName))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Mint returns the lowest flockN name with no state yet. A minted flock is
// always fresh: naming an existing flock explicitly is what resumes one.
func Mint(root string) (string, error) {
	existing, err := List(root)
	if err != nil {
		return "", err
	}
	taken := make(map[string]bool, len(existing))
	for _, name := range existing {
		taken[name] = true
	}

	for n := 1; ; n++ {
		name := fmt.Sprintf("flock%d", n)
		if !taken[name] {
			return name, nil
		}
	}
}

// FromEnv returns the flock the calling process belongs to. Flock-scoped
// commands are useless without it, so an unset value is a hard error rather
// than a default.
func FromEnv() (string, error) {
	name := os.Getenv(Env)
	if name == "" {
		return "", fmt.Errorf("%s not set; run inside a flock session", Env)
	}
	if err := Validate(name); err != nil {
		return "", fmt.Errorf("%s: %w", Env, err)
	}
	return name, nil
}
