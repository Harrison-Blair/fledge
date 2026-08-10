package harness

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInstalledFiltersSupportedHarnesses(t *testing.T) {
	lookedUp := make([]string, 0, 4)
	installed := Installed(func(name string) (string, error) {
		lookedUp = append(lookedUp, name)
		if name == "claude" || name == "pi" {
			return "/tools/" + name, nil
		}
		return "", errors.New("not installed")
	})

	if want := []string{"claude", "codex", "pi", "opencode"}; !reflect.DeepEqual(lookedUp, want) {
		t.Fatalf("lookups = %v, want %v", lookedUp, want)
	}
	if len(installed) != 2 {
		t.Fatalf("Installed() = %#v, want two harnesses", installed)
	}
	if got, want := installed[0].ID, "claude"; got != want {
		t.Errorf("Installed()[0].ID = %q, want %q", got, want)
	}
	if got, want := installed[0].Path, "/tools/claude"; got != want {
		t.Errorf("Installed()[0].Path = %q, want %q", got, want)
	}
	if got, want := installed[1].ID, "pi"; got != want {
		t.Errorf("Installed()[1].ID = %q, want %q", got, want)
	}
}

func TestResolveInstalledHarness(t *testing.T) {
	installed := []Harness{{ID: "claude", Name: "Claude Code", Executable: "claude", Path: "/bin/claude"}}
	for _, value := range []string{"claude", "CLAUDE", " Claude Code "} {
		resolved, ok := Resolve(installed, value)
		if !ok || resolved.Path != "/bin/claude" {
			t.Errorf("Resolve(%q) = %#v, %t", value, resolved, ok)
		}
	}
	if resolved, ok := Resolve(installed, "codex"); ok || resolved != (Harness{}) {
		t.Errorf("Resolve(missing) = %#v, %t", resolved, ok)
	}
}

func TestDiscoverClaudeCatalogIsExact(t *testing.T) {
	runnerCalled := false
	catalog := Discover(context.Background(), Harness{ID: "claude", Name: "Claude Code"}, DiscoveryOptions{
		Runner: func(context.Context, string, ...string) ([]byte, error) {
			runnerCalled = true
			return nil, errors.New("must not run")
		},
	})

	if runnerCalled {
		t.Fatal("Claude discovery invoked Runner")
	}
	if !catalog.Models[0].Default || catalog.Models[0].Name != "Harness default" {
		t.Errorf("default model = %#v", catalog.Models[0])
	}
	want := map[string]struct {
		name        string
		description string
	}{
		"fable":             {"Fable (moving alias)", "Moving alias for the latest Claude Fable model"},
		"haiku":             {"Haiku (moving alias)", "Moving alias for the latest Claude Haiku model"},
		"opus":              {"Opus (moving alias)", "Moving alias for the latest Claude Opus model"},
		"sonnet":            {"Sonnet (moving alias)", "Moving alias for the latest Claude Sonnet model"},
		"claude-fable-5":    {"Claude Fable 5", "Current canonical Claude Fable 5 model"},
		"claude-mythos-5":   {"Claude Mythos 5", "Glasswing-restricted current canonical Claude Mythos 5 model"},
		"claude-opus-5":     {"Claude Opus 5", "Current canonical Claude Opus 5 model"},
		"claude-opus-4-8":   {"Claude Opus 4.8", "Current canonical Claude Opus 4.8 model"},
		"claude-opus-4-7":   {"Claude Opus 4.7", "Current canonical Claude Opus 4.7 model"},
		"claude-opus-4-6":   {"Claude Opus 4.6", "Current canonical Claude Opus 4.6 model"},
		"claude-sonnet-5":   {"Claude Sonnet 5", "Current canonical Claude Sonnet 5 model"},
		"claude-sonnet-4-6": {"Claude Sonnet 4.6", "Current canonical Claude Sonnet 4.6 model"},
		"claude-haiku-4-5":  {"Claude Haiku 4.5", "Current canonical Claude Haiku 4.5 model"},
		"claude-opus-4-5":   {"Claude Opus 4.5 (legacy)", "Active legacy Claude Opus 4.5 model"},
		"claude-sonnet-4-5": {"Claude Sonnet 4.5 (legacy)", "Active legacy Claude Sonnet 4.5 model"},
	}
	if got, wantCount := len(catalog.Models), len(want)+1; got != wantCount {
		t.Fatalf("Claude model count = %d, want %d: %#v", got, wantCount, catalog.Models)
	}
	for _, model := range catalog.Models[1:] {
		metadata, ok := want[model.ID]
		if !ok {
			t.Errorf("unexpected Claude model ID %q", model.ID)
			continue
		}
		if model.Name != metadata.name || model.Description != metadata.description ||
			model.Provider != "anthropic" {
			t.Errorf("Claude model %q = %#v, want name %q, description %q, anthropic provider", model.ID, model, metadata.name, metadata.description)
		}
		delete(want, model.ID)
	}
	if len(want) != 0 {
		t.Errorf("missing Claude model IDs: %v", want)
	}
	if catalog.Warning != "" {
		t.Errorf("warning = %q, want empty", catalog.Warning)
	}
}

func TestDiscoverCodexCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "models_cache.json")
	data := `{"models":[
		{"slug":"gpt-5.6","display_name":"GPT 5.6","description":"frontier","visibility":"list"},
		{"slug":"gpt-5.5","visibility":"visible"},
		{"slug":"internal","visibility":"hide"},
		{"slug":"hidden","visibility":"list","hidden":true},
		{"display_name":"missing slug"}
	]}`
	if err := os.WriteFile(cache, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	catalog := Discover(context.Background(), Harness{ID: "codex", Name: "Codex"}, DiscoveryOptions{CodexCachePath: cache})
	assertModelIDs(t, catalog.Models, []string{"", "gpt-5.6", "gpt-5.5"})
	if got, want := catalog.Models[2].Name, "gpt-5.5"; got != want {
		t.Errorf("fallback name = %q, want %q", got, want)
	}
	if catalog.Models[1].Description != "frontier" {
		t.Errorf("model metadata = %#v", catalog.Models[1])
	}
}

func TestDiscoverCodexUsesCodexHome(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, "models_cache.json")
	if err := os.WriteFile(cache, []byte(`{"models":[{"slug":"gpt-from-home"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", home)

	catalog := Discover(context.Background(), Harness{ID: "codex", Name: "Codex"}, DiscoveryOptions{})
	assertModelIDs(t, catalog.Models, []string{"", "gpt-from-home"})
}

func TestDiscoverRunsHarnessModelCommands(t *testing.T) {
	tests := []struct {
		name       string
		harness    Harness
		wantArgs   []string
		output     string
		wantModels []string
	}{
		{
			name:       "pi",
			harness:    Harness{ID: "pi", Name: "Pi", Path: "/custom/pi"},
			wantArgs:   []string{"--list-models"},
			output:     "openai-codex gpt-5.6 400K 128K yes yes\n",
			wantModels: []string{"", "openai-codex/gpt-5.6"},
		},
		{
			name:       "opencode",
			harness:    Harness{ID: "opencode", Name: "OpenCode", Path: "/custom/opencode"},
			wantArgs:   []string{"models"},
			output:     "anthropic/claude-sonnet-4\n",
			wantModels: []string{"", "anthropic/claude-sonnet-4"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotPath string
			var gotArgs []string
			catalog := Discover(context.Background(), test.harness, DiscoveryOptions{
				Runner: func(_ context.Context, path string, args ...string) ([]byte, error) {
					gotPath = path
					gotArgs = append([]string(nil), args...)
					return []byte(test.output), nil
				},
			})
			if gotPath != test.harness.Path {
				t.Errorf("runner path = %q, want %q", gotPath, test.harness.Path)
			}
			if !reflect.DeepEqual(gotArgs, test.wantArgs) {
				t.Errorf("runner args = %v, want %v", gotArgs, test.wantArgs)
			}
			assertModelIDs(t, catalog.Models, test.wantModels)
		})
	}
}

func TestDiscoverFailureKeepsHarnessDefaultAndWarns(t *testing.T) {
	tests := []struct {
		name    string
		harness Harness
		options DiscoveryOptions
		warning string
	}{
		{
			name:    "command failure",
			harness: Harness{ID: "pi", Name: "Pi", Path: "pi"},
			options: DiscoveryOptions{Runner: func(context.Context, string, ...string) ([]byte, error) {
				return nil, errors.New("credentials unavailable")
			}},
			warning: "model discovery for Pi failed: credentials unavailable",
		},
		{
			name:    "empty command output",
			harness: Harness{ID: "opencode", Name: "OpenCode", Path: "opencode"},
			options: DiscoveryOptions{Runner: func(context.Context, string, ...string) ([]byte, error) {
				return nil, nil
			}},
			warning: "returned no models",
		},
		{
			name:    "missing Codex cache",
			harness: Harness{ID: "codex", Name: "Codex"},
			options: DiscoveryOptions{CodexCachePath: filepath.Join(t.TempDir(), "missing")},
			warning: "read Codex model cache",
		},
		{
			name:    "unsupported harness",
			harness: Harness{ID: "other", Name: "Other"},
			warning: `unsupported harness "other"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := Discover(context.Background(), test.harness, test.options)
			assertModelIDs(t, catalog.Models, []string{""})
			if !catalog.Models[0].Default {
				t.Errorf("default model = %#v", catalog.Models[0])
			}
			if !strings.Contains(catalog.Warning, test.warning) {
				t.Errorf("warning = %q, want substring %q", catalog.Warning, test.warning)
			}
		})
	}
}

