package profiles

import (
	"context"
	"encoding/json"
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
		{"planner", "pi", "openai-codex/gpt-6-astra", []string{"--thinking", "xhigh"}},
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
		if strings.TrimSpace(p.Sections.Mission) == "" || roles[p.Sections.Mission] || !p.Protocol {
			t.Fatalf("mission not distinct or protocol off: %+v", p)
		}
		roles[p.Sections.Mission] = true
		if p.Harness == "codex" {
			t.Fatalf("%s uses the codex harness; codex models go through pi", p.Name)
		}
		for _, a := range p.Args {
			if strings.Contains(strings.ToLower(a), "permission") || strings.Contains(a, "bypass") || strings.Contains(a, "dangerous") || strings.Contains(a, "yolo") || strings.Contains(a, "sandbox") {
				t.Fatalf("%s ships permission-changing args %q", p.Name, p.Args)
			}
		}
	}
	if !strings.Contains(builtin(t, "reviewer").Sections.Mission, "without editing") || !strings.Contains(builtin(t, "planner").Sections.Mission, "read-only") {
		t.Fatal("role briefs lost their distinguishing text")
	}
}

func TestSameNameFileOverlaysItsBuiltin(t *testing.T) {
	root := repository(t)
	path := write(t, root, "reviewer", "schema_version = 1\nmodel = \"other\"\n[sections_append]\nmission = \"Focus on Go.\"\n")
	base := builtin(t, "reviewer")
	p := load(t, root, "reviewer")
	if p.Harness != "pi" || p.Model != "other" || !reflect.DeepEqual(p.Args, []string{}) {
		t.Fatalf("%+v", p)
	}
	if p.Sections.Mission != base.Sections.Mission+"\n\nFocus on Go." || !p.Protocol {
		t.Fatalf("%+v", p)
	}
	if p.Source != "repo" || p.Path == nil || *p.Path != path || p.Base == nil || *p.Base != "builtin:reviewer" {
		t.Fatalf("provenance: %+v", p)
	}
}

func TestOmittedFieldsInheritAndEmptyValuesClear(t *testing.T) {
	root := repository(t)
	write(t, root, "planner", "schema_version = 1\n")
	base := builtin(t, "planner")
	if got := load(t, root, "planner"); got.Harness != base.Harness || got.Model != base.Model || !reflect.DeepEqual(got.Args, base.Args) || got.Sections != base.Sections || got.Protocol != base.Protocol {
		t.Fatalf("omitted fields lost: %+v", got)
	}
	write(t, root, "planner", "schema_version = 1\nargs = []\nmodel = \"\"\nprotocol = false\n[sections]\nmission = \"\"\n")
	if got := load(t, root, "planner"); got.Harness != "pi" || got.Model != "" || !reflect.DeepEqual(got.Args, []string{}) || got.Sections.Mission != "" || got.Protocol || got.Brief() != "" {
		t.Fatalf("empty values did not clear: %+v", got)
	}
	write(t, root, "planner", "schema_version = 1\nargs = [\"--x\"]\n[sections]\nmission = \"Replacement.\"\n")
	if got := load(t, root, "planner"); !reflect.DeepEqual(got.Args, []string{"--x"}) || got.Sections.Mission != "Replacement." || !got.Protocol {
		t.Fatalf("replacement: %+v", got)
	}
}

