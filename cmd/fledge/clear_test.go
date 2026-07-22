package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func stubClearOperations(t *testing.T, running func(root, name string) bool, removeAll func(string) error) {
	t.Helper()
	originalRunning := clearFlockRunning
	originalSession := clearFlockSession
	originalRemoveAll := clearFlockRemoveAll
	originalOrphans := clearFlockOrphans
	originalOrphanSession := clearOrphanSession
	clearFlockRunning = running
	clearFlockSession = func(root, name string) error { return nil }
	clearFlockRemoveAll = removeAll
	clearFlockOrphans = func(root string) ([]string, error) { return nil, nil }
	clearOrphanSession = func(name string) error { return nil }
	t.Cleanup(func() {
		clearFlockRunning = originalRunning
		clearFlockSession = originalSession
		clearFlockRemoveAll = originalRemoveAll
		clearFlockOrphans = originalOrphans
		clearOrphanSession = originalOrphanSession
	})
}

func savedFlock(t *testing.T, root, name string) string {
	t.Helper()
	dir := flock.Dir(root, name)
	if err := os.MkdirAll(filepath.Join(dir, ".rpc"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"journal.jsonl", "daemon.log", ".rpc/request"} {
		if err := os.WriteFile(filepath.Join(dir, filename), []byte("state"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func interactiveClear(t *testing.T, input string) {
	t.Helper()
	stubStdinTerminal(t)
	stubStdoutTerminal(t)
	withStdin(t, input)
}

func TestFlockClearHelpAndSyntax(t *testing.T) {
	out, err := captureRun(t, "flock", "clear", "--help")
	if err != nil || out != helpPages["flock clear"] {
		t.Fatalf("clear help = %q, %v", out, err)
	}
	if !strings.Contains(helpPages["flock"], "clear [name]") {
		t.Fatalf("flock help does not advertise clear:\n%s", helpPages["flock"])
	}

	for name, args := range map[string][]string{
		"flag":    {"flock", "clear", "--json"},
		"name":    {"flock", "clear", "Bad"},
		"excess":  {"flock", "clear", "alpha", "bravo"},
		"unknown": {"flock", "clear", "-Q"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := captureRun(t, args...)
			if err == nil {
				t.Fatal("invalid clear invocation succeeded")
			}
			if !strings.Contains(err.Error(), helpPages["flock clear"]) {
				t.Errorf("usage error missing clear help:\n%v", err)
			}
		})
	}
}

func TestFlockClearRefusesWithoutBothTerminals(t *testing.T) {
	for name, stub := range map[string]func(*testing.T){
		"stdin": func(t *testing.T) {
			stubStdinNotTerminal(t)
			stubStdoutTerminal(t)
		},
		"stdout": func(t *testing.T) {
			stubStdinTerminal(t)
			stubStdoutNotTerminal(t)
		},
	} {
		t.Run(name, func(t *testing.T) {
			root, _ := scaffoldedWorkspace(t)
			dir := savedFlock(t, root, "alpha")
			t.Chdir(root)
			stub(t)
			withStdin(t, "y\n")

			out, err := captureRun(t, "flock", "clear")
			if err == nil || !strings.Contains(err.Error(), "terminal") {
				t.Fatalf("non-terminal clear = %q, %v", out, err)
			}
			if out != "" {
				t.Errorf("clear previewed state before refusing:\n%s", out)
			}
			if _, err := os.Stat(dir); err != nil {
				t.Errorf("clear modified state after refusing: %v", err)
			}
		})
	}
}

func TestFlockClearNoMatchingStateSkipsPrompt(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	interactiveClear(t, "y\n")
	stubClearOperations(t, func(root, name string) bool { return false }, os.RemoveAll)

	for name, args := range map[string][]string{
		"empty": {"flock", "clear"},
		"named": {"flock", "clear", "missing"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := captureRun(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out, "[y/N]") {
				t.Errorf("clear prompted with no matching state:\n%s", out)
			}
			if !strings.Contains(out, "no saved") {
				t.Errorf("clear did not report the no-op:\n%s", out)
			}
		})
	}
}

func TestFlockClearAlsoRemovesProjectOrphanSessions(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	interactiveClear(t, "yes\n")
	stubClearOperations(t, func(root, name string) bool { return false }, os.RemoveAll)
	orphans := []string{flock.SessionPrefix(root) + "flock2", flock.SessionPrefix(root) + "flock1"}
	clearFlockOrphans = func(gotRoot string) ([]string, error) {
		if gotRoot != canonical(t, root) {
			t.Fatalf("orphan root = %q, want %q", gotRoot, canonical(t, root))
		}
		return orphans, nil
	}
	var removed []string
	clearOrphanSession = func(name string) error {
		removed = append(removed, name)
		return nil
	}

	out, err := captureRun(t, "flock", "clear")
	if err != nil {
		t.Fatalf("orphan clear: %v\n%s", err, out)
	}
	if !slices.Equal(removed, orphans) {
		t.Fatalf("removed sessions = %v, want %v", removed, orphans)
	}
	for _, name := range orphans {
		if !strings.Contains(out, name+"  orphan managed herdr session") ||
			!strings.Contains(out, "herdr session "+name+": cleared") {
			t.Errorf("orphan output missing %q:\n%s", name, out)
		}
	}
}

func TestFlockClearOrphanSessionsRequireConfirmation(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	interactiveClear(t, "n\n")
	stubClearOperations(t, func(root, name string) bool { return false }, os.RemoveAll)
	orphan := flock.SessionPrefix(root) + "flock9"
	clearFlockOrphans = func(root string) ([]string, error) { return []string{orphan}, nil }
	removed := false
	clearOrphanSession = func(name string) error {
		removed = true
		return nil
	}

	out, err := captureRun(t, "flock", "clear")
	if err != nil {
		t.Fatal(err)
	}
	if removed || !strings.Contains(out, orphan) || !strings.Contains(out, "aborted") {
		t.Fatalf("declined orphan clear = removed %v\n%s", removed, out)
	}
}

func TestManagedOrphanSessionsAreScopedToProjectAndSavedFlocks(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	savedFlock(t, root, "linked")
	otherRoot, _ := scaffoldedWorkspace(t)
	bin := t.TempDir()
	linked := flock.SessionName(root, "linked")
	orphan := flock.SessionName(root, "orphan")
	other := flock.SessionName(otherRoot, "orphan")
	script := `#!/bin/sh
printf '{"sessions":[{"name":"` + linked + `"},{"name":"` + orphan + `"},{"name":"` + other + `"},{"name":"operator-session"}]}'
`
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := managedOrphanSessions(canonical(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{orphan}) {
		t.Fatalf("managed orphans = %v, want only %q", got, orphan)
	}
}

func TestFlockClearDefaultNoLeavesState(t *testing.T) {
	for name, input := range map[string]string{
		"n": "n\n", "eof": "", "empty": "\n", "other": "okay\n",
	} {
		t.Run(name, func(t *testing.T) {
			root, _ := scaffoldedWorkspace(t)
			dir := savedFlock(t, root, "alpha")
			t.Chdir(root)
			interactiveClear(t, input)
			stubClearOperations(t, func(root, name string) bool { return false }, os.RemoveAll)

			out, err := captureRun(t, "flock", "clear")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "alpha  down") || !strings.Contains(out, "aborted") {
				t.Errorf("declined clear output:\n%s", out)
			}
			if _, err := os.Stat(dir); err != nil {
				t.Errorf("declined clear removed state: %v", err)
			}
		})
	}
}

func TestFlockClearNamedRemovesOnlyRequestedState(t *testing.T) {
	for _, input := range []string{"y\n", "yes\n"} {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			root, _ := scaffoldedWorkspace(t)
			alpha := savedFlock(t, root, "alpha")
			bravo := savedFlock(t, root, "bravo")
			t.Setenv(flock.Env, "bravo")
			t.Chdir(root)
			interactiveClear(t, input)
			stubClearOperations(t, func(root, name string) bool { return false }, os.RemoveAll)

			out, err := captureRun(t, "flock", "clear", "alpha")
			if err != nil {
				t.Fatalf("named clear: %v\n%s", err, out)
			}
			if !strings.Contains(out, "flock alpha: cleared") || strings.Contains(out, "bravo  down") {
				t.Errorf("named clear output:\n%s", out)
			}
			if _, err := os.Stat(alpha); !os.IsNotExist(err) {
				t.Errorf("alpha still exists: %v", err)
			}
			if _, err := os.Stat(bravo); err != nil {
				t.Errorf("bravo was removed: %v", err)
			}
		})
	}
}

func TestBareFlockClearIgnoresEnvironmentAndClearsInSortedOrder(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		savedFlock(t, root, name)
	}
	t.Setenv(flock.Env, "charlie")
	t.Chdir(root)
	interactiveClear(t, "yes\n")
	stubClearOperations(t, func(root, name string) bool { return false }, os.RemoveAll)

	out, err := captureRun(t, "flock", "clear")
	if err != nil {
		t.Fatalf("bulk clear: %v\n%s", err, out)
	}
	last := -1
	for _, want := range []string{"alpha", "bravo", "charlie"} {
		at := strings.Index(out, want)
		if at <= last {
			t.Errorf("preview is not sorted at %q:\n%s", want, out)
		}
		last = at
	}
	names, err := flock.List(root)
	if err != nil || len(names) != 0 {
		t.Fatalf("saved flocks after clear = %v, %v", names, err)
	}
	parent := filepath.Join(root, scaffold.DirName, flock.DirName)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		t.Fatalf("flocks parent was not preserved: %v", err)
	}
}

