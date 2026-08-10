// Package harness discovers installed coding-agent harnesses and the models
// they make available.
package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const DefaultDiscoveryTimeout = 10 * time.Second

// Harness describes a supported coding-agent executable.
type Harness struct {
	ID          string
	Name        string
	Executable  string
	Path        string
	Description string
}

// Model describes a model offered by a harness. An empty ID on the default
// entry means that no model argument should be passed to the harness.
type Model struct {
	ID          string
	Name        string
	Provider    string
	Description string
	Default     bool
}

// Catalog is a harness's discovered model list. Model discovery is advisory:
// Models always contains the harness-default entry, and Warning explains any
// failure that prevented discovery of additional entries.
type Catalog struct {
	Models  []Model
	Warning string
}

// LookPath resolves an executable name.
type LookPath func(string) (string, error)

// Runner invokes a model-discovery command.
type Runner func(context.Context, string, ...string) ([]byte, error)

// DiscoveryOptions supplies the replaceable boundaries used by discovery.
// Zero values select the production runner, default Codex cache location, and
// DefaultDiscoveryTimeout.
type DiscoveryOptions struct {
	Runner         Runner
	CodexCachePath string
	Timeout        time.Duration
}

var supported = []Harness{
	{ID: "claude", Name: "Claude Code", Executable: "claude", Description: "Anthropic's coding agent"},
	{ID: "codex", Name: "Codex", Executable: "codex", Description: "OpenAI's coding agent"},
	{ID: "pi", Name: "Pi", Executable: "pi", Description: "Pi coding agent"},
	{ID: "opencode", Name: "OpenCode", Executable: "opencode", Description: "Provider-independent coding agent"},
}

// Installed returns supported harnesses whose executable can be found. The
// supplied resolver makes installation detection deterministic in tests.
func Installed(lookPath LookPath) []Harness {
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	installed := make([]Harness, 0, len(supported))
	for _, candidate := range supported {
		path, err := lookPath(candidate.Executable)
		if err != nil {
			continue
		}
		candidate.Path = path
		installed = append(installed, candidate)
	}
	return installed
}

// Resolve finds an installed harness by ID, display name, or executable name.
func Resolve(installed []Harness, value string) (Harness, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range installed {
		if value == candidate.ID || value == strings.ToLower(candidate.Name) ||
			value == strings.ToLower(candidate.Executable) {
			return candidate, true
		}
	}
	return Harness{}, false
}

// Discover returns the harness default followed by any models that can be
// discovered before the deadline. Discovery failure is non-fatal and is
// reported as a warning so callers can still accept a custom model ID.
func Discover(ctx context.Context, selected Harness, options DiscoveryOptions) Catalog {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultDiscoveryTimeout
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	catalog := Catalog{Models: []Model{{
		Name:        "Harness default",
		Description: "Use the harness's configured default",
		Default:     true,
	}}}

	var (
		models []Model
		err    error
	)
	commandArms := map[string]struct {
		args  []string
		parse func([]byte) ([]Model, error)
	}{
		"pi":       {[]string{"--list-models"}, ParsePiModels},
		"opencode": {[]string{"models"}, ParseOpenCodeModels},
	}
	switch {
	case selected.ID == "claude":
		models = claudeModels()
	case selected.ID == "codex":
		models, err = discoverCodex(discoveryCtx, options.CodexCachePath)
	default:
		arm, ok := commandArms[selected.ID]
		if !ok {
			err = fmt.Errorf("unsupported harness %q", selected.ID)
			break
		}
		var output []byte
		output, err = runDiscoveryCommand(discoveryCtx, options.Runner, selected, arm.args...)
		if err == nil {
			models, err = arm.parse(output)
		}
	}

	if err != nil {
		if discoveryCtx.Err() != nil {
			err = discoveryCtx.Err()
		}
		catalog.Warning = fmt.Sprintf("model discovery for %s failed: %v", harnessName(selected), err)
	} else if selected.ID != "claude" && len(models) == 0 {
		catalog.Warning = fmt.Sprintf(
			"model discovery for %s returned no models; only the harness default is available",
			harnessName(selected),
		)
	}

	catalog.Models = append(catalog.Models, normalizeAndSort(models)...)
	return catalog
}

