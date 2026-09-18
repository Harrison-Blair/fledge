package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	outputs map[string]string
	errs    map[string]error
	calls   []string
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	if err := f.errs[key]; err != nil {
		return nil, err
	}
	out, ok := f.outputs[key]
	if !ok {
		return nil, errors.New("executable not found")
	}
	return []byte(out), nil
}
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
func fixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	write(t, filepath.Join(home, ".pi", "agent", "models-store.json"), `{"openai-codex":{"models":[{"id":"gpt-5.3-codex","name":"GPT-5.3 Codex"},{"id":"nameless"}],"checkedAt":1},"opencode":{"models":[{"id":"big-pickle","name":"Big Pickle"}]}}`)
	write(t, filepath.Join(home, ".codex", "models_cache.json"), `{"models":[{"slug":"gpt-5.6-sol","display_name":"GPT-5.6-Sol","visibility":"list"},{"slug":"hidden","display_name":"Hidden","visibility":"hide"},{"slug":"unmarked","display_name":"Unmarked"},{"slug":"internal","display_name":"Internal","visibility":"internal"},{"slug":"gpt-5.3-codex","display_name":"GPT-5.3 Codex","visibility":"list"}]}`)
	old := filepath.Join(home, ".claude", "cache", "model-catalog", "old.json")
	newest := filepath.Join(home, ".claude", "cache", "model-catalog", "new.json")
	write(t, old, `{"catalog":{"config":{"models":[{"id":"claude-stale","name":"Stale"}]}}}`)
	write(t, newest, `{"catalog":{"config":{"models":[{"id":"claude-opus-5","name":"Opus 5"},{"id":"claude-haiku-4-5","name":"Haiku 4.5"}]}}}`)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(old, base, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newest, base.Add(time.Hour), base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	return home
}
func fixtureRunner() *fakeRunner {
	return &fakeRunner{outputs: map[string]string{
		"opencode models":            "opencode/big-pickle\nanthropic/claude-opus-5\n\n",
		"cursor-agent --list-models": "Available models\n\nauto - Auto (current, default)\ngpt-5.3-codex-low - Codex 5.3 Low\n",
	}}
}
func name(s string) *string { return &s }
func TestModelsDiscoversAllHarnessesSorted(t *testing.T) {
	d := Discovery{Home: fixtureHome(t), Run: fixtureRunner().run}
	out := d.Models(context.Background(), ModelsOptions{})
	if out.Status != "success" || out.Error != nil || out.Operation != "agent.models" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
	want := ModelsResult{Models: []ModelRow{
		{Harness: "claude", Model: "claude-haiku-4-5", Name: name("Haiku 4.5")},
		{Harness: "claude", Model: "claude-opus-5", Name: name("Opus 5")},
		{Harness: "codex", Model: "gpt-5.3-codex", Name: name("GPT-5.3 Codex")},
		{Harness: "codex", Model: "gpt-5.6-sol", Name: name("GPT-5.6-Sol")},
		{Harness: "cursor", Model: "auto", Name: name("Auto (current, default)")},
		{Harness: "cursor", Model: "gpt-5.3-codex-low", Name: name("Codex 5.3 Low")},
		{Harness: "opencode", Model: "anthropic/claude-opus-5"},
		{Harness: "opencode", Model: "opencode/big-pickle"},
		{Harness: "pi", Model: "openai-codex/gpt-5.3-codex", Name: name("GPT-5.3 Codex")},
		{Harness: "pi", Model: "openai-codex/nameless"},
		{Harness: "pi", Model: "opencode/big-pickle", Name: name("Big Pickle")},
	}}
	if !reflect.DeepEqual(out.Result, want) {
		t.Fatalf("got %+v\nwant %+v", out.Result, want)
	}
}
func TestModelsHarnessFilter(t *testing.T) {
	home := fixtureHome(t)
	for _, tc := range []struct {
		harness string
		want    []string
	}{
		{"claude", []string{"claude-haiku-4-5", "claude-opus-5"}},
		{"cursor", []string{"auto", "gpt-5.3-codex-low"}},
		{"kiro", []string{}},
	} {
		t.Run(tc.harness, func(t *testing.T) {
			runner := fixtureRunner()
			out := Discovery{Home: home, Run: runner.run}.Models(context.Background(), ModelsOptions{Harness: tc.harness})
			if out.Status != "success" || out.Error != nil {
				t.Fatalf("%+v", out)
			}
			got := []string{}
			for _, m := range out.Result.(ModelsResult).Models {
				if m.Harness != tc.harness {
					t.Fatalf("leaked %s", m.Harness)
				}
				got = append(got, m.Model)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			if tc.harness != "cursor" && len(runner.calls) != 0 {
				t.Fatalf("ran commands for another harness: %q", runner.calls)
			}
		})
	}
}
func TestModelsRejectsUnknownHarness(t *testing.T) {
	runner := fixtureRunner()
	out := Discovery{Home: fixtureHome(t), Run: runner.run}.Models(context.Background(), ModelsOptions{Harness: "nope"})
	if out.Status != "rejected" || out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Phase != "validation" || out.ExitCode() != 2 {
		t.Fatalf("%+v", out)
	}
	if out.Result != nil || out.Effects == nil || len(runner.calls) != 0 {
		t.Fatalf("discovery ran or envelope malformed: %+v", out)
	}
}
func TestModelsFailSoft(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, home string, r *fakeRunner)
	}{
		{"empty home", func(*testing.T, string, *fakeRunner) {}},
		{"malformed files", func(t *testing.T, home string, r *fakeRunner) {
			write(t, filepath.Join(home, ".pi", "agent", "models-store.json"), `{"broken":`)
			write(t, filepath.Join(home, ".codex", "models_cache.json"), `[]`)
			write(t, filepath.Join(home, ".claude", "cache", "model-catalog", "a.json"), `not json`)
		}},
		{"wrong shapes", func(t *testing.T, home string, r *fakeRunner) {
			write(t, filepath.Join(home, ".pi", "agent", "models-store.json"), `{"p":{"models":"nope"}}`)
			write(t, filepath.Join(home, ".codex", "models_cache.json"), `{"models":{}}`)
			write(t, filepath.Join(home, ".claude", "cache", "model-catalog", "a.json"), `{"catalog":{"config":{"models":{}}}}`)
		}},
		{"commands fail", func(t *testing.T, home string, r *fakeRunner) {
			r.errs = map[string]error{"opencode models": errors.New("exit 1"), "cursor-agent --list-models": errors.New("exit 1")}
		}},
		{"command output is noise", func(t *testing.T, home string, r *fakeRunner) {
			r.outputs = map[string]string{"opencode models": "\n  \n", "cursor-agent --list-models": "Available models\n\n"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			runner := &fakeRunner{}
			tc.setup(t, home, runner)
			out := Discovery{Home: home, Run: runner.run}.Models(context.Background(), ModelsOptions{})
			if out.Status != "success" || out.Error != nil {
				t.Fatalf("%+v", out)
			}
			if got := out.Result.(ModelsResult).Models; len(got) != 0 {
				t.Fatalf("unexpected rows %+v", got)
			}
		})
	}
}
func TestModelsPartialFailureKeepsOtherHarnesses(t *testing.T) {
	home := fixtureHome(t)
	write(t, filepath.Join(home, ".codex", "models_cache.json"), `{"models":[`)
	runner := fixtureRunner()
	runner.errs = map[string]error{"cursor-agent --list-models": errors.New("exit 1")}
	out := Discovery{Home: home, Run: runner.run}.Models(context.Background(), ModelsOptions{})
	seen := map[string]bool{}
	for _, m := range out.Result.(ModelsResult).Models {
		seen[m.Harness] = true
	}
	if out.Status != "success" || seen["codex"] || seen["cursor"] || !seen["pi"] || !seen["claude"] || !seen["opencode"] {
		t.Fatalf("%+v", out)
	}
}
func TestModelsJSONEnvelope(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".codex", "models_cache.json"), `{"models":[{"slug":"gpt-5.6-sol","display_name":"GPT-5.6-Sol","visibility":"list"}]}`)
	runner := &fakeRunner{outputs: map[string]string{"opencode models": "opencode/big-pickle\n"}}
	out := Discovery{Home: home, Run: runner.run}.Models(context.Background(), ModelsOptions{})
	var b strings.Builder
	if err := out.Write(&b, true); err != nil {
		t.Fatal(err)
	}
	want := `{"operation":"agent.models","status":"success","result":{"models":[{"harness":"codex","model":"gpt-5.6-sol","name":"GPT-5.6-Sol"},{"harness":"opencode","model":"opencode/big-pickle","name":null}]},"effects":[],"error":null}` + "\n"
	if b.String() != want {
		t.Fatalf("got %s\nwant %s", b.String(), want)
	}
	var round Outcome
	if err := json.Unmarshal([]byte(b.String()), &round); err != nil {
		t.Fatal(err)
	}
}
func TestModelsEmptyResultIsNotNull(t *testing.T) {
	out := Discovery{Home: t.TempDir(), Run: (&fakeRunner{}).run}.Models(context.Background(), ModelsOptions{})
	var b strings.Builder
	if err := out.Write(&b, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"result":{"models":[]}`) {
		t.Fatal(b.String())
	}
}
func TestLocalDiscoveryUsesHomeAndRunner(t *testing.T) {
	t.Setenv("HOME", "/nonexistent/fledge-home")
	d := LocalDiscovery()
	if d.Home != "/nonexistent/fledge-home" || d.Run == nil {
		t.Fatalf("%+v", d)
	}
	if _, err := d.Run(context.Background(), "fledge-definitely-missing-binary-9f2c"); err == nil {
		t.Fatal("missing binary succeeded")
	}
}
