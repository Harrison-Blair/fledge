// Package hooktest drives scripts/hooks/pre-commit end-to-end against a real
// temporary git repository, exercising it exactly as git would.
package hooktest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hookScriptPath returns the absolute path to scripts/hooks/pre-commit in
// the repo this test package lives in.
func hookScriptPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	// internal/hooktest/precommit_test.go -> repo root is two levels up.
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	return filepath.Join(repoRoot, "scripts", "hooks", "pre-commit")
}

// setupRepo creates a fresh temp git repo, installs the pre-commit hook
// script (copied from the real repo), optionally points core.hooksPath at
// it, and commits an initial clean file so HEAD is valid.
func setupRepo(t *testing.T, configureHooksPath bool) string {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test")

	hooksDir := filepath.Join(dir, "scripts", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	src, err := os.ReadFile(hookScriptPath(t))
	if err != nil {
		t.Fatalf("read source hook script: %v", err)
	}
	dst := filepath.Join(hooksDir, "pre-commit")
	if err := os.WriteFile(dst, src, 0o755); err != nil {
		t.Fatalf("write hook script into temp repo: %v", err)
	}

	if configureHooksPath {
		run(t, dir, "git", "config", "core.hooksPath", "scripts/hooks")
	}

	// A minimal go.mod so `go vet ./...` / `gofmt` run meaningfully rooted
	// at the temp repo.
	writeFile(t, dir, "go.mod", "module hooktestrepo\n\ngo 1.21\n")

	writeFile(t, dir, "initial.go", "package main\n")
	run(t, dir, "git", "add", "initial.go", "go.mod")
	run(t, dir, "git", "commit", "-m", "initial")

	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

// commit stages the given files and runs `git commit`, returning the exit
// code and combined stdout/stderr.
func commit(t *testing.T, dir string, files ...string) (int, string) {
	t.Helper()
	addArgs := append([]string{"add"}, files...)
	run(t, dir, "git", addArgs...)

	cmd := exec.Command("git", "commit", "-m", "test commit")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("git commit failed to run: %v\n%s", err, out)
		}
	}
	return exitCode, string(out)
}

func TestPreCommitHook_BlocksUnformattedFile(t *testing.T) {
	dir := setupRepo(t, true)

	// Deliberately misformatted: no space before '{', unaligned braces.
	bad := "package main\n\nfunc main(){\nx:=1\n_=x\n}\n"
	writeFile(t, dir, "bad.go", bad)

	code, out := commit(t, dir, "bad.go")
	if code == 0 {
		t.Fatalf("expected commit to be blocked, but it succeeded; output:\n%s", out)
	}
	if !strings.Contains(out, "bad.go") {
		t.Errorf("expected hook output to name the offending file bad.go; got:\n%s", out)
	}
}

func TestPreCommitHook_BlocksVetViolation(t *testing.T) {
	dir := setupRepo(t, true)

	// gofmt-clean, but fails go vet: Printf format/argument mismatch.
	vetBad := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Printf(\"%d\\n\", \"hello\")\n}\n"
	writeFile(t, dir, "vetbad.go", vetBad)

	code, out := commit(t, dir, "vetbad.go")
	if code == 0 {
		t.Fatalf("expected commit to be blocked, but it succeeded; output:\n%s", out)
	}
	if !strings.Contains(out, "vetbad.go") {
		t.Errorf("expected hook output to show go vet's diagnostic naming vetbad.go; got:\n%s", out)
	}
}

func TestPreCommitHook_AllowsCleanCommit(t *testing.T) {
	dir := setupRepo(t, true)

	clean := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"
	writeFile(t, dir, "clean.go", clean)

	before, err := os.ReadFile(filepath.Join(dir, "clean.go"))
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	code, out := commit(t, dir, "clean.go")
	if code != 0 {
		t.Fatalf("expected clean commit to succeed; output:\n%s", out)
	}

	after, err := os.ReadFile(filepath.Join(dir, "clean.go"))
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("hook mutated the committed file: before=%q after=%q", before, after)
	}
}

func TestPreCommitHook_NoOpWithoutHooksPathConfigured(t *testing.T) {
	dir := setupRepo(t, false) // core.hooksPath left unset

	bad := "package main\n\nfunc main(){\nx:=1\n_=x\n}\n"
	writeFile(t, dir, "bad.go", bad)

	code, out := commit(t, dir, "bad.go")
	if code != 0 {
		t.Fatalf("expected commit to succeed since core.hooksPath is unset; output:\n%s", out)
	}
}

// ciLintCommands are the literal lint commands PLM-012's CI (pr-check.yml /
// release.yml, see FTHR-022/FTHR-023) runs. The hook must invoke the exact
// same commands.
var ciLintCommands = []string{"gofmt -l .", "go vet ./..."}

func TestPreCommitHook_MatchesCICommands(t *testing.T) {
	src, err := os.ReadFile(hookScriptPath(t))
	if err != nil {
		t.Fatalf("read hook script: %v", err)
	}
	content := string(src)
	for _, want := range ciLintCommands {
		if !strings.Contains(content, want) {
			t.Errorf("hook script does not contain CI-matching command %q", want)
		}
	}
}
