package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/proposal"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
)

func repository(t *testing.T) string {
	t.Helper()
	root := gittest.Repository(t)
	gittest.Commit(t, root)
	return root
}

// write creates .fledge/profiles/file under root with content.
func write(t *testing.T, root, file, content string) string {
	t.Helper()
	path := filepath.Join(root, ".fledge", "profiles", file)
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

var roles = []string{"debugger", "implementer", "integrator", "orchestrator", "planner", "researcher", "reviewer", "verifier"}

// Built-ins are eight Markdown role briefs; the shared protocol is not one.
func TestBuiltinsShipEightMarkdownRoles(t *testing.T) {
	list, err := List(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	distinct := map[string]bool{}
	for _, p := range list {
		names = append(names, p.Name)
		if p.Source != "builtin" || p.Path != nil {
			t.Fatalf("provenance: %+v", p)
		}
		if strings.TrimSpace(p.Role) == "" || distinct[p.Role] || !strings.HasPrefix(p.Role, "## Mission\n") {
			t.Fatalf("role not a distinct Markdown brief: %q", p.Role)
		}
		distinct[p.Role] = true
	}
	if !slices.Equal(names, roles) {
		t.Fatal(names)
	}
	if _, err := Load(context.Background(), t.TempDir(), "protocol"); err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Fatalf("protocol is selectable: %v", err)
	}
	for _, c := range []struct{ name, phrase string }{
		{"verifier", "fledge task verify"},
		{"reviewer", "Edit files"},
		{"planner", "fledge task import --dry-run"},
		{"planner", "a short Markdown report under `.fledge/tmp/plans/`"},
		{"planner", "`.fledge/tmp/plans/<task-id>.toml` (your own task id)"},
		{"planner", "Never run the real import; the orchestrator imports."},
		{"planner", "`fledge task complete --id <task-id> --file <report>`"},
		{"integrator", "main"},
		{"implementer", "Do not run `fledge task complete`"},
		{"debugger", "Do not run `fledge task complete`"},
		{"orchestrator", "fledge agent cleanup"},
	} {
		if !strings.Contains(builtin(t, c.name).Role, c.phrase) {
			t.Errorf("%s lost %q", c.name, c.phrase)
		}
	}
}

// Structured reads became ordinary conditional reading instructions.
func TestBuiltinRolesReadRepositoryInstructionsIfPresent(t *testing.T) {
	for _, name := range roles {
		role := builtin(t, name).Role
		if !strings.Contains(role, "`AGENTS.md`") || !strings.Contains(role, "if present") {
			t.Errorf("%s does not ask to read AGENTS.md if present:\n%s", name, role)
		}
	}
	for _, name := range []string{"orchestrator", "planner"} {
		if !strings.Contains(builtin(t, name).Role, "`README.md`") {
			t.Errorf("%s dropped README.md", name)
		}
	}
}

// Built-ins ship to every repository, so they name no repository's branch or
// backlog: the integration branch comes from the brief, else the documented
// fledge.baseBranch, origin/HEAD, main fallback.
func TestBuiltinRolesNameNoRepositoryBranch(t *testing.T) {
	dev := regexp.MustCompile(`\bdev\b`)
	for _, name := range roles {
		role := builtin(t, name).Role
		if dev.MatchString(role) || strings.Contains(role, "Touch main") || strings.Contains(role, "backlog check-off") {
			t.Errorf("%s names a repository-specific branch or backlog:\n%s", name, role)
		}
	}
	for _, name := range []string{"integrator", "orchestrator", "implementer", "debugger"} {
		if !strings.Contains(builtin(t, name).Role, "integration branch") {
			t.Errorf("%s does not name the integration branch", name)
		}
	}
	for _, name := range []string{"integrator", "orchestrator"} {
		role := builtin(t, name).Role
		for _, want := range []string{"fledge.baseBranch", "origin/HEAD", "stop"} {
			if !strings.Contains(role, want) {
				t.Errorf("%s lacks %q", name, want)
			}
		}
	}
}

// Built-in guidance matches current Fledge: spawn waits for readiness before
// sending the first prompt, profiles choose no harness, and task brief
// headings are advisory.
func TestBuiltinGuidanceIsCurrent(t *testing.T) {
	for _, name := range roles {
		brief := builtin(t, name).Brief()
		for _, stale := range []string{"agent_not_ready", "rejects the first prompt", "six-heading"} {
			if strings.Contains(brief, stale) {
				t.Errorf("%s still says %q", name, stale)
			}
		}
		for _, quoted := range strings.Split(brief, "`")[1:] {
			if strings.HasPrefix(quoted, "fledge agent spawn ") && !strings.Contains(quoted, "--harness ") {
				t.Errorf("%s: %q lacks --harness", name, quoted)
			}
		}
	}
	orchestrator := builtin(t, "orchestrator").Role
	for _, want := range []string{"`fledge agent spawn --harness ", "fledge task template", "advisory"} {
		if !strings.Contains(orchestrator, want) {
			t.Errorf("orchestrator lacks %q", want)
		}
	}
	if planner := builtin(t, "planner").Role; !strings.Contains(planner, "advisory") {
		t.Error("planner does not call template headings advisory")
	}
}

// The brief is the role text byte for byte, then the shared protocol once.
func TestBriefIsRoleThenSharedProtocolOnce(t *testing.T) {
	for role, want := range map[string]string{
		"R.\n":            "R.\n\n## Fledge protocol\n" + shared,
		"R.":              "R.\n\n## Fledge protocol\n" + shared,
		"\n  R. \\n\n\n":  "\n  R. \\n\n\n\n## Fledge protocol\n" + shared,
		"# Own heading\n": "# Own heading\n\n## Fledge protocol\n" + shared,
	} {
		if got := (Profile{Role: role}).Brief(); got != want {
			t.Errorf("%q: got %q\nwant %q", role, got, want)
		}
	}
	if !strings.Contains(shared, "fledge agent current") || !strings.HasSuffix(shared, ".\n") {
		t.Fatalf("shared block not embedded: %q", shared)
	}
	for _, name := range roles {
		if b := builtin(t, name).Brief(); strings.Count(b, shared) != 1 || strings.Count(b, "## Fledge protocol") != 1 {
			t.Errorf("%s protocol not exactly once", name)
		}
	}
}

func TestRepositoryMarkdownReplacesBuiltinWholeFile(t *testing.T) {
	root := repository(t)
	text := "\n- Run `fledge task list --json`; use C:\\dir and a*b.  \r\n  1. Nested.\n\n"
	path := write(t, root, "reviewer.md", text)
	p := load(t, root, "reviewer")
	if p.Role != text || p.Source != "repo" || p.Path == nil || *p.Path != path {
		t.Fatalf("%+v", p)
	}
	if strings.Contains(p.Brief(), builtin(t, "reviewer").Role) || !strings.HasPrefix(p.Brief(), text) {
		t.Fatalf("built-in text survived: %q", p.Brief())
	}
}

func TestCustomMarkdownProfilesAreAdded(t *testing.T) {
	root := repository(t)
	path := write(t, root, "go-review.md", "Review Go.\n")
	write(t, root, "notes.txt", "ignored")
	write(t, root, "reviewer.md", "Local.\n")
	if p := load(t, root, "go-review"); p.Role != "Review Go.\n" || p.Source != "repo" || *p.Path != path {
		t.Fatalf("%+v", p)
	}
	list, err := List(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Profile{}
	for _, p := range list {
		byName[p.Name] = p
	}
	if names := slices.Collect(maps.Keys(byName)); len(list) != 9 || byName["go-review"].Role != "Review Go.\n" || byName["reviewer"].Role != "Local.\n" || byName["verifier"].Source != "builtin" {
		t.Fatal(names)
	}
	if !slices.IsSortedFunc(list, func(a, b Profile) int { return strings.Compare(a.Name, b.Name) }) {
		t.Fatal("not sorted")
	}
}

func TestInvalidMarkdownProfilesFailWithoutFallback(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"empty", "", "must not be blank"},
		{"blank", " \n\t\r\n", "must not be blank"},
		{"invalid utf-8", "a\xffb", "must be valid UTF-8"},
		{"nul", "a\x00b", "must not contain NUL"},
		{"invalid utf-8 and nul", "\x00\xff", "must be valid UTF-8"},
		{"nul and blank", " \x00 ", "must not contain NUL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := repository(t)
			path := write(t, root, "reviewer.md", tc.content)
			checkRejected(t, root, "reviewer", path, tc.want)
			_, err := Load(context.Background(), root, "reviewer")
			if want := "profile " + path + ": " + tc.want; err.Error() != want {
				t.Fatalf("got %q, want %q", err, want)
			}
		})
	}
	t.Run("directory", func(t *testing.T) {
		root := repository(t)
		path := filepath.Join(root, ".fledge", "profiles", "reviewer.md")
		os.MkdirAll(path, 0755)
		checkRejected(t, root, "reviewer", path, "regular file")
	})
	t.Run("symlink", func(t *testing.T) {
		root := repository(t)
		target := write(t, root, "real.txt", "Role.\n")
		path := filepath.Join(filepath.Dir(target), "reviewer.md")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		checkRejected(t, root, "reviewer", path, "regular file")
	})
}