func runDiscoveryCommand(ctx context.Context, run Runner, selected Harness, args ...string) ([]byte, error) {
	if run == nil {
		run = commandRunner
	}
	path := selected.Path
	if path == "" {
		path = selected.Executable
	}
	output, err := run(ctx, path, args...)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func commandRunner(ctx context.Context, path string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if err == nil {
		return output, nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return nil, err
	}
	return nil, fmt.Errorf("%w: %s", err, detail)
}

func claudeModels() []Model {
	return []Model{
		{ID: "fable", Name: "Fable (moving alias)", Provider: "anthropic", Description: "Moving alias for the latest Claude Fable model"},
		{ID: "haiku", Name: "Haiku (moving alias)", Provider: "anthropic", Description: "Moving alias for the latest Claude Haiku model"},
		{ID: "opus", Name: "Opus (moving alias)", Provider: "anthropic", Description: "Moving alias for the latest Claude Opus model"},
		{ID: "sonnet", Name: "Sonnet (moving alias)", Provider: "anthropic", Description: "Moving alias for the latest Claude Sonnet model"},
		{ID: "claude-fable-5", Name: "Claude Fable 5", Provider: "anthropic", Description: "Current canonical Claude Fable 5 model"},
		{ID: "claude-mythos-5", Name: "Claude Mythos 5", Provider: "anthropic", Description: "Glasswing-restricted current canonical Claude Mythos 5 model"},
		{ID: "claude-opus-5", Name: "Claude Opus 5", Provider: "anthropic", Description: "Current canonical Claude Opus 5 model"},
		{ID: "claude-opus-4-8", Name: "Claude Opus 4.8", Provider: "anthropic", Description: "Current canonical Claude Opus 4.8 model"},
		{ID: "claude-opus-4-7", Name: "Claude Opus 4.7", Provider: "anthropic", Description: "Current canonical Claude Opus 4.7 model"},
		{ID: "claude-opus-4-6", Name: "Claude Opus 4.6", Provider: "anthropic", Description: "Current canonical Claude Opus 4.6 model"},
		{ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Provider: "anthropic", Description: "Current canonical Claude Sonnet 5 model"},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Provider: "anthropic", Description: "Current canonical Claude Sonnet 4.6 model"},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Provider: "anthropic", Description: "Current canonical Claude Haiku 4.5 model"},
		{ID: "claude-opus-4-5", Name: "Claude Opus 4.5 (legacy)", Provider: "anthropic", Description: "Active legacy Claude Opus 4.5 model"},
		{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5 (legacy)", Provider: "anthropic", Description: "Active legacy Claude Sonnet 4.5 model"},
	}
}

func discoverCodex(ctx context.Context, cachePath string) ([]Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cachePath == "" {
		var err error
		cachePath, err = defaultCodexCachePath()
		if err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("read Codex model cache: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ParseCodexModels(data)
}

func defaultCodexCachePath() (string, error) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory: %w", err)
		}
		home = filepath.Join(userHome, ".codex")
	}
	return filepath.Join(home, "models_cache.json"), nil
}