func TestDiscoverTimesOut(t *testing.T) {
	catalog := Discover(context.Background(), Harness{ID: "pi", Name: "Pi", Path: "pi"}, DiscoveryOptions{
		Timeout: time.Millisecond,
		Runner: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	assertModelIDs(t, catalog.Models, []string{""})
	if !strings.Contains(catalog.Warning, "context deadline exceeded") {
		t.Errorf("warning = %q, want deadline warning", catalog.Warning)
	}
}

func TestDiscoverHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	catalog := Discover(ctx, Harness{ID: "pi", Name: "Pi", Path: "pi"}, DiscoveryOptions{
		Runner: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, ctx.Err()
		},
	})
	if !strings.Contains(catalog.Warning, "context canceled") {
		t.Errorf("warning = %q, want cancellation warning", catalog.Warning)
	}
}

func TestParseCodexModelsRejectsMalformedJSON(t *testing.T) {
	models, err := ParseCodexModels([]byte(`{"models":`))
	if err == nil || !strings.Contains(err.Error(), "decode models_cache.json") {
		t.Fatalf("ParseCodexModels() = %#v, %v", models, err)
	}
}

func TestParsePiModelsPreservesCompleteRoutesAndIgnoresANSIHeaders(t *testing.T) {
	models, err := ParsePiModels([]byte(
		"\x1b[1;34mprovider model context max-out thinking images\x1b[0m\n" +
			"\x1b[32mopenrouter\x1b[0m anthropic/claude-4 200K 64K yes yes\n" +
			"opencode-go openai/gpt-5.6 400K 128K yes yes\n" +
			"opencode moonshot/kimi-k2 200K 64K yes yes\n" +
			"Use /login to authenticate\n"))
	if err != nil {
		t.Fatal(err)
	}
	assertModelIDs(t, models, []string{
		"openrouter/anthropic/claude-4",
		"opencode-go/openai/gpt-5.6",
		"opencode/moonshot/kimi-k2",
	})
	if got, want := models[0].Provider, "openrouter"; got != want {
		t.Errorf("provider = %q, want %q", got, want)
	}
}

func TestParseOpenCodeModelsPreservesIDsAndStripsANSI(t *testing.T) {
	models, err := ParseOpenCodeModels([]byte(
		"\x1b[1mopenrouter/anthropic/claude-sonnet-4\x1b[0m\n" +
			"\x1b[31mzhipu/glm-5\x1b[0m\n" +
			"not-qualified\n" +
			"Error loading ignored/provider\n"))
	if err != nil {
		t.Fatal(err)
	}
	assertModelIDs(t, models, []string{"openrouter/anthropic/claude-sonnet-4", "zhipu/glm-5"})
	if models[0].Provider != "openrouter" || models[0].Name != "anthropic/claude-sonnet-4" {
		t.Errorf("first model = %#v", models[0])
	}
}

func TestProviderNames(t *testing.T) {
	tests := []struct {
		provider string
		name     string
	}{
		{provider: "openai-codex", name: "OpenAI Codex"},
		{provider: "opencode-go", name: "OpenCode Go"},
		{provider: "opencode", name: "OpenCode Zen"},
		{provider: "some_provider", name: "Some Provider"},
	}
	for _, test := range tests {
		if got := ProviderName(test.provider); got != test.name {
			t.Errorf("ProviderName(%q) = %q, want %q", test.provider, got, test.name)
		}
	}
}

