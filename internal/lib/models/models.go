// Package models discovers the models each harness has available locally.
package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner executes a harness command and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Row is one discovered model; Name is nil when the source has no display name.
type Row struct {
	Harness string  `json:"harness"`
	Model   string  `json:"model"`
	Name    *string `json:"name"`
}

// Discovery locates models from harness caches under Home and harness commands run through Run.
type Discovery struct {
	Home string
	Run  Runner
}

// LocalDiscovery reads the real home directory and executes real harness commands.
func LocalDiscovery() Discovery {
	home, _ := os.UserHomeDir()
	return Discovery{Home: home, Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, name, args...)
		command.Stderr = io.Discard
		return command.Output()
	}}
}

// modelSource discovers one harness's models; any error yields no rows for that harness.
// readOnly marks sources that only read local cache files and never execute a command.
type modelSource struct {
	kind     string
	readOnly bool
	list     func(context.Context, Discovery) ([]Row, error)
}

var modelSources = []modelSource{
	{"pi", true, piModels},
	{"codex", true, codexModels},
	{"claude", true, claudeModels},
	{"opencode", false, opencodeModels},
	{"cursor", false, cursorModels},
}

// ModelHarnesses lists the harness kinds that have a local model source, in
// discovery order. The result is a fresh slice the caller may modify.
func ModelHarnesses() []string {
	kinds := make([]string, len(modelSources))
	for i, s := range modelSources {
		kinds[i] = s.kind
	}
	return kinds
}

// ReadOnlyModelHarnesses lists the harness kinds whose local model source only
// reads cache files, executing no command. The result is a fresh slice.
func ReadOnlyModelHarnesses() []string {
	var kinds []string
	for _, s := range modelSources {
		if s.readOnly {
			kinds = append(kinds, s.kind)
		}
	}
	return kinds
}

// Discover returns one harness kind's locally discovered rows and its source
// error, letting callers tell a missing harness apart from a broken cache. An
// unknown kind is an error.
func (d Discovery) Discover(ctx context.Context, harness string) ([]Row, error) {
	for _, source := range modelSources {
		if source.kind != harness {
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
	return nil, fmt.Errorf("harness %q has no local model source", harness)
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
				rows = append(rows, Row{Model: provider + "/" + m.ID, Name: pointer(m.Name)})
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
			rows = append(rows, Row{Model: m.Slug, Name: pointer(m.DisplayName)})
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
			rows = append(rows, Row{Model: m.ID, Name: pointer(m.Name)})
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
			rows = append(rows, Row{Model: id, Name: pointer(strings.TrimSpace(label))})
		}
	}
	return rows, nil
}
func pointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
