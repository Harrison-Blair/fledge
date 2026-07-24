package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/flock"
)

// stubStdoutNotTerminal is stubStdoutTerminal's inverse, for the half of the
// TTY gate captureRun's pipe would otherwise satisfy by accident.
func stubStdoutNotTerminal(t *testing.T) {
	t.Helper()
	original := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdoutIsTerminal = original })
}

// runningFlock scaffolds a workspace with one flock whose daemon is up, and
// moves the test into it.
func runningFlock(t *testing.T) string {
	t.Helper()
	root, _ := scaffoldedWorkspace(t)
	t.Setenv(flock.Env, "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	startDaemon(t, root, "flock1")
	t.Chdir(root)
	return root
}

// There is no scripted path: stop refuses on either half of the gate rather
// than tearing a workspace down for a pipe.
func TestStopRefusesWithoutATerminal(t *testing.T) {
	for name, stub := range map[string]func(*testing.T){
		"stdin":  func(t *testing.T) { stubStdinNotTerminal(t); stubStdoutTerminal(t) },
		"stdout": func(t *testing.T) { stubStdinTerminal(t); stubStdoutNotTerminal(t) },
	} {
		t.Run(name, func(t *testing.T) {
			root := runningFlock(t)
			stub(t)
			withStdin(t, "y\n")

			out, err := captureRun(t, "stop")
			if err == nil {
				t.Fatal("stop succeeded without a terminal")
			}
			if !strings.Contains(err.Error(), "terminal") {
				t.Errorf("error does not mention terminal: %v", err)
			}
			// Refusing to run is a runtime outcome, not a typo in the command.
			if strings.Contains(err.Error(), "usage:") {
				t.Errorf("runtime error unexpectedly contains help:\n%s", err)
			}
			if out != "" {
				t.Errorf("stop listed flocks before refusing:\n%s", out)
			}
			if !client.Running(root, "flock1") {
				t.Error("stop tore a flock down without a terminal")
			}
		})
	}
}

// Nothing to stop is an answer, not a question: no prompt, no error.
func TestStopWithNoFlocksSkipsThePrompt(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "")
	stubStdinTerminal(t)
	stubStdoutTerminal(t)
	withStdin(t, "y\n")

	out, err := captureRun(t, "stop")
	if err != nil {
		t.Fatalf("stop with no flocks: %v", err)
	}
	if !strings.Contains(out, "no flocks") {
		t.Errorf("output missing the no-flocks message:\n%s", out)
	}
	if strings.Contains(out, "[y/N]") {
		t.Errorf("stop prompted with nothing to stop:\n%s", out)
	}
}