// checkRejected asserts that loading name and listing both fail with invalid
// input naming path and want.
func checkRejected(t *testing.T, root, name, path, want string) {
	t.Helper()
	p, err := Load(context.Background(), root, name)
	if err == nil {
		t.Fatalf("accepted: %+v", p)
	}
	if !errors.As(err, new(*cli.InputError)) || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), path) {
		t.Fatalf("%T %v", err, err)
	}
	if _, err := List(context.Background(), root); err == nil || !errors.As(err, new(*cli.InputError)) || !strings.Contains(err.Error(), path) {
		t.Fatalf("listing: %v", err)
	}
}

// Legacy TOML profiles are a clean break: selecting one, listing any, or a
// same-name Markdown and TOML pair fails with migration guidance.
func TestLegacyTOMLProfilesAreRejected(t *testing.T) {
	legacy := "schema_version = 1\nharness = \"claude\"\n"
	for _, name := range []string{"reviewer", "scout"} {
		t.Run("selected "+name, func(t *testing.T) {
			root := repository(t)
			path := write(t, root, name+".toml", legacy)
			checkRejected(t, root, name, path, "legacy TOML profile")
			_, err := Load(context.Background(), root, name)
			for _, want := range []string{name + ".md", "--harness", "--model"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("guidance lacks %q: %v", want, err)
				}
			}
		})
	}
	t.Run("same-name conflict", func(t *testing.T) {
		root := repository(t)
		write(t, root, "reviewer.md", "Role.\n")
		path := write(t, root, "reviewer.toml", legacy)
		checkRejected(t, root, "reviewer", path, "conflicts with "+filepath.Join(root, ".fledge", "profiles", "reviewer.md"))
	})
	t.Run("unselected legacy fails only listing", func(t *testing.T) {
		root := repository(t)
		path := write(t, root, "Old Name.toml", legacy)
		if p := load(t, root, "reviewer"); p.Source != "builtin" {
			t.Fatalf("%+v", p)
		}
		if _, err := List(context.Background(), root); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "legacy TOML profile") {
			t.Fatalf("listing: %v", err)
		}
	})
}

