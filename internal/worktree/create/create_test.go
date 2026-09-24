package create

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
)

type call = herdrscript.Call

func repository(t *testing.T) string {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=Test", "-c", "user.email=t@example.com", "commit", "-qm", "initial", "--allow-empty"}} {
		if b, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
	return root
}

func listing(root string) herdr.WorktreeListResult {
	l := herdr.WorktreeListResult{Type: "worktree_list", Worktrees: []herdr.Worktree{}}
	l.Source.RepoRoot = root
	return l
}

func created(path string) herdr.CreatedResult {
	p := herdrscript.Pane("w3:p1", "w3", "w3:t1")
	b := "topic"
	return herdr.CreatedResult{Type: "worktree_created", Workspace: herdr.Workspace{ID: "w3"}, Tab: herdr.Tab{ID: "w3:t1", WorkspaceID: "w3"}, RootPane: p, Worktree: herdr.Worktree{Path: path, Branch: &b}}
}

// checkout stands in for Herdr creating the linked checkout as worktree.create
// is served.
func checkout(t *testing.T, root, branch, path string) func() {
	return func() {
		if b, err := exec.Command("git", "-C", root, "worktree", "add", "-q", "-b", branch, path).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, b)
		}
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
	return string(b)
}

func TestCreatesManagedCheckout(t *testing.T) {
	root := repository(t)
	sub := filepath.Join(root, "sub")
	os.Mkdir(sub, 0o755)
	path := filepath.Join(root, ".fledge", "worktrees", "topic")
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Params: map[string]any{"cwd": root}, Result: listing(root)},
		call{Method: "worktree.create", Params: map[string]any{"cwd": root, "branch": "topic", "base": "main", "path": path, "focus": false}, Result: created(path), Before: checkout(t, root, "topic", path)},
	), Options{Branch: "topic", Base: "main", Cwd: sub})
	if out.Status != "success" || out.Operation != "worktree.create" {
		t.Fatalf("%+v", out)
	}
	if r := out.Result.(Result); r != (Result{Path: path, Branch: "topic", WorkspaceID: "w3"}) {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge", ".gitignore")); err != nil {
		t.Fatal("managed directory not prepared")
	}
	n := len(out.Effects)
	if n < 4 || out.Effects[n-4] != (libagent.Effect{Action: "created", Kind: "worktree", Path: path}) || out.Effects[n-3] != (libagent.Effect{Action: "created", Kind: "workspace", ID: "w3"}) {
		t.Fatalf("%+v", out.Effects)
	}
}

