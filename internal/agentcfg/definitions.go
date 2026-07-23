package agentcfg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
	"github.com/goccy/go-yaml"
)

const (
	// IndexVersion is the on-disk version of every generated agent index.
	IndexVersion = 1
	// AgentsDir is the directory containing portable definitions and indexes.
	AgentsDir        = "agents"
	UserDir          = "user"
	ManagedDir       = "fledge"
	ManagedFileName  = "fledge-agents.json"
	DefinitionSuffix = ".agent.md"
)

// AgentRecord is the deterministic, prompt-free projection of a Markdown
// definition. PromptHash lets consumers notice changes without duplicating
// the authoritative Markdown body in generated JSON.
type AgentRecord struct {
	Source      string     `json:"source"`
	Description string     `json:"description"`
	Tools       []string   `json:"tools,omitempty"`
	Profile     string     `json:"profile,omitempty"`
	Workspace   *Workspace `json:"workspace,omitempty"`
	PromptHash  string     `json:"prompt_hash"`
}

// Workspace requests a dedicated Herdr workspace for an agent definition.
// It is placement metadata, independent of the profile selected at spawn.
type Workspace struct {
	Label string `json:"label" yaml:"label"`
	Tab   string `json:"tab" yaml:"tab"`
}

// Index is the versioned shape shared by user, managed, and catalog indexes.
type Index struct {
	Version  int                    `json:"version"`
	Agents   map[string]AgentRecord `json:"agents"`
	Profiles map[string]Config      `json:"profiles"`
}

// Definition is a parsed portable agent definition.
type Definition struct {
	Name        string
	Description string
	Tools       []string
	Model       string
	Profile     string
	Prompt      string
	Source      string
	Managed     bool
	Launch      Config
	Workspace   *Workspace
}

type frontMatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
	Model       string   `yaml:"model"`
	Fledge      struct {
		Profile   string         `yaml:"profile"`
		Launch    Config         `yaml:"launch"`
		Workspace *Workspace     `yaml:"workspace"`
		Worktree  map[string]any `yaml:"worktree"`
	} `yaml:"fledge"`
}

// ParseDefinition parses a portable .agent.md file. name/path namespace checks
// are performed by Synchronize, which knows whether the source is managed.
func ParseDefinition(data []byte) (Definition, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Definition{}, errors.New("missing opening YAML frontmatter delimiter")
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Definition{}, errors.New("missing closing YAML frontmatter delimiter")
	}
	after := end + len("\n---")
	if after < len(rest) && rest[after] != '\n' {
		return Definition{}, errors.New("invalid closing YAML frontmatter delimiter")
	}
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(rest[:end]), &raw); err != nil {
		return Definition{}, fmt.Errorf("parse YAML frontmatter: %w", err)
	}
	if fledge, ok := raw["fledge"].(map[string]any); ok {
		if _, exists := fledge["worktree"]; exists {
			return Definition{}, errors.New("fledge.worktree is not supported yet")
		}
	}
	var fm frontMatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return Definition{}, fmt.Errorf("parse YAML frontmatter: %w", err)
	}
	if strings.TrimSpace(fm.Name) == "" {
		return Definition{}, errors.New("frontmatter name is required")
	}
	if strings.TrimSpace(fm.Description) == "" {
		return Definition{}, errors.New("frontmatter description is required")
	}
	if err := validateWorkspace(fm.Fledge.Workspace); err != nil {
		return Definition{}, err
	}
	prompt := strings.TrimPrefix(rest[after:], "\n")
	return Definition{
		Name: fm.Name, Description: fm.Description, Tools: fm.Tools,
		Model: fm.Model, Profile: fm.Fledge.Profile, Prompt: prompt,
		Launch: fm.Fledge.Launch, Workspace: fm.Fledge.Workspace,
	}, nil
}

func validateWorkspace(workspace *Workspace) error {
	if workspace == nil {
		return nil
	}
	if strings.TrimSpace(workspace.Label) == "" {
		return errors.New("fledge.workspace.label is required")
	}
	if strings.TrimSpace(workspace.Tab) == "" {
		return errors.New("fledge.workspace.tab is required")
	}
	for field, value := range map[string]string{"label": workspace.Label, "tab": workspace.Tab} {
		if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("fledge.workspace.%s must be a single, trimmed label", field)
		}
	}
	return nil
}

