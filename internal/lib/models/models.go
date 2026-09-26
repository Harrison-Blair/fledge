// Package models discovers the models each harness has available locally.
package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/harness"
	"github.com/Harrison-Blair/fledge/internal/lib/harnessenv"
)

// Row is one discovered model; Name is nil when the source has no display name.
type Row struct {
	Harness string  `json:"harness"`
	Model   string  `json:"model"`
	Name    *string `json:"name"`
}

// Discovery locates models from harness caches under Home and harness commands run through Run.
type Discovery harnessenv.Env

// LocalDiscovery reads the real home directory and executes real harness commands.
func LocalDiscovery() Discovery { return Discovery(harnessenv.Local()) }

// modelSource discovers one harness's models; any error yields no rows for that harness.
// Whether a kind has a source, and whether it only reads cache files, comes from
// its harness profile's Discovery.
type modelSource struct {
	kind string
	list func(context.Context, Discovery) ([]Row, error)
}

var modelSources = []modelSource{
	{"pi", piModels},
	{"codex", codexModels},
	{"claude", claudeModels},
	{"opencode", opencodeModels},
	{"cursor", cursorModels},
}

// sourcesWhere returns, in discovery order, the kinds whose profile Discovery matches.
func sourcesWhere(match func(harness.DiscoveryKind) bool) []string {
	var kinds []string
	for _, s := range modelSources {
		if p, _ := harness.Lookup(s.kind); match(p.Discovery) {
			kinds = append(kinds, s.kind)
		}
	}
	return kinds
}

// ModelHarnesses lists the harness kinds that have a local model source, in
// discovery order. The result is a fresh slice the caller may modify.
func ModelHarnesses() []string {
	return sourcesWhere(func(d harness.DiscoveryKind) bool { return d != harness.DiscoveryNone })
}

// ReadOnlyModelHarnesses lists the harness kinds whose local model source only
// reads cache files, executing no command. The result is a fresh slice.
func ReadOnlyModelHarnesses() []string {
	return sourcesWhere(func(d harness.DiscoveryKind) bool { return d == harness.DiscoveryCache })
}

// Discover returns one harness kind's locally discovered rows and its source
// error, letting callers tell a missing harness apart from a broken cache. An
// unknown kind is an error.
func (d Discovery) Discover(ctx context.Context, kind string) ([]Row, error) {
	for _, source := range modelSources {
		if source.kind != kind {
			continue
		}
		rows, err := source.list(ctx, d)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			rows[i].Harness = source.kind
		}
		return rows, nil
	}
	return nil, fmt.Errorf("harness %q has no local model source", kind)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
func piModels(_ context.Context, d Discovery) ([]Row, error) {
	var store map[string]struct {
		Models []struct{ ID, Name string } `json:"models"`
	}
	if err := readJSON(filepath.Join(d.Home, ".pi", "agent", "models-store.json"), &store); err != nil {
		return nil, err
	}
	var rows []Row
	for provider, entry := range store {
		for _, m := range entry.Models {
			if m.ID != "" {
				rows = append(rows, Row{Model: provider + "/" + m.ID, Name: libagent.Pointer(m.Name)})
			}
		}
	}
	return rows, nil
}
func codexModels(_ context.Context, d Discovery) ([]Row, error) {
	var cache struct {
		Models []struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Visibility  string `json:"visibility"`
		} `json:"models"`
	}
	if err := readJSON(filepath.Join(d.Home, ".codex", "models_cache.json"), &cache); err != nil {
		return nil, err
	}
	var rows []Row
	for _, m := range cache.Models {
		if m.Slug != "" && m.Visibility == "list" {
			rows = append(rows, Row{Model: m.Slug, Name: libagent.Pointer(m.DisplayName)})
		}
	}
	return rows, nil
}
func claudeModels(_ context.Context, d Discovery) ([]Row, error) {
	dir := filepath.Join(d.Home, ".claude", "cache", "model-catalog")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	newest, found := "", false
	var latest int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		if modified := info.ModTime().UnixNano(); !found || modified > latest {
			newest, latest, found = filepath.Join(dir, e.Name()), modified, true
		}
	}
	if !found {
		return nil, nil
	}
	var catalog struct {
		Catalog struct {
			Config struct {
				Models []struct{ ID, Name string } `json:"models"`
			} `json:"config"`
		} `json:"catalog"`
	}
	if err := readJSON(newest, &catalog); err != nil {
		return nil, err
	}
	var rows []Row
	for _, m := range catalog.Catalog.Config.Models {
		if m.ID != "" {
			rows = append(rows, Row{Model: m.ID, Name: libagent.Pointer(m.Name)})
		}
	}
	return rows, nil
}
func lines(ctx context.Context, d Discovery, name string, args ...string) ([]string, error) {
	output, err := d.Run(ctx, name, args...)
	if err != nil {
		return nil, err
	}
	var result []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			result = append(result, line)
		}
	}
	return result, scanner.Err()
}
func opencodeModels(ctx context.Context, d Discovery) ([]Row, error) {
	output, err := lines(ctx, d, "opencode", "models")
	if err != nil {
		return nil, err
	}
	var rows []Row
	for _, line := range output {
		rows = append(rows, Row{Model: line})
	}
	return rows, nil
}
func cursorModels(ctx context.Context, d Discovery) ([]Row, error) {
	output, err := lines(ctx, d, "cursor-agent", "--list-models")
	if err != nil {
		return nil, err
	}
	var rows []Row
	for _, line := range output {
		id, label, ok := strings.Cut(line, " - ")
		if id = strings.TrimSpace(id); ok && id != "" {
			rows = append(rows, Row{Model: id, Name: libagent.Pointer(strings.TrimSpace(label))})
		}
	}
	return rows, nil
}
