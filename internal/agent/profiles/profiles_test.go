package profiles

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func render(t *testing.T, out libagent.Outcome, asJSON bool) string {
	t.Helper()
	var b bytes.Buffer
	out.Write(&b, asJSON, Render)
	return b.String()
}

func TestListShowsBuiltinsOutsideGit(t *testing.T) {
	out := Run(context.Background(), t.TempDir(), Options{})
	if out.Status != "success" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
	want := "NAME          HARNESS  MODEL                     SOURCE\n" +
		"implementer   claude   claude-opus-5-5           built-in\n" +
		"orchestrator  claude   claude-opus-5-5           built-in\n" +
		"planner       pi       openai-codex/gpt-6-astra  built-in\n" +
		"reviewer      pi       openai-codex/gpt-6-astra  built-in\n" +
		"verifier      pi       openai-codex/gpt-6-astra  built-in\n"
	if got := render(t, out, false); got != want {
		t.Fatalf("%q\nwant %q", got, want)
	}
	var envelope struct {
		Operation string
		Result    struct {
			Profiles []struct {
				Name, Source, Brief string
				Args                []string
			}
		}
	}
	if err := json.Unmarshal([]byte(render(t, out, true)), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Operation != "agent.profiles" || len(envelope.Result.Profiles) != 5 || envelope.Result.Profiles[2].Name != "planner" || envelope.Result.Profiles[2].Args[0] != "--thinking" || envelope.Result.Profiles[2].Brief == "" {
		t.Fatalf("%+v", envelope)
	}
}

func TestShowPrintsResolvedProfileAndProvenance(t *testing.T) {
	root := t.TempDir()
	exec.Command("git", "-C", root, "init", "-q").Run()
	path := filepath.Join(root, ".fledge", "profiles", "go-review.toml")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("schema_version = 1\nextends = \"builtin:planner\"\n[sections_append]\nmission = \"Mind Go.\"\n"), 0644)
	out := Run(context.Background(), root, Options{Name: "go-review"})
	got := render(t, out, false)
	for _, want := range []string{"Profile go-review\n", "  source: " + path + "\n", "  extends: builtin:planner\n", "  harness: pi\n", "  model: openai-codex/gpt-6-astra\n", "  args: \"--thinking\" \"xhigh\"\n", "  role:\n    ## Mission\n    Investigate", "\n\n    Mind Go.\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	list := render(t, Run(context.Background(), root, Options{}), false)
	if !strings.Contains(list, "go-review     pi       openai-codex/gpt-6-astra  "+path+"\n") {
		t.Fatal(list)
	}
	if _, err := os.Lstat(filepath.Join(root, ".fledge", ".gitignore")); !os.IsNotExist(err) {
		t.Fatal("listing wrote managed files")
	}
}

func TestShowUnsetFieldsAndBuiltinSource(t *testing.T) {
	root := t.TempDir()
	exec.Command("git", "-C", root, "init", "-q").Run()
	path := filepath.Join(root, ".fledge", "profiles", "scout.toml")
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("schema_version = 1\n"), 0644)
	got := render(t, Run(context.Background(), root, Options{Name: "scout"}), false)
	for _, want := range []string{"  harness: -\n", "  model: -\n", "  args: -\n", "  role: -\n"} {
		if !strings.Contains(got, want) || strings.Contains(got, "extends") {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	if got := render(t, Run(context.Background(), t.TempDir(), Options{Name: "reviewer"}), false); !strings.Contains(got, "  source: built-in\n") {
		t.Fatal(got)
	}
}

func TestUnknownOrInvalidProfilesAreRejected(t *testing.T) {
	root := t.TempDir()
	exec.Command("git", "-C", root, "init", "-q").Run()
	os.MkdirAll(filepath.Join(root, ".fledge", "profiles"), 0755)
	os.WriteFile(filepath.Join(root, ".fledge", "profiles", "reviewer.toml"), []byte("bad"), 0644)
	for _, o := range []Options{{Name: "nope"}, {Name: "reviewer"}, {}} {
		dir := root
		if o.Name == "nope" {
			dir = t.TempDir()
		}
		if out := Run(context.Background(), dir, o); out.Status != "rejected" || out.ExitCode() != 2 {
			t.Fatalf("%+v: %+v", o, out)
		}
	}
}