// Synchronize rebuilds the user and managed indexes atomically from Markdown.
// Markdown is authoritative; generated indexes are never merged back in.
func Synchronize(root string) error {
	base := filepath.Join(root, scaffold.DirName, AgentsDir)
	userDefs, err := scanDefinitions(base, UserDir, false)
	if err != nil {
		return err
	}
	managedDefs, err := scanDefinitions(base, ManagedDir, true)
	if err != nil {
		return err
	}

	user := newIndex()
	managed := newIndex()
	for _, set := range []struct {
		defs []Definition
		idx  *Index
	}{{userDefs, &user}, {managedDefs, &managed}} {
		for _, d := range set.defs {
			if _, exists := user.Agents[d.Name]; exists {
				return fmt.Errorf("duplicate agent %q", d.Name)
			}
			if _, exists := managed.Agents[d.Name]; exists {
				return fmt.Errorf("duplicate agent %q", d.Name)
			}
			h := sha256.Sum256([]byte(d.Prompt))
			set.idx.Agents[d.Name] = AgentRecord{Source: d.Source, Description: d.Description, Tools: d.Tools, Profile: d.Profile, Workspace: cloneWorkspace(d.Workspace), PromptHash: hex.EncodeToString(h[:])}
			if d.Profile != "" && (d.Model != "" || !reflect.DeepEqual(d.Launch, Config{})) {
				cfg, err := deriveProfile(d)
				if err != nil {
					return fmt.Errorf("%s: %w", d.Source, err)
				}
				if prev, ok := set.idx.Profiles[d.Profile]; ok && !reflect.DeepEqual(prev, cfg) {
					return fmt.Errorf("profile %q has conflicting declarations", d.Profile)
				}
				set.idx.Profiles[d.Profile] = cfg
			}
		}
	}

	catalog, err := readIndex(filepath.Join(base, "catalog.json"))
	if err != nil {
		return err
	}
	resolved := map[string]Config{}
	for _, src := range []struct {
		name     string
		profiles map[string]Config
	}{
		{"user", user.Profiles}, {"managed", managed.Profiles}, {"catalog", catalog.Profiles},
	} {
		for name, cfg := range src.profiles {
			if err := cfg.ValidateProfile(name); err != nil {
				return fmt.Errorf("%s index: %w", src.name, err)
			}
			if prev, ok := resolved[name]; ok && !reflect.DeepEqual(prev, cfg) {
				return fmt.Errorf("profile %q conflicts between configured sources", name)
			}
			resolved[name] = cfg
		}
	}
	for _, d := range append(append([]Definition{}, userDefs...), managedDefs...) {
		if d.Profile != "" {
			if _, ok := resolved[d.Profile]; !ok {
				return fmt.Errorf("%s: profile %q is not configured and no routable model declares it", d.Source, d.Profile)
			}
		}
	}
	if err := writeIndexAtomic(filepath.Join(base, "agents.json"), user); err != nil {
		return err
	}
	return writeIndexAtomic(filepath.Join(base, ManagedFileName), managed)
}

func cloneWorkspace(in *Workspace) *Workspace {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func newIndex() Index {
	return Index{Version: IndexVersion, Agents: map[string]AgentRecord{}, Profiles: map[string]Config{}}
}

func scanDefinitions(base, dir string, managed bool) ([]Definition, error) {
	root := filepath.Join(base, dir)
	var defs []Definition
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), DefinitionSuffix) {
			return nil
		}
		rel, err := filepath.Rel(base, name)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 {
			return fmt.Errorf("%s: agent definitions must be <source>/<name>/<name>.agent.md", filepath.ToSlash(rel))
		}
		folder := parts[1]
		fileName := strings.TrimSuffix(parts[2], DefinitionSuffix)
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		d, err := ParseDefinition(data)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.ToSlash(rel), err)
		}
		if folder != fileName || d.Name != folder {
			return fmt.Errorf("%s: folder, filename, and frontmatter name must all be %q", filepath.ToSlash(rel), folder)
		}
		if err := validateDefinitionName(d.Name, managed); err != nil {
			return fmt.Errorf("%s: %w", filepath.ToSlash(rel), err)
		}
		d.Source, d.Managed = filepath.ToSlash(rel), managed
		defs = append(defs, d)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return defs, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs, nil
}