func TestFlockClearSkipsRunningAndClearsRemainingTargets(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	alpha := savedFlock(t, root, "alpha")
	bravo := savedFlock(t, root, "bravo")
	charlie := savedFlock(t, root, "charlie")
	t.Chdir(root)
	interactiveClear(t, "y\n")
	stubClearOperations(t, func(root, name string) bool { return name == "bravo" }, os.RemoveAll)

	out, err := captureRun(t, "flock", "clear")
	if err == nil || !strings.Contains(err.Error(), "1 of 3") {
		t.Fatalf("mixed clear = %v\n%s", err, out)
	}
	for _, want := range []string{"flock alpha: cleared", "flock bravo: skipped", "flock charlie: cleared"} {
		if !strings.Contains(out, want) {
			t.Errorf("mixed clear missing %q:\n%s", want, out)
		}
	}
	for path, exists := range map[string]bool{alpha: false, bravo: true, charlie: false} {
		_, statErr := os.Stat(path)
		if exists && statErr != nil || !exists && !os.IsNotExist(statErr) {
			t.Errorf("state %s existence = %v, want %v", path, statErr, exists)
		}
	}
}

func TestFlockClearRechecksBeforeDeletion(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	dir := savedFlock(t, root, "alpha")
	t.Chdir(root)
	interactiveClear(t, "y\n")
	checks := 0
	removed := false
	stubClearOperations(t, func(root, name string) bool {
		checks++
		return checks == 2
	}, func(path string) error {
		removed = true
		return os.RemoveAll(path)
	})

	out, err := captureRun(t, "flock", "clear", "alpha")
	if err == nil || !strings.Contains(out, "skipped") {
		t.Fatalf("raced clear = %v\n%s", err, out)
	}
	if checks != 2 || removed {
		t.Fatalf("liveness checks = %d, remove called = %v", checks, removed)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("raced flock state was removed: %v", err)
	}
}

