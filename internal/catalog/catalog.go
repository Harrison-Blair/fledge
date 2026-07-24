// Package catalog discovers the agents the installed integrations can launch
// and writes them to .fledge/agents/fledge/catalog.json. Discovery is
// exec-and-parse only:
// fledge asks Codex and Pi what models they serve and probes Claude Code for
// native default and model-family launchers.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// Note records why an integration contributed nothing, or why one discovered
// model was dropped. Notes are operator output, not errors: a machine without
// codex still gets a pi catalog.
type Note struct {
	Integration string
	Detail      string
}

// cmdTimeout bounds each discovery exec so a hung binary cannot hang init.
const cmdTimeout = 30 * time.Second

// entry is one discovered launcher before it is named.
type entry struct {
	name        string
	integration string
	provider    string
	model       string
}

// source is one binary discovery asks. Sources run in fixed order so the
// catalog, and which collider survives a post-suffix collision, are
// deterministic.
type source struct {
	integration string
	argv        []string
	parse       func([]byte) ([]entry, error)
}

var sources = []source{
	{integration: "claude", argv: []string{"claude", "--version"}, parse: parseClaudeVersion},
	{integration: "codex", argv: []string{"codex", "debug", "models"}, parse: parseCodexModels},
	{integration: "pi", argv: []string{"pi", "--list-models"}, parse: parsePiModels},
}

// Discover asks each installed integration what it can launch and returns the
// results as named configs. A missing binary, a failed run, or unparseable
// output skips that integration with a Note. An empty map means no integration
// answered — callers must then keep any existing catalog rather than write an
// empty one over it (a broken PATH is not "nothing installed").
func Discover() (map[string]agentcfg.Config, []Note) {
	configs := map[string]agentcfg.Config{}
	var notes []Note

	for _, src := range sources {
		out, note := run(src)
		if note != nil {
			notes = append(notes, *note)
			continue
		}
		entries, err := src.parse(out)
		if err != nil {
			notes = append(notes, Note{src.integration, fmt.Sprintf("unexpected %s output: %v", strings.Join(src.argv, " "), err)})
			continue
		}
		for _, e := range entries {
			name := e.name
			if name == "" {
				name = slugName(e.model)
			}
			name += sourceSuffix(e.integration, e.provider)
			if name == "" {
				notes = append(notes, Note{src.integration, fmt.Sprintf("model %q has no usable name; dropped", e.model)})
				continue
			}
			cfg := agentcfg.Config{Integration: e.integration, Provider: e.provider, Model: e.model}
			// Validate rather than trust the derivation: a suffix from an
			// unknown provider is still only as clean as slugName made it.
			if err := cfg.ValidateProfile(name); err != nil {
				notes = append(notes, Note{src.integration, fmt.Sprintf("model %q: %v; dropped", e.model, err)})
				continue
			}
			if prev, taken := configs[name]; taken {
				notes = append(notes, Note{src.integration, fmt.Sprintf("model %q collides with %q as %s; dropped", e.model, prev.Model, name)})
				continue
			}
			configs[name] = cfg
		}
	}
	return configs, notes
}

// run executes one source, returning its stdout or the Note that excuses it.
func run(src source) ([]byte, *Note) {
	return runWithTimeout(src, cmdTimeout)
}

// runWithTimeout is run with a configurable deadline so timeout behavior can
// be covered without making tests wait for the production discovery timeout.
func runWithTimeout(src source, timeout time.Duration) ([]byte, *Note) {
	bin, err := exec.LookPath(src.argv[0])
	if err != nil {
		return nil, &Note{src.integration, src.argv[0] + " is not on PATH; skipped"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, src.argv[1:]...).Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &Note{src.integration, fmt.Sprintf("%s timed out after %s; skipped %s discovery", strings.Join(src.argv, " "), timeout, src.integration)}
		}
		return nil, &Note{src.integration, fmt.Sprintf("%s failed: %v", strings.Join(src.argv, " "), err)}
	}
	return out, nil
}

// parseClaudeVersion turns a successful `claude --version` probe into a
// model-less default plus the family aliases Claude Code supports without
// needing model enumeration.
func parseClaudeVersion(_ []byte) ([]entry, error) {
	return []entry{
		{name: "default", integration: "claude"},
		{name: "opus", integration: "claude", model: "opus"},
		{name: "fable", integration: "claude", model: "fable"},
		{name: "sonnet", integration: "claude", model: "sonnet"},
		{name: "haiku", integration: "claude", model: "haiku"},
	}, nil
}

// parseCodexModels reads `codex debug models` JSON. Models the catalog hides
// (visibility other than "list") are not spawnable choices and are dropped.
func parseCodexModels(out []byte) ([]entry, error) {
	var doc struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	var entries []entry
	for _, m := range doc.Models {
		if m.Visibility != "list" {
			continue
		}
		entries = append(entries, entry{integration: "codex", model: m.Slug})
	}
	return entries, nil
}

// parsePiModels reads the fixed-width `pi --list-models` table: a header line,
// then one row per model with provider and model in the first two columns.
// There is no JSON mode to prefer.
func parsePiModels(out []byte) ([]entry, error) {
	var entries []entry
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "provider" {
			continue
		}
		if len(fields) < 2 {
			return nil, fmt.Errorf("row %q has no model column", line)
		}
		entries = append(entries, entry{integration: "pi", provider: fields[0], model: fields[1]})
	}
	return entries, nil
}

// sourceSuffix names the source a generated entry came from. Every entry is
// suffixed — not just colliders — so a name never changes when a later re-init
// finds the same model served by a second source.
func sourceSuffix(integration, provider string) string {
	if integration == "claude" {
		return "cl"
	}
	if integration == "codex" {
		return "cx"
	}
	switch provider {
	case "openai-codex":
		return "pi"
	case "opencode":
		return "oc"
	case "opencode-go":
		return "og"
	}
	return slugName(provider)
}

// slugName reduces a model id to the lowercase alphanumerics the agent naming
// rule allows.
func slugName(model string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(model) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Write replaces root's catalog with configs. Overwriting is the contract —
// the catalog is generated state — and MarshalIndent sorts map keys, so the
// same discovery writes the same bytes.
func Write(root string, configs map[string]agentcfg.Config) error {
	idx := agentcfg.Index{
		Version:  agentcfg.IndexVersion,
		Agents:   map[string]agentcfg.AgentRecord{},
		Profiles: configs,
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	name := filepath.Join(root, scaffold.DirName, agentcfg.CatalogName)
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".catalog-*")
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
