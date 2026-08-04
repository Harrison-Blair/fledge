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
	Maker       string
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
	switch selected.ID {
	case "claude":
		models = claudeModels()
	case "codex":
		models, err = discoverCodex(discoveryCtx, options.CodexCachePath)
	case "pi":
		var output []byte
		output, err = runDiscoveryCommand(discoveryCtx, options.Runner, selected, "--list-models")
		if err == nil {
			models, err = ParsePiModels(output)
		}
	case "opencode":
		var output []byte
		output, err = runDiscoveryCommand(discoveryCtx, options.Runner, selected, "models")
		if err == nil {
			models, err = ParseOpenCodeModels(output)
		}
	default:
		err = fmt.Errorf("unsupported harness %q", selected.ID)
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
		{ID: "haiku", Name: "haiku", Maker: "Claude", Description: "Stable Claude Haiku alias"},
		{ID: "opus", Name: "opus", Maker: "Claude", Description: "Stable Claude Opus alias"},
		{ID: "sonnet", Name: "sonnet", Maker: "Claude", Description: "Stable Claude Sonnet alias"},
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
			Maker:       makerFor("", entry.Slug),
			Description: entry.Description,
		})
	}
	return models, nil
}

// ParsePiModels parses the column-oriented output of pi --list-models.
func ParsePiModels(data []byte) ([]Model, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var models []Model
	for scanner.Scan() {
		line := strings.TrimSpace(stripANSI(scanner.Text()))
		lower := strings.ToLower(line)
		if line == "" || strings.HasPrefix(lower, "provider ") ||
			strings.HasPrefix(lower, "no models") || strings.HasPrefix(lower, "use /login") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.EqualFold(fields[0], "provider") || strings.HasSuffix(fields[0], ":") {
			continue
		}
		provider, modelID := fields[0], fields[1]
		if provider == "" || modelID == "" {
			continue
		}
		launchID := provider + "/" + modelID
		models = append(models, Model{
			ID:          launchID,
			Name:        modelID,
			Provider:    provider,
			Maker:       makerFor(provider, modelID),
			Description: "Pi route: " + launchID,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return models, nil
}

// ParseOpenCodeModels parses provider/model IDs emitted by opencode models.
func ParseOpenCodeModels(data []byte) ([]Model, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var models []Model
	for scanner.Scan() {
		line := strings.TrimSpace(stripANSI(scanner.Text()))
		if line == "" || strings.HasPrefix(strings.ToLower(line), "error") {
			continue
		}
		id := strings.Fields(line)[0]
		provider, modelID, ok := strings.Cut(id, "/")
		if !ok || provider == "" || modelID == "" {
			continue
		}
		models = append(models, Model{
			ID:          id,
			Name:        modelID,
			Provider:    provider,
			Maker:       makerFor(provider, modelID),
			Description: "OpenCode route: " + id,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return models, nil
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
		if model.Maker == "" {
			model.Maker = makerFor(model.Provider, model.ID)
		}
		normalized = append(normalized, model)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		left, right := normalized[i], normalized[j]
		if left.Provider != "" || right.Provider != "" {
			if comparison := compareProviders(left.Provider, right.Provider); comparison != 0 {
				return comparison < 0
			}
			if !ProviderUsesCreatorGroups(left.Provider) {
				return compareNames(left.Name, right.Name) < 0
			}
		}
		if comparison := compareMakers(left.Maker, right.Maker); comparison != 0 {
			return comparison < 0
		}
		return compareNames(left.Name, right.Name) < 0
	})
	return normalized
}

func makerFor(provider, model string) string {
	value := strings.ToLower(model)
	if !ProviderUsesCreatorGroups(provider) {
		value = strings.ToLower(provider + " " + model)
	}
	matches := []struct {
		keys  []string
		maker string
	}{
		{[]string{"anthropic", "claude"}, "Claude"},
		{[]string{"openai", "codex", "gpt-", "o1", "o3", "o4"}, "OpenAI"},
		{[]string{"zhipu", "zai", "glm"}, "Zhipu"},
		{[]string{"deepseek"}, "DeepSeek"},
		{[]string{"google", "gemini"}, "Google"},
		{[]string{"xai", "grok"}, "xAI"},
		{[]string{"mistral"}, "Mistral"},
		{[]string{"meta", "llama"}, "Meta"},
		{[]string{"moonshot", "kimi"}, "Moonshot"},
		{[]string{"alibaba", "qwen"}, "Alibaba"},
	}
	for _, match := range matches {
		for _, key := range match.keys {
			if strings.Contains(value, key) {
				return match.maker
			}
		}
	}
	if ProviderUsesCreatorGroups(provider) || provider == "" {
		return "Other"
	}
	return ProviderName(provider)
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
		return normalizeProviderName(provider)
	}
}

// ProviderUsesCreatorGroups reports whether a provider group should have
// separate model-maker subgroups in a picker.
func ProviderUsesCreatorGroups(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "opencode-go", "opencode":
		return true
	default:
		return false
	}
}

func compareProviders(left, right string) int {
	leftRank, leftPreferred := preferredProviderRank(left)
	rightRank, rightPreferred := preferredProviderRank(right)
	if leftPreferred != rightPreferred {
		if leftPreferred {
			return -1
		}
		return 1
	}
	if leftPreferred && leftRank != rightRank {
		return leftRank - rightRank
	}
	return strings.Compare(strings.ToLower(ProviderName(left)), strings.ToLower(ProviderName(right)))
}

func preferredProviderRank(provider string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai-codex":
		return 0, true
	case "opencode-go":
		return 1, true
	case "opencode":
		return 2, true
	default:
		return 0, false
	}
}

func compareMakers(left, right string) int {
	leftOther := strings.EqualFold(left, "Other")
	rightOther := strings.EqualFold(right, "Other")
	if leftOther != rightOther {
		if leftOther {
			return 1
		}
		return -1
	}
	return strings.Compare(strings.ToLower(left), strings.ToLower(right))
}

func compareNames(left, right string) int {
	return strings.Compare(strings.ToLower(left), strings.ToLower(right))
}

func normalizeProviderName(provider string) string {
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

func harnessName(selected Harness) string {
	if selected.Name != "" {
		return selected.Name
	}
	if selected.ID != "" {
		return selected.ID
	}
	return "harness"
}

// stripANSI removes CSI and OSC escape sequences commonly emitted by model
// listing commands when they mistakenly color redirected output.
func stripANSI(value string) string {
	var output strings.Builder
	for index := 0; index < len(value); {
		if value[index] != '\x1b' {
			output.WriteByte(value[index])
			index++
			continue
		}
		index++
		if index >= len(value) {
			break
		}
		switch value[index] {
		case '[':
			index++
			for index < len(value) {
				final := value[index]
				index++
				if final >= 0x40 && final <= 0x7e {
					break
				}
			}
		case ']':
			index++
			for index < len(value) {
				if value[index] == '\a' {
					index++
					break
				}
				if value[index] == '\x1b' && index+1 < len(value) && value[index+1] == '\\' {
					index += 2
					break
				}
				index++
			}
		default:
			index++
		}
	}
	return output.String()
}