func TestCustomProfilesExtendABuiltinOrStandAlone(t *testing.T) {
	root := repository(t)
	write(t, root, "go-review", "schema_version = 1\nextends = \"builtin:reviewer\"\nharness = \"claude\"\nmodel = \"sonnet\"\n")
	p := load(t, root, "go-review")
	if p.Harness != "claude" || p.Model != "sonnet" || p.Sections != builtin(t, "reviewer").Sections || !p.Protocol || *p.Base != "builtin:reviewer" {
		t.Fatalf("%+v", p)
	}
	// An explicit base replaces the same-name default.
	write(t, root, "verifier", "schema_version = 1\nextends = \"builtin:planner\"\n")
	if p := load(t, root, "verifier"); !reflect.DeepEqual(p.Args, []string{"--thinking", "xhigh"}) || p.Sections != builtin(t, "planner").Sections || *p.Base != "builtin:planner" {
		t.Fatalf("%+v", p)
	}
	write(t, root, "scout", "schema_version = 1\n[sections]\nmission = \"Explore.\"\n")
	p = load(t, root, "scout")
	if p.Harness != "" || p.Model != "" || !reflect.DeepEqual(p.Args, []string{}) || p.Sections != (Sections{Mission: "Explore."}) || p.Protocol || !reflect.DeepEqual(p.Reads, []string{}) || p.Base != nil || p.Source != "repo" {
		t.Fatalf("%+v", p)
	}
	write(t, root, "solo", "schema_version = 1\n[sections_append]\nnever = \"Only this.\"\n")
	if p := load(t, root, "solo"); p.Sections != (Sections{Never: "Only this."}) {
		t.Fatalf("%+v", p.Sections)
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
		{"removed role", "reviewer", "schema_version = 1\nrole = \"a\"\n", "role was replaced by [sections]"},
		{"removed role_append", "reviewer", "schema_version = 1\nrole_append = \"a\"\n", "[sections]"},
		{"unknown section", "reviewer", "schema_version = 1\n[sections]\nmision = \"a\"\n", "mision"},
		{"unknown appended section", "reviewer", "schema_version = 1\n[sections_append]\nnotes = \"a\"\n", "notes"},
		{"case-variant section", "reviewer", "schema_version = 1\n[sections]\nMission = \"a\"\n", "Mission"},
		{"case-variant sections table", "reviewer", "schema_version = 1\n[Sections]\nmission = \"a\"\n", "Sections"},
		{"section in both tables", "reviewer", "schema_version = 1\n[sections]\nnever = \"a\"\n[sections_append]\nnever = \"b\"\n", "never"},
		{"section wrong type", "reviewer", "schema_version = 1\nsections = \"a\"\n", "sections"},
		{"appended section wrong type", "reviewer", "schema_version = 1\nsections_append = [\"a\"]\n", "sections_append"},
		{"section value wrong type", "reviewer", "schema_version = 1\n[sections]\nmission = 1\n", "mission"},
		{"absolute read", "reviewer", "schema_version = 1\nreads = [\"/etc/passwd\"]\n", "reads"},
		{"parent read", "reviewer", "schema_version = 1\nreads = [\"../x.md\"]\n", "reads"},
		{"inner parent read", "reviewer", "schema_version = 1\nreads = [\"docs/../../x.md\"]\n", "reads"},
		{"nul read", "reviewer", "schema_version = 1\nreads = [\"a\\u0000\"]\n", "NUL"},
		{"nul section", "reviewer", "schema_version = 1\n[sections]\nreport = \"a\\u0000\"\n", "NUL"},
		{"nul appended section", "reviewer", "schema_version = 1\n[sections_append]\nreport = \"a\\u0000\"\n", "NUL"},
		{"protocol wrong type", "reviewer", "schema_version = 1\nprotocol = \"yes\"\n", "protocol"},
		{"unknown base", "custom", "schema_version = 1\nextends = \"builtin:nope\"\n", "extends"},
		{"repo base", "custom", "schema_version = 1\nextends = \"reviewer\"\n", "extends"},
		{"unknown harness", "reviewer", "schema_version = 1\nharness = \"nope\"\n", "harness"},
		{"empty harness", "reviewer", "schema_version = 1\nharness = \"\"\n", "harness"},
		{"nul", "reviewer", "schema_version = 1\nmodel = \"a\\u0000b\"\n", "NUL"},
		{"invalid utf-8 section", "reviewer", "schema_version = 1\n[sections]\nmission = \"a\xffb\"\n", "reviewer.toml"},
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

func TestSectionsAppendReadsAndProtocolOverlay(t *testing.T) {
	root := repository(t)
	write(t, root, "base", "schema_version = 1\nprotocol = true\nreads = [\"AGENTS.md\", \"docs/guide.md\"]\n[sections]\nmission = \"Do it.\"\nnever = \"Push.\"\n")
	p := load(t, root, "base")
	if !p.Protocol || !reflect.DeepEqual(p.Reads, []string{"AGENTS.md", "docs/guide.md"}) || p.Sections != (Sections{Mission: "Do it.", Never: "Push."}) {
		t.Fatalf("%+v", p)
	}
	write(t, root, "reviewer", "schema_version = 1\nreads = []\n[sections]\nreport = \"Findings.\"\n[sections_append]\nmission = \"Also Go.\"\nalways = \"Cite.\"\n")
	base := builtin(t, "reviewer")
	p = load(t, root, "reviewer")
	want := base.Sections
	want.Report, want.Mission, want.Always = "Findings.", base.Sections.Mission+"\n\nAlso Go.", "Cite."
	if p.Sections != want || !reflect.DeepEqual(p.Reads, []string{}) || !p.Protocol {
		t.Fatalf("%+v", p)
	}
	write(t, root, "reviewer", "schema_version = 1\nprotocol = false\n")
	if p := load(t, root, "reviewer"); p.Protocol || p.Sections != base.Sections {
		t.Fatalf("%+v", p)
	}
}

func TestBriefRendersPresentPartsInFixedOrder(t *testing.T) {
	p := Profile{
		Reads:    []string{"AGENTS.md", "README.md"},
		Protocol: true,
		Sections: Sections{Mission: "M.", Workflow: "W.", Always: "A.", Never: "N.", Protocol: "P.", Report: "R."},
	}
	want := "## Mission\nM.\n\n## Read first\nRead these files in your working directory before starting: `AGENTS.md`, `README.md`.\n\n" +
		"## Workflow\nW.\n\n## Always\nA.\n\n## Never\nN.\n\n## Fledge protocol\n" + shared + "\nP.\n\n## Report\nR."
	if got := p.Brief(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(shared, "fledge agent current") || !strings.HasSuffix(shared, ".\n") {
		t.Fatalf("shared block not embedded: %q", shared)
	}
	if got := (Profile{Sections: Sections{Workflow: "W.", Protocol: "P."}}).Brief(); got != "## Workflow\nW.\n\n## Fledge protocol\nP." {
		t.Fatalf("%q", got)
	}
	if got := (Profile{Protocol: true}).Brief(); got != "## Fledge protocol\n"+shared {
		t.Fatalf("%q", got)
	}
	if got := (Profile{Reads: []string{}}).Brief(); got != "" {
		t.Fatalf("%q", got)
	}
}

func TestSectionMarkdownSurvivesTOMLByteForByte(t *testing.T) {
	root := repository(t)
	text := "- Run `fledge task list --json` first.\n- Pass `--flag` exactly; use `a*b` and C:\\dir.\n  1. Nested `code`."
	write(t, root, "md", "schema_version = 1\n[sections]\nworkflow = \"\"\"\n"+strings.ReplaceAll(text, "\\", "\\\\")+"\n\"\"\"\n")
	p := load(t, root, "md")
	if p.Sections.Workflow != text+"\n" {
		t.Fatalf("%q", p.Sections.Workflow)
	}
	if got := p.Brief(); got != "## Workflow\n"+text+"\n" {
		t.Fatalf("%q", got)
	}
}

// Section text renders exactly as decoded; joins add only the newlines that
// separate blocks by one blank line.
func TestBriefKeepsSectionTextExact(t *testing.T) {
	p := Profile{Sections: Sections{Mission: "\n  M.\n", Workflow: "W.", Never: "N.\n\n", Report: "R.\n"}}
	want := "## Mission\n\n  M.\n\n## Workflow\nW.\n\n## Never\nN.\n\n\n## Report\nR.\n"
	if got := p.Brief(); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestReadsOverlayReplacesAndClears(t *testing.T) {
	base := Profile{Reads: []string{"AGENTS.md", "README.md"}}
	for content, want := range map[string][]string{
		"schema_version = 1\n":                          {"AGENTS.md", "README.md"},
		"schema_version = 1\nreads = [\"docs/x.md\"]\n": {"docs/x.md"},
		"schema_version = 1\nreads = []\n":              {},
	} {
		f, err := decode([]byte(content))
		if err != nil {
			t.Fatal(err)
		}
		if got := base.overlay(f).Reads; !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: %#v want %#v", content, got, want)
		}
	}
}

func TestProfileJSONCarriesSectionsReadsProtocolAndBrief(t *testing.T) {
	p := builtin(t, "reviewer")
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["role"]; ok {
		t.Fatalf("role still present: %s", b)
	}
	sections, _ := got["sections"].(map[string]any)
	if sections["mission"] != p.Sections.Mission || len(sections) != 6 || got["protocol"] != true || got["brief"] != p.Brief() || got["brief"] == "" {
		t.Fatalf("%s", b)
	}
	if reads, ok := got["reads"].([]any); !ok || len(reads) != len(p.Reads) {
		t.Fatalf("reads: %s", b)
	}
}

func TestBuiltinsRequireProtocolAndMission(t *testing.T) {
	for name, content := range map[string]string{
		"no protocol": "schema_version = 1\n[sections]\nmission = \"M.\"\n",
		"no mission":  "schema_version = 1\nprotocol = true\n[sections]\nnever = \"N.\"\n",
		"extends":     "schema_version = 1\nextends = \"builtin:reviewer\"\nprotocol = true\n[sections]\nmission = \"M.\"\n",
	} {
		if p, err := builtinProfile("x", []byte(content)); err == nil {
			t.Fatalf("%s accepted: %+v", name, p)
		}
	}
	if _, err := builtinProfile("x", []byte("schema_version = 1\nprotocol = true\n[sections]\nmission = \"M.\"\n")); err != nil {
		t.Fatal(err)
	}
}