func validateDefinitionName(name string, managed bool) error {
	if err := validPortableName(name); err != nil {
		return err
	}
	isManaged := strings.HasPrefix(name, "fledge-")
	if managed && !isManaged {
		return fmt.Errorf("managed agent %q must use the fledge-* namespace", name)
	}
	if !managed && isManaged {
		return fmt.Errorf("user agent %q uses the reserved fledge-* namespace", name)
	}
	return nil
}

func validPortableName(name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return fmt.Errorf("invalid agent name %q: use kebab-case", name)
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return fmt.Errorf("invalid agent name %q: use kebab-case", name)
		}
	}
	return nil
}

func deriveProfile(d Definition) (Config, error) {
	if d.Profile == "" {
		return Config{}, errors.New("fledge.profile is required when declaring launch fields")
	}
	if d.Model == "" {
		return Config{}, fmt.Errorf("profile %q needs a routable model", d.Profile)
	}
	cfg := Config{}
	integration, provider, err := Route(d.Model)
	if err != nil {
		return Config{}, err
	}
	cfg.Integration, cfg.Provider, cfg.Model = integration, provider, d.Model
	mergeConfig(&cfg, d.Launch)
	if cfg.Integration != "pi" && d.Launch.Integration != "" {
		cfg.Provider = d.Launch.Provider
	}
	if err := cfg.ValidateProfile(d.Profile); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func mergeConfig(dst *Config, src Config) {
	if src.Integration != "" {
		dst.Integration = src.Integration
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.Provider != "" {
		dst.Provider = src.Provider
	}
	if src.Cwd != "" {
		dst.Cwd = src.Cwd
	}
	if src.PermissionMode != "" {
		dst.PermissionMode = src.PermissionMode
	}
	if src.Sandbox != "" {
		dst.Sandbox = src.Sandbox
	}
	if src.Argv != nil {
		dst.Argv = append([]string(nil), src.Argv...)
	}
	if src.Env != nil {
		dst.Env = cloneMap(src.Env)
	}
}

func cloneMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func readIndex(name string) (Index, error) {
	data, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return newIndex(), nil
	}
	if err != nil {
		return Index{}, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, fmt.Errorf("parse %s: %w", filepath.Base(name), err)
	}
	if idx.Version != IndexVersion {
		return Index{}, fmt.Errorf("parse %s: unsupported index version %d", filepath.Base(name), idx.Version)
	}
	if idx.Agents == nil {
		idx.Agents = map[string]AgentRecord{}
	}
	if idx.Profiles == nil {
		idx.Profiles = map[string]Config{}
	}
	return idx, nil
}

func writeIndexAtomic(name string, idx Index) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, name); err != nil {
		return err
	}
	ok = true
	return nil
}

// LoadDefinitions synchronizes and returns all configured agents, their
// resolved profiles, and authoritative prompt bodies.
func LoadDefinitions(root string) (map[string]Definition, map[string]Config, error) {
	if err := Synchronize(root); err != nil {
		return nil, nil, err
	}
	base := filepath.Join(root, scaffold.DirName, AgentsDir)
	userDefs, err := scanDefinitions(base, UserDir, false)
	if err != nil {
		return nil, nil, err
	}
	managedDefs, err := scanDefinitions(base, ManagedDir, true)
	if err != nil {
		return nil, nil, err
	}
	defs := map[string]Definition{}
	for _, d := range append(userDefs, managedDefs...) {
		defs[d.Name] = d
	}
	profiles, err := Load(root)
	if err != nil {
		return nil, nil, err
	}
	return defs, profiles, nil
}

// FindDefinition resolves a definition by configured name or source path.
func FindDefinition(root, value string) (Definition, Config, error) {
	defs, profiles, err := LoadDefinitions(root)
	if err != nil {
		return Definition{}, Config{}, err
	}
	for _, d := range defs {
		abs := filepath.Join(root, scaffold.DirName, AgentsDir, filepath.FromSlash(d.Source))
		if value == d.Name || samePath(value, abs) || filepath.ToSlash(value) == d.Source {
			var cfg Config
			if d.Profile != "" {
				cfg = profiles[d.Profile]
			}
			return d, cfg, nil
		}
	}
	return Definition{}, Config{}, fmt.Errorf("no configured agent definition %q", value)
}

func samePath(a, b string) bool {
	aa, e1 := filepath.Abs(a)
	bb, e2 := filepath.Abs(b)
	return e1 == nil && e2 == nil && aa == bb
}
