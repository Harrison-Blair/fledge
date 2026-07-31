package agentspawn

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestInstalledHarnessesAndResolution(t *testing.T) {
	installed := Installed(func(name string) (string, error) {
		if name == "codex" || name == "pi" {
			return "/bin/" + name, nil
		}
		return "", errors.New("missing")
	})
	if len(installed) != 2 || installed[0].ID != "codex" || installed[1].ID != "pi" {
		t.Fatalf("installed = %#v", installed)
	}
	if harness, ok := Resolve(installed, "Codex"); !ok || harness.Path != "/bin/codex" {
		t.Fatalf("resolution = %#v, %t", harness, ok)
	}
}

func TestParseCodexModelsHonorsVisibility(t *testing.T) {
	models, err := ParseCodexModels([]byte(`{"models":[
		{"slug":"gpt-5.6","display_name":"GPT 5.6","description":"frontier","visibility":"list"},
		{"slug":"internal","visibility":"hide"},
		{"slug":"hidden","visibility":"list","hidden":true}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "gpt-5.6" || models[0].Maker != "OpenAI" {
		t.Fatalf("models = %#v", models)
	}
}

func TestParsePiModelsPreservesProviderRoute(t *testing.T) {
	models, err := ParsePiModels([]byte(
		"provider    model                 context  max-out  thinking  images\n" +
			"openrouter  anthropic/claude-4  200K     64K      yes       yes\n" +
			"deepseek    deepseek-chat       64K      8K       yes       no\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "openrouter/anthropic/claude-4" ||
		models[0].Provider != "openrouter" ||
		!strings.Contains(models[0].Description, models[0].ID) {
		t.Fatalf("models = %#v", models)
	}
}

func TestPiProviderNamesAndPreferredOrdering(t *testing.T) {
	models := normalizeAndSort([]Model{
		{ID: "zeta/model", Name: "model", Provider: "zeta"},
		{ID: "opencode/model", Name: "model", Provider: "opencode"},
		{ID: "alpha/model", Name: "model", Provider: "alpha"},
		{ID: "opencode-go/model", Name: "model", Provider: "opencode-go"},
		{ID: "openai-codex/model", Name: "model", Provider: "openai-codex"},
	})
	got := make([]string, 0, len(models))
	for _, model := range models {
		got = append(got, ProviderName(model.Provider))
	}
	want := []string{"OpenAI Codex", "OpenCode Go", "OpenCode Zen", "Alpha", "Zeta"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("provider ordering = %v, want %v", got, want)
	}
}

func TestPiOpenCodeModelsGroupByCreatorWithOtherLast(t *testing.T) {
	models := normalizeAndSort([]Model{
		{ID: "opencode-go/mystery", Name: "mystery", Provider: "opencode-go", Maker: makerFor("opencode-go", "mystery")},
		{ID: "opencode-go/openai/gpt-5", Name: "openai/gpt-5", Provider: "opencode-go", Maker: makerFor("opencode-go", "openai/gpt-5")},
		{ID: "opencode-go/anthropic/claude-4", Name: "anthropic/claude-4", Provider: "opencode-go", Maker: makerFor("opencode-go", "anthropic/claude-4")},
	})
	got := make([]string, 0, len(models))
	for _, model := range models {
		got = append(got, model.Maker+":"+model.Name)
	}
	want := []string{
		"Claude:anthropic/claude-4",
		"OpenAI:openai/gpt-5",
		"Other:mystery",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("creator ordering = %v, want %v", got, want)
	}
}

func TestProviderlessModelsGroupByCreatorWithOtherLast(t *testing.T) {
	models := normalizeAndSort([]Model{
		{ID: "mystery-model", Name: "mystery-model"},
		{ID: "glm-5", Name: "glm-5"},
		{ID: "claude-4", Name: "claude-4"},
	})
	got := make([]string, 0, len(models))
	for _, model := range models {
		got = append(got, model.Maker+":"+model.Name)
	}
	want := []string{
		"Claude:claude-4",
		"Zhipu:glm-5",
		"Other:mystery-model",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("creator ordering = %v, want %v", got, want)
	}
}

func TestParseOpenCodeModelsAndMakerNormalization(t *testing.T) {
	models, err := ParseOpenCodeModels([]byte(
		"openrouter/anthropic/claude-sonnet-4\n" +
			"zhipu/glm-5\n" +
			"not-qualified\n"))
	if err != nil {
		t.Fatal(err)
	}
	sorted := normalizeAndSort(models)
	if len(sorted) != 2 || sorted[0].Maker != "Claude" || sorted[1].Maker != "Zhipu" {
		t.Fatalf("models = %#v", sorted)
	}
}

func TestDiscoverAlwaysKeepsHarnessDefaultOnFailure(t *testing.T) {
	catalog := Discover(context.Background(), Harness{ID: "pi", Name: "Pi", Path: "pi"},
		func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("boom")
		})
	if len(catalog.Models) != 1 || !catalog.Models[0].Default || catalog.Warning == "" {
		t.Fatalf("catalog = %#v", catalog)
	}
}

func TestDiscoverWarnsOnEmptyCatalog(t *testing.T) {
	catalog := Discover(context.Background(), Harness{ID: "opencode", Name: "OpenCode", Path: "opencode"},
		func(context.Context, string, ...string) ([]byte, error) {
			return []byte("\n"), nil
		})
	if len(catalog.Models) != 1 || catalog.Warning == "" {
		t.Fatalf("catalog = %#v", catalog)
	}
}

func TestDiscoveryHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	catalog := Discover(ctx, Harness{ID: "pi", Name: "Pi", Path: "pi"},
		func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
	if !strings.Contains(catalog.Warning, "context canceled") || len(catalog.Models) != 1 {
		t.Fatalf("catalog = %#v", catalog)
	}
}

func TestClaudeCatalogDefaultThenAlphabeticalAliases(t *testing.T) {
	catalog := Discover(context.Background(), Harness{ID: "claude", Name: "Claude Code"}, nil)
	got := make([]string, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		got = append(got, model.Name)
	}
	want := []string{"Harness default", "haiku", "opus", "sonnet"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ordering = %v, want %v", got, want)
	}
}
