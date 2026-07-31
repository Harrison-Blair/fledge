package agentspawn

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
	"sort"
	"strings"
	"time"
	"unicode"
)

const discoveryTimeout = 10 * time.Second

type Harness struct {
	ID          string
	Name        string
	Executable  string
	Path        string
	Description string
}

type Model struct {
	ID          string
	Name        string
	Provider    string
	Maker       string
	Description string
	Default     bool
}

type Catalog struct {
	Models  []Model
	Warning string
}

type Runner func(context.Context, string, ...string) ([]byte, error)

var harnesses = []Harness{
	{ID: "claude", Name: "Claude Code", Executable: "claude", Description: "Anthropic's coding agent"},
	{ID: "codex", Name: "Codex", Executable: "codex", Description: "OpenAI's coding agent"},
	{ID: "opencode", Name: "OpenCode", Executable: "opencode", Description: "Provider-independent coding agent"},
	{ID: "pi", Name: "Pi", Executable: "pi", Description: "Pi coding agent"},
}

func Installed(lookPath func(string) (string, error)) []Harness {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	var installed []Harness
	for _, harness := range harnesses {
		path, err := lookPath(harness.Executable)
		if err != nil {
			continue
		}
		harness.Path = path
		installed = append(installed, harness)
	}
	return installed
}

func Resolve(installed []Harness, value string) (Harness, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, harness := range installed {
		if value == harness.ID || value == strings.ToLower(harness.Name) ||
			value == strings.ToLower(harness.Executable) {
			return harness, true
		}
	}
	return Harness{}, false
}

func Discover(ctx context.Context, harness Harness, run Runner) Catalog {
	if run == nil {
		run = commandRunner
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	catalog := Catalog{Models: []Model{{
		Name: "Harness default", Description: "Use the harness's configured default", Default: true,
	}}}
	var models []Model
	var err error
	switch harness.ID {
	case "claude":
		models = []Model{
			{ID: "haiku", Name: "haiku", Maker: "Claude", Description: "Stable Claude Haiku alias"},
			{ID: "opus", Name: "opus", Maker: "Claude", Description: "Stable Claude Opus alias"},
			{ID: "sonnet", Name: "sonnet", Maker: "Claude", Description: "Stable Claude Sonnet alias"},
		}
	case "codex":
		models, err = discoverCodex(discoveryCtx)
	case "pi":
		var out []byte
		out, err = run(discoveryCtx, harness.Path, "--list-models")
		if err == nil {
			models, err = ParsePiModels(out)
		}
	case "opencode":
		var out []byte
		out, err = run(discoveryCtx, harness.Path, "models")
		if err == nil {
			models, err = ParseOpenCodeModels(out)
		}
	default:
		err = fmt.Errorf("unsupported harness %q", harness.ID)
	}
	if err != nil {
		if discoveryCtx.Err() != nil {
			err = discoveryCtx.Err()
		}
		catalog.Warning = fmt.Sprintf("model discovery for %s failed: %v", harness.Name, err)
	} else if harness.ID != "claude" && len(models) == 0 {
		catalog.Warning = fmt.Sprintf("model discovery for %s returned no models; only the harness default is available", harness.Name)
	}
	catalog.Models = append(catalog.Models, normalizeAndSort(models)...)
	return catalog
}

func commandRunner(ctx context.Context, path string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
		return nil, err
	}
	return out, nil
}

func discoverCodex(ctx context.Context) ([]Model, error) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		home = filepath.Join(userHome, ".codex")
	}
	file, err := os.Open(filepath.Join(home, "models_cache.json"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	type readResult struct {
		data []byte
		err  error
	}
	read := make(chan readResult, 1)
	go func() {
		data, readErr := io.ReadAll(file)
		read <- readResult{data: data, err: readErr}
	}()
	var data []byte
	select {
	case <-ctx.Done():
		_ = file.Close()
		return nil, ctx.Err()
	case result := <-read:
		if result.err != nil {
			return nil, result.err
		}
		data = result.data
	}
	return ParseCodexModels(data)
}

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
	var models []Model
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
			ID: entry.Slug, Name: name, Maker: makerFor("", entry.Slug), Description: entry.Description,
		})
	}
	return models, nil
}

func ParsePiModels(data []byte) ([]Model, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var models []Model
	for scanner.Scan() {
		line := strings.TrimSpace(stripANSI(scanner.Text()))
		if line == "" || strings.HasPrefix(strings.ToLower(line), "provider ") ||
			strings.HasPrefix(line, "No models") || strings.HasPrefix(line, "Use /login") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "provider" {
			continue
		}
		provider, id := fields[0], fields[1]
		if strings.HasSuffix(provider, ":") || id == "" {
			continue
		}
		launchID := provider + "/" + id
		models = append(models, Model{
			ID: launchID, Name: id, Provider: provider, Maker: makerFor(provider, id),
			Description: "Pi route: " + launchID,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return models, nil
}

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
			ID: id, Name: modelID, Maker: makerFor(provider, modelID),
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
	out := make([]Model, 0, len(models))
	for _, model := range models {
		if model.ID == "" || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		if model.Name == "" {
			model.Name = model.ID
		}
		if model.Maker == "" {
			model.Maker = makerFor("", model.ID)
		}
		out = append(out, model)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Provider != "" || out[j].Provider != "" {
			if comparison := compareProviders(out[i].Provider, out[j].Provider); comparison != 0 {
				return comparison < 0
			}
			if providerUsesCreatorGroups(out[i].Provider) {
				if comparison := compareMakers(out[i].Maker, out[j].Maker); comparison != 0 {
					return comparison < 0
				}
			}
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		if out[i].Maker != out[j].Maker {
			return strings.ToLower(out[i].Maker) < strings.ToLower(out[j].Maker)
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func makerFor(provider, model string) string {
	value := strings.ToLower(model)
	if !providerUsesCreatorGroups(provider) {
		value = strings.ToLower(provider + " " + model)
	}
	for _, match := range []struct {
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
	} {
		for _, key := range match.keys {
			if strings.Contains(value, key) {
				return match.maker
			}
		}
	}
	if providerUsesCreatorGroups(provider) {
		return "Other"
	}
	if provider == "" {
		return "Other"
	}
	return normalizeProviderName(provider)
}

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

func ProviderUsesCreatorGroups(provider string) bool {
	return providerUsesCreatorGroups(provider)
}

func providerUsesCreatorGroups(provider string) bool {
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

func stripANSI(value string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range value {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if unicode.IsLetter(r) {
				inEscape = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