// ParseCodexModels parses Codex's local models_cache.json payload.
func ParseCodexModels(data []byte) ([]Model, error) {
	var payload struct {
		Models []struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Description string `json:"description"`
			Visibility  string `json:"visibility"`
			Hidden      bool   `json:"hidden"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode models_cache.json: %w", err)
	}

	models := make([]Model, 0, len(payload.Models))
	for _, entry := range payload.Models {
		if entry.Slug == "" || entry.Hidden ||
			(entry.Visibility != "" && entry.Visibility != "list" && entry.Visibility != "visible") {
			continue
		}
		name := entry.DisplayName
		if name == "" {
			name = entry.Slug
		}
		models = append(models, Model{
			ID:          entry.Slug,
			Name:        name,
			Description: entry.Description,
		})
	}
	return models, nil
}

// scanModels strips ANSI escapes from each line of data, skips blanks, and
// hands every remaining line to extract, collecting the models it accepts.
func scanModels(data []byte, extract func(line string) (Model, bool)) ([]Model, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var models []Model
	for scanner.Scan() {
		line := strings.TrimSpace(stripANSI(scanner.Text()))
		if line == "" {
			continue
		}
		if model, ok := extract(line); ok {
			models = append(models, model)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return models, nil
}

// ParsePiModels parses the column-oriented output of pi --list-models.
func ParsePiModels(data []byte) ([]Model, error) {
	return scanModels(data, func(line string) (Model, bool) {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "provider ") ||
			strings.HasPrefix(lower, "no models") || strings.HasPrefix(lower, "use /login") {
			return Model{}, false
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.EqualFold(fields[0], "provider") || strings.HasSuffix(fields[0], ":") {
			return Model{}, false
		}
		provider, modelID := fields[0], fields[1]
		if provider == "" || modelID == "" {
			return Model{}, false
		}
		launchID := provider + "/" + modelID
		return Model{
			ID:          launchID,
			Name:        modelID,
			Provider:    provider,
			Description: "Pi route: " + launchID,
		}, true
	})
}

// ParseOpenCodeModels parses provider/model IDs emitted by opencode models.
func ParseOpenCodeModels(data []byte) ([]Model, error) {
	return scanModels(data, func(line string) (Model, bool) {
		if strings.HasPrefix(strings.ToLower(line), "error") {
			return Model{}, false
		}
		id := strings.Fields(line)[0]
		provider, modelID, ok := strings.Cut(id, "/")
		if !ok || provider == "" || modelID == "" {
			return Model{}, false
		}
		return Model{
			ID:          id,
			Name:        modelID,
			Provider:    provider,
			Description: "OpenCode route: " + id,
		}, true
	})
}

func normalizeAndSort(models []Model) []Model {
	seen := make(map[string]bool)
	normalized := make([]Model, 0, len(models))
	for _, model := range models {
		if model.ID == "" || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		if model.Name == "" {
			model.Name = model.ID
		}
		normalized = append(normalized, model)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		left, right := normalized[i], normalized[j]
		if comparison := compareProviders(left.Provider, right.Provider); comparison != 0 {
			return comparison < 0
		}
		return compareNames(left.Name, right.Name) < 0
	})
	return normalized
}

// ProviderName returns a human-friendly provider group name.
func ProviderName(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai-codex":
		return "OpenAI Codex"
	case "opencode-go":
		return "OpenCode Go"
	case "opencode":
		return "OpenCode Zen"
	default:
		words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(strings.TrimSpace(provider)))
		for index, word := range words {
			runes := []rune(strings.ToLower(word))
			if len(runes) > 0 {
				runes[0] = unicode.ToUpper(runes[0])
			}
			words[index] = string(runes)
		}
		return strings.Join(words, " ")
	}
}

func compareProviders(left, right string) int {
	return strings.Compare(strings.ToLower(ProviderName(left)), strings.ToLower(ProviderName(right)))
}

func compareNames(left, right string) int {
	return strings.Compare(strings.ToLower(left), strings.ToLower(right))
}

func harnessName(selected Harness) string {
	if selected.Name != "" {
		return selected.Name
	}
	if selected.ID != "" {
		return selected.ID
	}
	return "harness"
}

// csiPattern matches an ANSI CSI escape sequence: ESC '[', parameter bytes
// (0x30-0x3f), intermediate bytes (0x20-0x2f), and a final byte (0x40-0x7e).
var csiPattern = regexp.MustCompile("\x1b\\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]")

// stripANSI removes CSI escape sequences commonly emitted by model listing
// commands when they mistakenly color redirected output.
func stripANSI(value string) string {
	return csiPattern.ReplaceAllString(value, "")
}
