package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/project"
)

func TestStartAttachesResolvedRunningSession(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	session := project.SessionName(root)
	binary, attachLog, workspaceLog := fakeStartBinary(t, root, session, true, true, 0)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"start", "--herdr-bin", binary}, bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if got := readFile(t, attachLog); got != root+"|session attach "+session+"\n" {
		t.Fatalf("attachment args = %q", got)
	}
	if _, err := os.Stat(workspaceLog + ".picker"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reused session enqueued picker: %v", err)
	}
	if !strings.Contains(stdout.String(), "Already running Fledge session "+session+"\n") ||
		!strings.Contains(stdout.String(), "Session source: derived\n") {
		t.Fatalf("unexpected diagnostics: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Herdr UI closed") {
		t.Fatalf("normal attachment exit reported a coordinated stop: %s", stdout.String())
	}
}

func TestFreshDetachedStartSkipsOrchestratorPicker(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	session := project.SessionName(root)
	binary, attachLog, workspaceLog := fakeStartBinary(t, root, session, false, false, 0)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"start", "--detach", "--herdr-bin", binary, "--timeout", "2s",
	}, bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	for _, path := range []string{attachLog, workspaceLog + ".picker"} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("detached start unexpectedly used %s: %v", path, err)
		}
	}
}

func TestStartNewSessionAttachesAfterReadiness(t *testing.T) {
	root := initializedProject(t)
	nested := filepath.Join(root, "src", "component")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	session := project.SessionName(root)
	binary, attachLog, workspaceLog := fakeStartBinary(t, root, session, false, false, 0)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"start", "--herdr-bin", binary, "--timeout", "2s",
	}, bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if got := readFile(t, attachLog); got != nested+"|session attach "+session+"\n" {
		t.Fatalf("attachment args = %q", got)
	}
	if got := readFile(t, workspaceLog); got != nested+"\n" {
		t.Fatalf("initial workspace cwd = %q, want %q", got, nested)
	}
	pickerCommand := readFile(t, workspaceLog+".picker")
	if strings.Count(pickerCommand, "\n") != 1 ||
		!strings.HasPrefix(pickerCommand, "pane.send_input|pane-created|") ||
		!strings.Contains(pickerCommand, " agent spawn --name fledge-orchestrator|enter\n") {
		t.Fatalf("orchestrator picker command = %q", pickerCommand)
	}
	if !strings.Contains(stdout.String(), "Started Fledge session "+session+"\n") {
		t.Fatalf("unexpected diagnostics: %s", stdout.String())
	}
}

func TestFreshStartPickerFailureWarnsAndStillAttaches(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	session := project.SessionName(root)
	binary, attachLog, _ := fakeStartBinary(
		t, root, session, false, false, 0, "pane.send_input",
	)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"start", "--herdr-bin", binary, "--timeout", "2s",
	}, bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if got := readFile(t, attachLog); got != root+"|session attach "+session+"\n" {
		t.Fatalf("attachment args = %q", got)
	}
	if !strings.Contains(stderr.String(), "Warning: could not open orchestrator picker:") ||
		!strings.Contains(stderr.String(), "injected pane.send_input failure") {
		t.Fatalf("missing picker warning: %s", stderr.String())
	}
}