func TestFlockClearContinuesAfterDeletionFailure(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	alpha := savedFlock(t, root, "alpha")
	bravo := savedFlock(t, root, "bravo")
	t.Chdir(root)
	interactiveClear(t, "y\n")
	stubClearOperations(t, func(root, name string) bool { return false }, func(path string) error {
		if filepath.Base(path) == "alpha" {
			return errors.New("permission denied")
		}
		return os.RemoveAll(path)
	})

	out, err := captureRun(t, "flock", "clear")
	if err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("failed clear = %v\n%s", err, out)
	}
	if !strings.Contains(out, "flock alpha: failed to clear: permission denied") || !strings.Contains(out, "flock bravo: cleared") {
		t.Errorf("failure output:\n%s", out)
	}
	if _, err := os.Stat(alpha); err != nil {
		t.Errorf("failed target disappeared: %v", err)
	}
	if _, err := os.Stat(bravo); !os.IsNotExist(err) {
		t.Errorf("later target was not cleared: %v", err)
	}
}

func TestFlockClearRemovesManagedSessionBeforeLocalState(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	dir := savedFlock(t, root, "alpha")
	var order []string
	err := clearFlocks(
		root,
		[]string{"alpha"},
		func(root, name string) bool { return false },
		func(gotRoot, name string) error {
			if gotRoot != root || flock.SessionName(gotRoot, name) != flock.SessionName(root, "alpha") {
				t.Fatalf("session cleanup = %q, %q", gotRoot, name)
			}
			order = append(order, "session")
			return nil
		},
		func(path string) error {
			if path != dir {
				t.Fatalf("remove path = %q, want %q", path, dir)
			}
			order = append(order, "state")
			return os.RemoveAll(path)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "session,state" {
		t.Fatalf("cleanup order = %v", order)
	}
}

func TestFlockClearPreservesStateWhenManagedSessionCleanupFails(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	alpha := savedFlock(t, root, "alpha")
	bravo := savedFlock(t, root, "bravo")
	err := clearFlocks(
		root,
		[]string{"alpha", "bravo"},
		func(root, name string) bool { return false },
		func(root, name string) error {
			if name == "alpha" {
				return errors.New("herdr refused")
			}
			return nil
		},
		os.RemoveAll,
	)
	if err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("clear error = %v", err)
	}
	if _, err := os.Stat(alpha); err != nil {
		t.Errorf("state disappeared after session cleanup failure: %v", err)
	}
	if _, err := os.Stat(bravo); !os.IsNotExist(err) {
		t.Errorf("later state was not cleared: %v", err)
	}
}

func TestClearedNameIsAvailableToMintAgain(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	savedFlock(t, root, "flock1")
	if err := clearFlocks(root, []string{"flock1"}, func(root, name string) bool { return false }, func(root, name string) error { return nil }, os.RemoveAll); err != nil {
		t.Fatal(err)
	}
	name, err := flock.Mint(root)
	if err != nil || name != "flock1" {
		t.Fatalf("mint after clear = %q, %v", name, err)
	}
}