func TestStopInsideFlockOnlyOffersAndStopsThatFlock(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	for _, name := range []string{"flock1", "flock2"} {
		if err := os.MkdirAll(flock.Dir(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
	t.Setenv(flock.Env, "flock2")
	stubStdinTerminal(t)
	stubStdoutTerminal(t)
	withStdin(t, "y\n")

	out, err := captureRun(t, "stop")
	if err != nil {
		t.Fatalf("scoped stop: %v\n%s", err, out)
	}
	if !strings.Contains(out, "flock2  down") {
		t.Errorf("scoped stop did not preview its flock:\n%s", out)
	}
	if !strings.Contains(out, "stop flock2 above? [y/N]") {
		t.Errorf("scoped stop used the wrong confirmation:\n%s", out)
	}
	if !strings.Contains(out, "flock flock2: daemon already down") {
		t.Errorf("scoped stop did not execute its flock:\n%s", out)
	}
	if strings.Contains(out, "flock1") || strings.Contains(out, "stop all flocks") {
		t.Errorf("scoped stop leaked into workspace-wide mode:\n%s", out)
	}
}

// Declining a confirmation is a successful outcome, exactly as it is for
// deinit: the operator asked a question and got their answer.
func TestStopDeclineLeavesFlocksRunning(t *testing.T) {
	for name, input := range map[string]string{
		"n":     "n\n",
		"eof":   "",
		"enter": "\n",
		"nope":  "nope\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := runningFlock(t)
			stubStdinTerminal(t)
			stubStdoutTerminal(t)
			withStdin(t, input)

			out, err := captureRun(t, "stop")
			if err != nil {
				t.Fatalf("declined stop returned an error: %v", err)
			}
			if !strings.Contains(out, "aborted") {
				t.Errorf("output missing the abort message:\n%s", out)
			}
			if !strings.Contains(out, "flock1") {
				t.Errorf("output missing the flock listing:\n%s", out)
			}
			if !client.Running(root, "flock1") {
				t.Error("stop tore a flock down after the prompt was declined")
			}
		})
	}
}

// captureStdout collects what fn prints, the helper-level counterpart to
// captureRun's command-level swap.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = original
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// One stuck flock must not strand the rest of the workspace, and the operator
// has to be told which one it was.
func TestStopFlocksKeepsGoingAndReportsPartialFailure(t *testing.T) {
	var asked []string
	var err error
	out := captureStdout(t, func() {
		err = stopFlocks("/ws", []string{"flock1", "flock2", "flock3"}, func(root, name string) error {
			asked = append(asked, name)
			if name == "flock2" {
				return errors.New("flock flock2: daemon is wedged")
			}
			return nil
		})
	})

	if err == nil {
		t.Fatal("stopFlocks succeeded though a flock failed to stop")
	}
	if !strings.Contains(err.Error(), "1 of 3") {
		t.Errorf("error does not count the failures: %v", err)
	}
	if len(asked) != 3 {
		t.Errorf("stopped %v; a failure must not strand the flocks after it", asked)
	}
	if !strings.Contains(out, "flock flock2: daemon is wedged") {
		t.Errorf("output does not name the flock that failed:\n%s", out)
	}
}

func TestStopFlocksSucceedsWhenEveryFlockStops(t *testing.T) {
	err := stopFlocks("/ws", []string{"flock1", "flock2"}, func(root, name string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("stopFlocks: %v", err)
	}
}

// The whole point of the command: one yes tears down every flock in the
// workspace, through the same path flock stop uses.
func TestStopConfirmedTearsDownEveryFlock(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Setenv(flock.Env, "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	recDir := t.TempDir()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, closeSession := liveSocket(t, sock)
	t.Cleanup(closeSession)
	const session = "fledge-testws-abc123-flock1"
	if err := os.WriteFile(filepath.Join(recDir, "session"), []byte(session), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeHerdr(t, recDir, sock)

	// A second flock with state but no daemon, so one run covers both the
	// already-down and the running branch of stopFlock.
	if err := os.MkdirAll(flock.Dir(root, "flock2"), 0o755); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		daemon.RunBound(root, "flock1", session)
		close(done)
	}()
	waitDaemonUp(t, root, "flock1")

	t.Chdir(root)
	stubStdinTerminal(t)
	stubStdoutTerminal(t)
	withStdin(t, "y\n")

	out, err := captureRun(t, "stop")
	if err != nil {
		t.Fatalf("stop: %v\n%s", err, out)
	}
	if !strings.Contains(out, "flock flock1: stopped") {
		t.Errorf("output missing the running flock's teardown:\n%s", out)
	}
	if !strings.Contains(out, "flock flock2: daemon already down") {
		t.Errorf("output missing the down flock's report:\n%s", out)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("daemon still up after stop returned")
	}

	// Reuse of stopFlock, observed rather than asserted structurally: only that
	// path deletes a managed session's record.
	deleted, err := os.ReadFile(filepath.Join(recDir, "deleted"))
	if err != nil {
		t.Fatalf("stop never deleted the managed session record: %v", err)
	}
	if got := string(deleted); got != session {
		t.Errorf("deleted session %q, want %q", got, session)
	}
}
