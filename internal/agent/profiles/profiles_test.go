package profiles

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
)

func render(t *testing.T, out libagent.Outcome, asJSON bool) string {
	t.Helper()
	var b bytes.Buffer
	out.Write(&b, asJSON, Render)
	return b.String()
}

// repoWith creates a Git checkout holding one .fledge/profiles file.
func repoWith(t *testing.T, file, content string) (string, string) {
	t.Helper()
	root := t.TempDir()
	gittest.Git(t, root, "init", "-q")
	path := filepath.Join(root, ".fledge", "profiles", file)
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
	return root, path
}

func TestListShowsBuiltinsOutsideGit(t *testing.T) {
	out := Run(context.Background(), t.TempDir(), Options{})
	if out.Status != "success" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
	want := "NAME          SOURCE\n" +
		"debugger      built-in\n" +
		"implementer   built-in\n" +
		"integrator    built-in\n" +
		"orchestrator  built-in\n" +
		"planner       built-in\n" +
		"researcher    built-in\n" +
		"reviewer      built-in\n" +
		"verifier      built-in\n"
	if got := render(t, out, false); got != want {
		t.Fatalf("%q\nwant %q", got, want)
	}
	var envelope struct {
		Operation string
		Result    struct{ Profiles []map[string]any }
	}
	if err := json.Unmarshal([]byte(render(t, out, true)), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Operation != "agent.profiles" || len(envelope.Result.Profiles) != 8 {
		t.Fatalf("%+v", envelope)
	}
	for _, p := range envelope.Result.Profiles {
		brief, _ := p["brief"].(string)
		if keys := slices.Sorted(maps.Keys(p)); !slices.Equal(keys, []string{"brief", "name", "path", "source"}) || !strings.Contains(brief, "## Fledge protocol\n") || strings.Contains(brief, "## Project memory") {
			t.Fatalf("%v", p)
		}
	}
}

func TestShowPrintsBriefAndProvenance(t *testing.T) {
	root, path := repoWith(t, "go-review.md", "## Mission\nMind Go.\n")
	got := render(t, Run(context.Background(), root, Options{Name: "go-review"}), false)
	for _, want := range []string{"Profile go-review\n  source: " + path + "\n  brief:\n    ## Mission\n    Mind Go.\n\n    ## Fledge protocol\n    You are a Fledge-managed agent"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	for _, retired := range []string{"harness:", "model:", "args:", "reads:", "protocol:", "extends"} {
		if strings.Contains(got, "  "+retired) {
			t.Fatalf("still shows %q:\n%s", retired, got)
		}
	}
	list := render(t, Run(context.Background(), root, Options{}), false)
	if !strings.Contains(list, "go-review     "+path+"\n") {
		t.Fatal(list)
	}
	if _, err := os.Lstat(filepath.Join(root, ".fledge", ".gitignore")); !os.IsNotExist(err) {
		t.Fatal("listing wrote managed files")
	}
	if got := render(t, Run(context.Background(), t.TempDir(), Options{Name: "reviewer"}), false); !strings.Contains(got, "  source: built-in\n") {
		t.Fatal(got)
	}
}

func TestUnknownInvalidOrLegacyProfilesAreRejected(t *testing.T) {
	blank, _ := repoWith(t, "reviewer.md", " \n")
	legacy, _ := repoWith(t, "reviewer.toml", "schema_version = 1\n")
	for _, tc := range []struct {
		dir  string
		o    Options
		want string
	}{
		{t.TempDir(), Options{Name: "nope"}, "unknown profile"},
		{blank, Options{Name: "reviewer"}, "blank"},
		{blank, Options{}, "blank"},
		{legacy, Options{Name: "reviewer"}, "legacy TOML profile"},
		{legacy, Options{}, "legacy TOML profile"},
	} {
		out := Run(context.Background(), tc.dir, tc.o)
		if out.Status != "rejected" || out.ExitCode() != 2 || !strings.Contains(out.Error.Message, tc.want) {
			t.Fatalf("%+v: %+v %+v", tc.o, out, out.Error)
		}
	}
}
