package models

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestModelHarnesses(t *testing.T) {
	got := ModelHarnesses()
	want := []string{"pi", "codex", "claude", "opencode", "cursor"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	// A returned slice must not alias the internal source list.
	got[0] = "mutated"
	if ModelHarnesses()[0] != "pi" {
		t.Fatal("ModelHarnesses leaked its backing array")
	}
}

func TestReadOnlyModelHarnesses(t *testing.T) {
	got := ReadOnlyModelHarnesses()
	want := []string{"pi", "codex", "claude"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	// The result must not alias the internal source list.
	got[0] = "mutated"
	if ReadOnlyModelHarnesses()[0] != "pi" {
		t.Fatal("ReadOnlyModelHarnesses leaked its backing array")
	}
}

func TestReadOnlyDiscoverNeverRunsCommands(t *testing.T) {
	// Every read-only kind reads local caches only; none may execute a command.
	d := Discovery{Home: fixtureHome(t), Run: func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("read-only discovery must not run commands")
		return nil, nil
	}}
	for _, kind := range ReadOnlyModelHarnesses() {
		if _, err := d.Discover(context.Background(), kind); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}

func TestDiscoverReportsRowsAndTagsHarness(t *testing.T) {
	d := Discovery{Home: fixtureHome(t), Run: fixtureRunner().run}
	rows, err := d.Discover(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows %+v", rows)
	}
	for _, r := range rows {
		if r.Harness != "claude" {
			t.Fatalf("untagged row %+v", r)
		}
	}
}

func TestDiscoverSurfacesPerHarnessError(t *testing.T) {
	home := t.TempDir()
	// A malformed cache is a broken source, distinct from a missing one.
	write(t, filepath.Join(home, ".codex", "models_cache.json"), `{"models":[`)
	_, err := Discovery{Home: home, Run: (&fakeRunner{}).run}.Discover(context.Background(), "codex")
	if err == nil {
		t.Fatal("expected discovery error for malformed cache")
	}
}

func TestDiscoverEmptyCacheIsNotAnError(t *testing.T) {
	// An existing but empty claude catalog dir yields no rows and no error,
	// distinct from a missing dir (which callers treat as a broken source).
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "cache", "model-catalog"), 0o755); err != nil {
		t.Fatal(err)
	}
	rows, err := Discovery{Home: home, Run: (&fakeRunner{}).run}.Discover(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows %+v", rows)
	}
}

func TestDiscoverMissingSourceIsAnError(t *testing.T) {
	// A missing pi store surfaces the read error so the model check can flag it.
	_, err := Discovery{Home: t.TempDir(), Run: (&fakeRunner{}).run}.Discover(context.Background(), "pi")
	if err == nil {
		t.Fatal("expected error for a missing pi store")
	}
}

func TestDiscoverUnknownHarnessErrors(t *testing.T) {
	_, err := Discovery{Home: t.TempDir(), Run: (&fakeRunner{}).run}.Discover(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error for harness without a local source")
	}
}
