package profiles

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func git(t *testing.T, args ...string) {
	t.Helper()
	if b, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
}
func repository(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, "-C", root, "init", "-q")
	git(t, "-C", root, "-c", "user.name=Test", "-c", "user.email=t@example.com", "commit", "-qm", "initial", "--allow-empty")
	return root
}
func write(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, ".fledge", "profiles", name+".toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
func load(t *testing.T, cwd, name string) Profile {
	t.Helper()
	p, err := Load(context.Background(), cwd, name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func builtin(t *testing.T, name string) Profile { return load(t, t.TempDir(), name) }

func TestBuiltinsShipFiveRolesWithDefaults(t *testing.T) {
	list, err := List(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name, harness, model string
		args                 []string
	}{
		{"implementer", "claude", "claude-opus-5-5", []string{}},
		{"orchestrator", "claude", "claude-opus-5-5", []string{}},
		{"planner", "codex", "gpt-6-astra", []string{"-c", "model_reasoning_effort=xhigh"}},
		{"reviewer", "pi", "openai-codex/gpt-6-astra", []string{}},
		{"verifier", "pi", "openai-codex/gpt-6-astra", []string{}},
	}
	if len(list) != len(want) {
		t.Fatalf("%+v", list)
	}
	roles := map[string]bool{}
	for i, w := range want {
		p := list[i]
		if p.Name != w.name || p.Harness != w.harness || p.Model != w.model || !reflect.DeepEqual(p.Args, w.args) {
			t.Fatalf("%d: %+v want %+v", i, p, w)
		}
		if p.Source != "builtin" || p.Path != nil || p.Base != nil {
			t.Fatalf("provenance: %+v", p)
		}
		if strings.TrimSpace(p.Role) == "" || roles[p.Role] {
			t.Fatalf("role not distinct: %q", p.Role)
		}
		roles[p.Role] = true
		for _, a := range p.Args {
			if strings.Contains(strings.ToLower(a), "permission") || strings.Contains(a, "bypass") || strings.Contains(a, "dangerous") || strings.Contains(a, "yolo") || strings.Contains(a, "sandbox") {
				t.Fatalf("%s ships permission-changing args %q", p.Name, p.Args)
			}
		}
	}
	if !strings.Contains(builtin(t, "reviewer").Role, "without editing") || !strings.Contains(builtin(t, "planner").Role, "read-only") {
		t.Fatal("role briefs lost their distinguishing text")
	}
}

func TestSameNameFileOverlaysItsBuiltin(t *testing.T) {
	root := repository(t)
	path := write(t, root, "reviewer", "schema_version = 1\nmodel = \"other\"\nrole_append = \"Focus on Go.\"\n")
	base := builtin(t, "reviewer")
	p := load(t, root, "reviewer")
	if p.Harness != "pi" || p.Model != "other" || !reflect.DeepEqual(p.Args, []string{}) {
		t.Fatalf("%+v", p)
	}
	if p.Role != base.Role+"\n\nFocus on Go." {
		t.Fatalf("%q", p.Role)
	}
	if p.Source != "repo" || p.Path == nil || *p.Path != path || p.Base == nil || *p.Base != "builtin:reviewer" {
		t.Fatalf("provenance: %+v", p)
	}
}

func TestOmittedFieldsInheritAndEmptyValuesClear(t *testing.T) {
	root := repository(t)
	write(t, root, "planner", "schema_version = 1\n")
	base := builtin(t, "planner")
	if got := load(t, root, "planner"); got.Harness != base.Harness || got.Model != base.Model || !reflect.DeepEqual(got.Args, base.Args) || got.Role != base.Role {
		t.Fatalf("omitted fields lost: %+v", got)
	}
	write(t, root, "planner", "schema_version = 1\nargs = []\nmodel = \"\"\nrole = \"\"\n")
	if got := load(t, root, "planner"); got.Harness != "codex" || got.Model != "" || !reflect.DeepEqual(got.Args, []string{}) || got.Role != "" {
		t.Fatalf("empty values did not clear: %+v", got)
	}
	write(t, root, "planner", "schema_version = 1\nargs = [\"--x\"]\nrole = \"Replacement.\"\n")
	if got := load(t, root, "planner"); !reflect.DeepEqual(got.Args, []string{"--x"}) || got.Role != "Replacement." {
		t.Fatalf("replacement: %+v", got)
	}
}

func TestCustomProfilesExtendABuiltinOrStandAlone(t *testing.T) {
	root := repository(t)
	write(t, root, "go-review", "schema_version = 1\nextends = \"builtin:reviewer\"\nharness = \"claude\"\nmodel = \"sonnet\"\n")
	p := load(t, root, "go-review")
	if p.Harness != "claude" || p.Model != "sonnet" || p.Role != builtin(t, "reviewer").Role || *p.Base != "builtin:reviewer" {
		t.Fatalf("%+v", p)
	}
	// An explicit base replaces the same-name default.
	write(t, root, "verifier", "schema_version = 1\nextends = \"builtin:planner\"\n")
	if p := load(t, root, "verifier"); p.Harness != "codex" || p.Role != builtin(t, "planner").Role || *p.Base != "builtin:planner" {
		t.Fatalf("%+v", p)
	}
	write(t, root, "scout", "schema_version = 1\nrole = \"Explore.\"\n")
	p = load(t, root, "scout")
	if p.Harness != "" || p.Model != "" || !reflect.DeepEqual(p.Args, []string{}) || p.Role != "Explore." || p.Base != nil || p.Source != "repo" {
		t.Fatalf("%+v", p)
	}
	write(t, root, "solo", "schema_version = 1\nrole_append = \"Only this.\"\n")
	if p := load(t, root, "solo"); p.Role != "Only this." {
		t.Fatalf("%q", p.Role)
	}
}

func TestInvalidProfilesFailWithoutFallback(t *testing.T) {
	for _, tc := range []struct{ name, file, content, want string }{
		{"missing schema", "reviewer", "model = \"x\"\n", "schema_version"},
		{"future schema", "reviewer", "schema_version = 2\n", "schema_version"},
		{"unknown key", "reviewer", "schema_version = 1\nmodle = \"x\"\n", "modle"},
		{"case-variant key", "reviewer", "schema_version = 1\nMODEL = \"two\"\n", "MODEL"},
		{"canonical and case-variant key", "reviewer", "schema_version = 1\nmodel = \"one\"\nMODEL = \"two\"\n", "MODEL"},
		{"case-variant schema_version", "reviewer", "Schema_Version = 1\n", "Schema_Version"},
		{"wrong type", "reviewer", "schema_version = 1\nargs = \"--x\"\n", "args"},
		{"syntax", "reviewer", "schema_version = \n", "reviewer.toml"},
		{"role and append", "reviewer", "schema_version = 1\nrole = \"a\"\nrole_append = \"b\"\n", "role_append"},
		{"unknown base", "custom", "schema_version = 1\nextends = \"builtin:nope\"\n", "extends"},
		{"repo base", "custom", "schema_version = 1\nextends = \"reviewer\"\n", "extends"},
		{"unknown harness", "reviewer", "schema_version = 1\nharness = \"nope\"\n", "harness"},
		{"empty harness", "reviewer", "schema_version = 1\nharness = \"\"\n", "harness"},
		{"nul", "reviewer", "schema_version = 1\nrole = \"a\\u0000b\"\n", "NUL"},
		{"nul arg", "reviewer", "schema_version = 1\nargs = [\"a\\u0000\"]\n", "NUL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := repository(t)
			write(t, root, tc.file, tc.content)
			p, err := Load(context.Background(), root, tc.file)
			if err == nil {
				t.Fatalf("accepted: %+v", p)
			}
			if !errors.As(err, new(*libagent.InputError)) || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.file+".toml") {
				t.Fatalf("%T %v", err, err)
			}
			if _, err := List(context.Background(), root); err == nil {
				t.Fatal("listing hid an invalid file")
			}
		})
	}
}

func TestInvalidNamesAndMissingProfilesAreRejected(t *testing.T) {
	root := repository(t)
	os.MkdirAll(filepath.Join(root, ".fledge", "profiles", "dir.toml"), 0755)
	for _, name := range []string{"", "../reviewer", "Reviewer", "a/b", "missing", "dir"} {
		if p, err := Load(context.Background(), root, name); err == nil || !errors.As(err, new(*libagent.InputError)) {
			t.Fatalf("%q: %+v %v", name, p, err)
		}
	}
}

func TestProfilesComeFromTheInvokingCheckout(t *testing.T) {
	root := repository(t)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, "-C", root, "worktree", "add", "-qb", "linked", linked)
	os.MkdirAll(filepath.Join(linked, "sub"), 0755)
	write(t, root, "reviewer", "schema_version = 1\nmodel = \"primary\"\n")
	write(t, linked, "reviewer", "schema_version = 1\nmodel = \"linked\"\n")
	for cwd, want := range map[string]string{root: "primary", linked: "linked", filepath.Join(linked, "sub"): "linked"} {
		if p := load(t, cwd, "reviewer"); p.Model != want {
			t.Fatalf("%s: %+v", cwd, p)
		}
	}
}

func TestLoadingDoesNotWrite(t *testing.T) {
	root := repository(t)
	load(t, root, "reviewer")
	if _, err := List(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".fledge")); !os.IsNotExist(err) {
		t.Fatalf("created managed directory: %v", err)
	}
}

func TestListMergesRepoProfilesByName(t *testing.T) {
	root := repository(t)
	write(t, root, "reviewer", "schema_version = 1\nmodel = \"x\"\n")
	write(t, root, "alpha", "schema_version = 1\nharness = \"codex\"\n")
	os.WriteFile(filepath.Join(root, ".fledge", "profiles", "notes.md"), []byte("ignored"), 0644)
	list, err := List(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range list {
		names = append(names, p.Name)
		if p.Name == "reviewer" && (p.Model != "x" || p.Source != "repo") {
			t.Fatalf("%+v", p)
		}
	}
	if !slices.Equal(names, []string{"alpha", "implementer", "orchestrator", "planner", "reviewer", "verifier"}) {
		t.Fatal(names)
	}
}
