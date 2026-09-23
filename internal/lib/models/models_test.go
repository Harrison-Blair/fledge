package models

import (
	"context"
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
	write(t, filepath.Join(home, ".pi", "agent", "models-store.json"), `{"openai-codex":{"models":[{"id":"gpt-5.3-codex","name":"GPT-5.3 Codex"},{"id":"nameless"}],"checkedAt":1}}`)
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

// Each source reader keeps its own filtering: codex visibility, the newest
// claude catalog, blank opencode lines, and cursor's "id - label" rows.
func TestDiscoverParsesEachSource(t *testing.T) {
	d := Discovery{Home: fixtureHome(t), Run: fixtureRunner().run}
	for kind, want := range map[string][]Row{
		"pi":       {{Harness: "pi", Model: "openai-codex/gpt-5.3-codex", Name: name("GPT-5.3 Codex")}, {Harness: "pi", Model: "openai-codex/nameless"}},
		"codex":    {{Harness: "codex", Model: "gpt-5.6-sol", Name: name("GPT-5.6-Sol")}, {Harness: "codex", Model: "gpt-5.3-codex", Name: name("GPT-5.3 Codex")}},
		"claude":   {{Harness: "claude", Model: "claude-opus-5", Name: name("Opus 5")}, {Harness: "claude", Model: "claude-haiku-4-5", Name: name("Haiku 4.5")}},
		"opencode": {{Harness: "opencode", Model: "opencode/big-pickle"}, {Harness: "opencode", Model: "anthropic/claude-opus-5"}},
		"cursor":   {{Harness: "cursor", Model: "auto", Name: name("Auto (current, default)")}, {Harness: "cursor", Model: "gpt-5.3-codex-low", Name: name("Codex 5.3 Low")}},
	} {
		got, err := d.Discover(context.Background(), kind)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %+v, %v\nwant %+v", kind, got, err, want)
		}
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