// The new checkout gets the managed .fledge/.gitignore, so scratch files under
// .fledge/tmp stay ignored even when an allowlist root .gitignore re-includes
// directories and Markdown, while profile files can still be added.
func TestCreatedCheckoutIgnoresFledgeScratch(t *testing.T) {
	root := repository(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*\n!*/\n!*.md\n!.gitignore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".gitignore")
	git(t, root, "-c", "user.name=Test", "-c", "user.email=t@example.com", "commit", "-qm", "allowlist")
	path := filepath.Join(root, ".fledge", "worktrees", "topic")
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: listing(root)},
		call{Method: "worktree.create", Result: created(path), Before: checkout(t, root, "topic", path)},
	), Options{Branch: "topic", Cwd: root})
	if out.Status != "success" {
		t.Fatalf("%+v", out)
	}
	ignore := filepath.Join(path, ".fledge", ".gitignore")
	if b, err := os.ReadFile(ignore); err != nil || string(b) != "*\n!/profiles/\n!/profiles/*.toml\n" {
		t.Fatalf("%q %v", b, err)
	}
	n := len(out.Effects)
	if n < 2 || out.Effects[n-2] != (libagent.Effect{Action: "created", Kind: "directory", Path: filepath.Join(path, ".fledge")}) || out.Effects[n-1] != (libagent.Effect{Action: "created", Kind: "file", Path: ignore}) {
		t.Fatalf("%+v", out.Effects)
	}
	for _, dir := range []string{"tmp", "profiles"} {
		if err := os.MkdirAll(filepath.Join(path, ".fledge", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(path, ".fledge", "tmp", "report.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := git(t, path, "status", "--short"); s != "" {
		t.Fatalf("status %q", s)
	}
	git(t, path, "check-ignore", "-q", ".fledge/tmp/report.md")
	if err := os.WriteFile(filepath.Join(path, ".fledge", "profiles", "p.toml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := git(t, path, "status", "--short", "--untracked-files=all"); s != "?? .fledge/profiles/p.toml\n" {
		t.Fatalf("status %q", s)
	}
	git(t, path, "add", ".fledge/profiles/p.toml")
}

// A checkout Herdr created that cannot take the managed ignore file leaves a
// partial outcome naming the checkout, since creation cannot be undone.
func TestIgnoreFailureAfterCreateIsPartial(t *testing.T) {
	root := repository(t)
	path := filepath.Join(root, ".fledge", "worktrees", "topic")
	create := checkout(t, root, "topic", path)
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: listing(root)},
		call{Method: "worktree.create", Result: created(path), Before: func() {
			create()
			if err := os.WriteFile(filepath.Join(path, ".fledge"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	), Options{Branch: "topic", Cwd: root})
	if out.Status != "partial" || out.Error == nil || out.Error.Phase != "worktree.ignore" || !strings.Contains(out.Error.Message, path) {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if r, ok := out.Result.(Result); !ok || r.Path != path {
		t.Fatalf("%+v", out.Result)
	}
	if n := len(out.Effects); n < 2 || out.Effects[n-2].Kind != "worktree" || out.Effects[n-1].Kind != "workspace" {
		t.Fatalf("%+v", out.Effects)
	}
}

func TestRefusesExistingBranch(t *testing.T) {
	root := repository(t)
	if err := exec.Command("git", "-C", root, "branch", "topic").Run(); err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), herdrscript.Client(t, call{Method: "worktree.list", Result: listing(root)}), Options{Branch: "topic", Cwd: root})
	if out.ExitCode() != 2 || out.Status != "rejected" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge")); !os.IsNotExist(err) {
		t.Fatal("prepared before refusing")
	}
}

func TestRequiresBranch(t *testing.T) {
	out := Run(context.Background(), herdrscript.Client(t), Options{})
	if out.ExitCode() != 2 {
		t.Fatalf("%+v", out)
	}
}

func TestOutsideRepository(t *testing.T) {
	out := Run(context.Background(), herdrscript.Client(t), Options{Branch: "topic", Cwd: t.TempDir()})
	if out.ExitCode() != 1 || out.Status != "rejected" {
		t.Fatalf("%+v", out)
	}
}

func TestCreateFailureIsUnknownWhenUncertain(t *testing.T) {
	root := repository(t)
	out := Run(context.Background(), herdrscript.Client(t,
		call{Method: "worktree.list", Result: listing(root)},
		call{Method: "worktree.create", Result: map[string]any{"type": "worktree_created"}},
	), Options{Branch: "topic", Cwd: root})
	if out.Status != "unknown" || out.Error.Phase != "worktree.create" {
		t.Fatalf("%+v", out)
	}
}

func TestRender(t *testing.T) {
	r := Result{Path: "/r/.fledge/worktrees/topic", Branch: "topic", WorkspaceID: "w3"}
	var b bytes.Buffer
	if err := (libagent.Outcome{Status: "success", Result: r}).Write(&b, false, Render); err != nil {
		t.Fatal(err)
	}
	if want := "Created worktree /r/.fledge/worktrees/topic on branch topic in workspace w3.\n"; b.String() != want {
		t.Fatalf("%q", b.String())
	}
	herdrscript.CheckOutputFailures(t, Render, libagent.Outcome{Result: r})
}