func TestDiscoverKeepsOpenCodeGoAndZenSeparate(t *testing.T) {
	catalog := Discover(context.Background(), Harness{ID: "pi", Name: "Pi", Path: "pi"}, DiscoveryOptions{
		Runner: func(context.Context, string, ...string) ([]byte, error) {
			return []byte(
				"opencode moonshot/kimi-k2 200K 64K yes yes\n" +
					"opencode-go openai/gpt-5.6 400K 128K yes yes\n" +
					"openai-codex gpt-5.6-codex 400K 128K yes yes\n"), nil
		},
	})

	got := make([]string, 0, 3)
	for _, model := range catalog.Models[1:] {
		got = append(got, ProviderName(model.Provider)+":"+model.ID)
	}
	want := []string{
		"OpenAI Codex:openai-codex/gpt-5.6-codex",
		"OpenCode Go:opencode-go/openai/gpt-5.6",
		"OpenCode Zen:opencode/moonshot/kimi-k2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("provider groups = %v, want %v", got, want)
	}
}

func TestDiscoveryDeduplicatesAndSortsModels(t *testing.T) {
	catalog := Discover(context.Background(), Harness{ID: "opencode", Name: "OpenCode", Path: "opencode"}, DiscoveryOptions{
		Runner: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("zeta/z-model\nalpha/a-model\nzeta/z-model\n"), nil
		},
	})
	assertModelIDs(t, catalog.Models, []string{"", "alpha/a-model", "zeta/z-model"})
}

func TestCommandRunnerExecutesRealScripts(t *testing.T) {
	dir := t.TempDir()
	writeScript := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("exit zero returns combined output", func(t *testing.T) {
		path := writeScript("ok.sh", "printf 'openai/gpt-5\\nanthropic/claude\\n'\nexit 0\n")
		out, err := commandRunner(context.Background(), path)
		if err != nil {
			t.Fatalf("commandRunner() error = %v", err)
		}
		if want := "openai/gpt-5\nanthropic/claude\n"; string(out) != want {
			t.Errorf("commandRunner() output = %q, want %q", out, want)
		}
	})

	t.Run("non-zero with output wraps detail", func(t *testing.T) {
		path := writeScript("fail.sh", "echo boom\nexit 3\n")
		out, err := commandRunner(context.Background(), path)
		if out != nil {
			t.Errorf("commandRunner() output = %q, want nil on failure", out)
		}
		if err == nil {
			t.Fatal("commandRunner() error = nil, want failure")
		}
		if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "exit status 3") {
			t.Errorf("commandRunner() error = %q, want wrapped exit status and detail", err)
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Errorf("commandRunner() error = %v, want it to wrap *exec.ExitError", err)
		}
	})

	t.Run("non-zero without output returns bare error", func(t *testing.T) {
		path := writeScript("bare.sh", "exit 4\n")
		out, err := commandRunner(context.Background(), path)
		if out != nil {
			t.Errorf("commandRunner() output = %q, want nil on failure", out)
		}
		if err == nil {
			t.Fatal("commandRunner() error = nil, want failure")
		}
		if err.Error() != "exit status 4" {
			t.Errorf("commandRunner() error = %q, want bare %q", err, "exit status 4")
		}
	})
}

func TestDefaultCodexCachePathFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", "")
	t.Setenv("HOME", home)

	got, err := defaultCodexCachePath()
	if err != nil {
		t.Fatalf("defaultCodexCachePath() error = %v", err)
	}
	if want := filepath.Join(home, ".codex", "models_cache.json"); got != want {
		t.Errorf("defaultCodexCachePath() = %q, want %q", got, want)
	}
}

func TestParsePiModelsDropsLoneTokensAndLabelLines(t *testing.T) {
	models, err := ParsePiModels([]byte(
		"solo\n" +
			"group: heading\n" +
			"openai gpt-5.6 400K 128K yes yes\n"))
	if err != nil {
		t.Fatal(err)
	}
	assertModelIDs(t, models, []string{"openai/gpt-5.6"})
}

func assertModelIDs(t *testing.T, models []Model, want []string) {
	t.Helper()
	got := make([]string, 0, len(models))
	for _, model := range models {
		got = append(got, model.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("model IDs = %v, want %v (models: %#v)", got, want, models)
	}
}
