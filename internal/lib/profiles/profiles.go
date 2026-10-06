package profiles

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/brief"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
)

//go:embed builtin/*.md
var assets embed.FS

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// Profile is a named role brief. Role is its Markdown text exactly as
// written. Source is "builtin" or "repo"; Path names a repository file.
type Profile struct {
	Name   string  `json:"name"`
	Source string  `json:"source"`
	Path   *string `json:"path"`
	Role   string  `json:"-"`
}

// MarshalJSON adds the rendered brief, so JSON shows exactly what the
// profile renders.
func (p Profile) MarshalJSON() ([]byte, error) {
	type plain Profile
	return json.Marshal(struct {
		plain
		Brief string `json:"brief"`
	}{plain(p), p.Brief()})
}

// builtins reads the embedded role briefs; protocol.md is the shared block,
// not a role.
func builtins() (map[string]Profile, error) {
	entries, err := assets.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	result := map[string]Profile{}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if name == "protocol" {
			continue
		}
		data, err := assets.ReadFile("builtin/" + e.Name())
		if err != nil {
			return nil, err
		}
		if err := brief.CheckText(string(data)); err != nil {
			return nil, fmt.Errorf("built-in profile %s: %w", name, err)
		}
		result[name] = Profile{Name: name, Source: "builtin", Role: string(data)}
	}
	return result, nil
}

// Load resolves one profile for a spawn started in cwd. A repository file in
// the invoking checkout's .fledge/profiles replaces a same-name built-in; an
// invalid or legacy file fails rather than falling back.
func Load(ctx context.Context, cwd, name string) (Profile, error) {
	if !namePattern.MatchString(name) {
		return Profile{}, cli.Invalid("profile name must match [a-z][a-z0-9_-]{0,31}")
	}
	bases, err := builtins()
	if err != nil {
		return Profile{}, err
	}
	dir, err := repoDir(ctx, cwd)
	if err != nil {
		return Profile{}, err
	}
	if dir != "" {
		if err := legacy(dir, name); err != nil {
			return Profile{}, err
		}
		if p, found, err := read(name, filepath.Join(dir, name+".md")); found || err != nil {
			return p, err
		}
	}
	if p, ok := bases[name]; ok {
		return p, nil
	}
	return Profile{}, cli.Invalid("unknown profile %q; list profiles with: fledge agent profiles", name)
}

// List returns every effective profile for cwd, sorted by name. Any legacy
// TOML profile in the repository fails the listing.
func List(ctx context.Context, cwd string) ([]Profile, error) {
	bases, err := builtins()
	if err != nil {
		return nil, err
	}
	effective := maps.Clone(bases)
	dir, err := repoDir(ctx, cwd)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if dir != "" && err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".toml"); ok {
			return nil, legacyError(dir, name)
		}
	}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if !namePattern.MatchString(name) {
			return nil, invalid(path, errors.New("file name must match [a-z][a-z0-9_-]{0,31}.md"))
		}
		p, _, err := read(name, path)
		if err != nil {
			return nil, err
		}
		effective[name] = p
	}
	list := []Profile{}
	for _, name := range slices.Sorted(maps.Keys(effective)) {
		list = append(list, effective[name])
	}
	return list, nil
}

// legacy fails when dir holds a TOML profile for name.
func legacy(dir, name string) error {
	if _, err := os.Lstat(filepath.Join(dir, name+".toml")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return legacyError(dir, name)
}

// legacyError explains how to migrate the TOML profile for name in dir.
func legacyError(dir, name string) error {
	path := filepath.Join(dir, name+".toml")
	md := filepath.Join(dir, name+".md")
	conflict := ""
	if _, err := os.Lstat(md); err == nil {
		conflict = fmt.Sprintf(" and conflicts with %s", md)
	}
	return cli.Invalid("profile %s is a legacy TOML profile%s; profiles are now plain Markdown instructions: move its brief text into %s, pass launch settings to agent spawn (--harness, --model, native arguments after --), and delete the TOML file", path, conflict, md)
}

// read reads the repository profile at path, reporting whether it exists.
func read(name, path string) (Profile, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, true, err
	}
	if !info.Mode().IsRegular() {
		return Profile{}, true, invalid(path, errors.New("must be a regular file"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, true, err
	}
	if err := brief.CheckText(string(data)); err != nil {
		return Profile{}, true, invalid(path, err)
	}
	return Profile{Name: name, Source: "repo", Path: &path, Role: string(data)}, true, nil
}

// invalid reports a profile file problem as invalid input naming the file.
func invalid(path string, err error) error { return cli.Invalid("profile %s: %v", path, err) }

// repoDir returns the profile directory of the Git checkout containing cwd,
// or "" outside a checkout. It never creates anything.
func repoDir(ctx context.Context, cwd string) (string, error) {
	b, err := exec.CommandContext(ctx, "git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve repository: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(b)), ".fledge", "profiles"), nil
}