func TestInvalidNamesAndMissingProfilesAreRejected(t *testing.T) {
	root := repository(t)
	os.MkdirAll(filepath.Join(root, ".fledge", "profiles", "dir.md"), 0755)
	for _, name := range []string{"", "../reviewer", "Reviewer", "a/b", "missing", "dir", "protocol"} {
		if p, err := Load(context.Background(), root, name); err == nil || !errors.As(err, new(*cli.InputError)) {
			t.Fatalf("%q: %+v %v", name, p, err)
		}
	}
	root = repository(t)
	path := write(t, root, "Bad.md", "Role.\n")
	if _, err := List(context.Background(), root); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), ".md") {
		t.Fatalf("listing accepted a bad name: %v", err)
	}
}

func TestProfilesComeFromTheInvokingCheckout(t *testing.T) {
	root := repository(t)
	linked := filepath.Join(t.TempDir(), "linked")
	gittest.Git(t, root, "worktree", "add", "-qb", "linked", linked)
	os.MkdirAll(filepath.Join(linked, "sub"), 0755)
	write(t, root, "reviewer.md", "primary\n")
	write(t, linked, "reviewer.md", "linked\n")
	for cwd, want := range map[string]string{root: "primary\n", linked: "linked\n", filepath.Join(linked, "sub"): "linked\n"} {
		if p := load(t, cwd, "reviewer"); p.Role != want {
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

// Inspection JSON carries only identity, provenance, and the brief, which
// includes the shared protocol but never runtime memory.
func TestProfileJSONHasNameSourcePathAndBrief(t *testing.T) {
	root := repository(t)
	path := write(t, root, "scout.md", "Explore.\n")
	for _, p := range []Profile{builtin(t, "reviewer"), load(t, root, "scout")} {
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		keys := slices.Sorted(maps.Keys(got))
		brief, _ := got["brief"].(string)
		if !slices.Equal(keys, []string{"brief", "name", "path", "source"}) || brief != p.Brief() || !strings.Contains(brief, shared) || strings.Contains(brief, "Project memory\n") {
			t.Fatalf("%s", b)
		}
		if p.Source == "repo" && got["path"] != path || p.Source == "builtin" && got["path"] != nil {
			t.Fatalf("%s", b)
		}
	}
}

// TestBuiltinBriefsMatchTaskCommands keeps built-in briefs runnable: every
// quoted `fledge task complete` invocation names its task, and the planner
// asks for nothing the proposal format would reject.
func TestBuiltinBriefsMatchTaskCommands(t *testing.T) {
	list, err := List(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		for _, quoted := range strings.Split(p.Brief(), "`")[1:] {
			if args, ok := strings.CutPrefix(quoted, "fledge task complete "); ok && !strings.Contains(args, "--id ") {
				t.Errorf("%s: %q lacks --id", p.Name, quoted)
			}
		}
	}
	planner := builtin(t, "planner").Brief()
	if strings.Contains(planner, "size") {
		t.Errorf("planner asks for a size, which proposals reject: %s", planner)
	}
	if _, err := proposal.Decode([]byte(proposal.Skeleton() + "size = \"S\"\n")); err == nil || !strings.Contains(err.Error(), `unknown key "size"`) {
		t.Error("proposals accept a size key; the planner check above is stale")
	}
}

// The shared protocol points workers at the injected memory index and at
// recording durable facts with the memory commands.
func TestSharedProtocolDirectsWorkersToProjectMemory(t *testing.T) {
	for _, want := range []string{"Project memory", "`fledge memory add`"} {
		if !strings.Contains(shared, want) {
			t.Errorf("protocol lacks %q:\n%s", want, shared)
		}
	}
}