func TestStartDetachPreservesHumanAndJSONOutputWithoutAttaching(t *testing.T) {
	tests := []struct {
		name string
		json bool
	}{
		{name: "human"},
		{name: "json", json: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := initializedProject(t)
			t.Chdir(root)
			session := project.SessionName(root)
			binary, attachLog, _ := fakeStartBinary(t, root, session, true, true, 0)
			args := []string{"start", "--detach", "--herdr-bin", binary}
			if test.json {
				args = append(args, "--json")
			}

			var stdout, stderr bytes.Buffer
			code := Execute(context.Background(), args, bytes.NewBuffer(nil), &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr.String())
			}
			if _, err := os.Stat(attachLog); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("detach invoked attachment: %v", err)
			}
			if !test.json {
				if !strings.Contains(stdout.String(), "Already running Fledge session "+session+"\n") ||
					!strings.Contains(stdout.String(), "Session source: derived\n") {
					t.Fatalf("unexpected diagnostics: %s", stdout.String())
				}
				return
			}
			var envelope struct {
				SchemaVersion int                 `json:"schema_version"`
				OK            bool                `json:"ok"`
				Data          fledgeStartEnvelope `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.SchemaVersion != 1 || !envelope.OK ||
				envelope.Data.Session != session || envelope.Data.Started {
				t.Fatalf("unexpected envelope: %#v", envelope)
			}
		})
	}
}

func TestStartReportsAttachFailureAfterDiagnostics(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	session := project.SessionName(root)
	binary, _, _ := fakeStartBinary(t, root, session, true, true, 9)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"start", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "Error [attach_failed]") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Already running Fledge session "+session+"\n") {
		t.Fatalf("start diagnostics were not printed before failure: %s", stdout.String())
	}
}

func TestStartSetupFailurePreventsAttachment(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	binary, attachLog, _ := fakeStartBinary(
		t, root, project.SessionName(root), true, true, 0, "pane.split",
	)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"start", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "Error [session_setup_failed]") ||
		!strings.Contains(stderr.String(), "pane.split") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(attachLog); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attachment ran after setup failure: %v", err)
	}
}

func TestStartDetachStillPreparesOrchestratorLayout(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	binary, attachLog, _ := fakeStartBinary(
		t, root, project.SessionName(root), true, true, 0, "pane.split",
	)

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"start", "--detach", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "Error [session_setup_failed]") ||
		!strings.Contains(stderr.String(), "pane.split") {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(attachLog); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("detach invoked attachment: %v", err)
	}
}

func TestConcurrentStopMakesAttachedStartACleanExit(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	session := project.SessionName(root)
	binary, attached, _ := fakeCoordinatedStopBinary(t, root, session)

	var startOut, startErr bytes.Buffer
	startDone := make(chan int, 1)
	go func() {
		startDone <- Execute(context.Background(), []string{"start", "--herdr-bin", binary},
			bytes.NewBuffer(nil), &startOut, &startErr)
	}()
	waitForFile(t, attached)

	var stopOut, stopErr bytes.Buffer
	stopCode := Execute(context.Background(), []string{"stop", "--herdr-bin", binary},
		bytes.NewBuffer(nil), &stopOut, &stopErr)
	if stopCode != 0 {
		t.Fatalf("stop exit=%d stderr=%s", stopCode, stopErr.String())
	}
	select {
	case startCode := <-startDone:
		if startCode != 0 {
			t.Fatalf("start exit=%d stderr=%s", startCode, startErr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("attached start did not exit after stop")
	}
	if !strings.Contains(startOut.String(), "Fledge session "+session+" stopped; Herdr UI closed.\n") {
		t.Fatalf("missing coordinated closure message: %s", startOut.String())
	}
}

func TestAttachFailureWithUnreadableStopStateIsActionable(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	binary, attached, running := fakeCoordinatedStopBinary(t, root, project.SessionName(root))

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- Execute(context.Background(), []string{"start", "--herdr-bin", binary},
			bytes.NewBuffer(nil), &stdout, &stderr)
	}()
	waitForFile(t, attached)

	stateFiles, err := filepath.Glob(filepath.Join(os.Getenv("XDG_STATE_HOME"), "fledge", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sessionState string
	for _, path := range stateFiles {
		if filepath.Base(path) != "associations.json" {
			sessionState = path
			break
		}
	}
	if sessionState == "" {
		t.Fatal("session state file was not created before attachment")
	}
	if err := os.WriteFile(sessionState, []byte("{broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(running); err != nil {
		t.Fatal(err)
	}

	select {
	case code := <-done:
		if code != 1 || !strings.Contains(stderr.String(), "Error [state_unavailable]") ||
			!strings.Contains(stderr.String(), "could not inspect coordinated-stop state") {
			t.Fatalf("exit=%d stderr=%s", code, stderr.String())
		}
		if strings.Contains(stdout.String(), "Herdr UI closed") {
			t.Fatalf("state failure was reported as an intentional stop: %s", stdout.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("start did not report the state inspection failure")
	}
}
