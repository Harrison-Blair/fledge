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

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

//go:embed builtin/*.toml
var assets embed.FS

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// Profile is a resolved launch configuration. Reads lists repository files the
// agent reads first; Protocol renders the shared Fledge protocol block. Source
// is "builtin" or "repo"; Path names a repository file, and Base the built-in
// it inherits from.
type Profile struct {
	Name     string   `json:"name"`
	Harness  string   `json:"harness"`
	Model    string   `json:"model"`
	Args     []string `json:"args"`
	Reads    []string `json:"reads"`
	Protocol bool     `json:"protocol"`
	Sections Sections `json:"sections"`
	Source   string   `json:"source"`
	Path     *string  `json:"path"`
	Base     *string  `json:"base"`
}

// Sections are a profile's brief texts in render order. Protocol is the
// role's addendum under the shared Fledge protocol block.
type Sections struct {
	Mission  string `json:"mission"`
	Workflow string `json:"workflow"`
	Always   string `json:"always"`
	Never    string `json:"never"`
	Protocol string `json:"protocol"`
	Report   string `json:"report"`
}

// field returns the section named by one of sectionNames.
func (s *Sections) field(name string) *string {
	return map[string]*string{
		"mission": &s.Mission, "workflow": &s.Workflow, "always": &s.Always,
		"never": &s.Never, "protocol": &s.Protocol, "report": &s.Report,
	}[name]
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

// builtins decodes the embedded profiles through the same strict decoder as
// repository files.
func builtins() (map[string]Profile, error) {
	entries, err := assets.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	result := map[string]Profile{}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".toml")
		data, err := assets.ReadFile("builtin/" + e.Name())
		if err != nil {
			return nil, err
		}
		if result[name], err = builtinProfile(name, data); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// builtinProfile decodes one embedded profile. Built-ins cannot extend and
// must render the protocol block and a mission.
func builtinProfile(name string, data []byte) (Profile, error) {
	f, err := decode(data)
	if err == nil && f.Extends != nil {
		err = fmt.Errorf("built-in profiles cannot extend")
	}
	p := Profile{Name: name, Args: []string{}, Reads: []string{}, Source: "builtin"}.overlay(f)
	if err == nil && (!p.Protocol || p.Sections.Mission == "") {
		err = fmt.Errorf("built-in profiles need protocol = true and a mission")
	}
	if err != nil {
		return Profile{}, fmt.Errorf("built-in profile %s: %w", name, err)
	}
	return p, nil
}

// Load resolves one profile for a spawn started in cwd. A repository file in
// the invoking checkout's .fledge/profiles takes precedence over a built-in;
// an invalid file fails rather than falling back.
func Load(ctx context.Context, cwd, name string) (Profile, error) {
	if !namePattern.MatchString(name) {
		return Profile{}, libagent.Invalid("profile name must match [a-z][a-z0-9_-]{0,31}")
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
		path := filepath.Join(dir, name+".toml")
		if p, found, err := resolve(bases, name, path); found || err != nil {
			return p, err
		}
	}
	if p, ok := bases[name]; ok {
		return p, nil
	}
	return Profile{}, libagent.Invalid("unknown profile %q; list profiles with: fledge agent profiles", name)
}

// List returns every effective profile for cwd, sorted by name.
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
		name, ok := strings.CutSuffix(e.Name(), ".toml")
		if !ok {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if !namePattern.MatchString(name) {
			return nil, invalid(path, errors.New("file name must match [a-z][a-z0-9_-]{0,31}.toml"))
		}
		p, _, err := resolve(bases, name, path)
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

// resolve reads and resolves the repository file at path, reporting whether
// it exists.
func resolve(bases map[string]Profile, name, path string) (Profile, bool, error) {
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
	f, err := decode(data)
	if err != nil {
		return Profile{}, true, invalid(path, err)
	}
	base := ""
	if f.Extends != nil {
		n, ok := strings.CutPrefix(*f.Extends, "builtin:")
		if _, known := bases[n]; !ok || !known {
			return Profile{}, true, invalid(path, errors.New("extends must name a built-in profile as builtin:<name>"))
		}
		base = n
	} else if _, ok := bases[name]; ok {
		base = name
	}
	p := Profile{Args: []string{}, Reads: []string{}}
	if base != "" {
		p = bases[base]
		ref := "builtin:" + base
		p.Base = &ref
	}
	p = p.overlay(f)
	p.Name, p.Source, p.Path = name, "repo", &path
	return p, true, nil
}

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
