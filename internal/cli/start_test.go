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

	"github.com/Harrison-Blair/fledge/internal/agentprofile"
	"github.com/Harrison-Blair/fledge/internal/project"
)

func TestStartAttachesResolvedRunningSession(t *testing.T) {
	root := initializedProject(t)
	if err := os.MkdirAll(project.TempDir(root), 0o700); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(project.TempDir(root), "keep")
	if err := os.WriteFile(kept, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	session := project.SessionName(root)
	binary, attachLog, workspaceLog := fakeStartBinary(t, root, session,
		startOptions{Running: true, WorkspacePresent: true})

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
		!strings.Contains(stdout.String(), "Session source: derived\n") ||
		!strings.Contains(stdout.String(), "Temp: "+project.TempDir(root)+"\n") ||
		!strings.Contains(stdout.String(), "Temp cleanup: preserved (session already running)\n") {
		t.Fatalf("unexpected diagnostics: %s", stdout.String())
	}
	if data, err := os.ReadFile(kept); err != nil || string(data) != "preserve" {
		t.Fatalf("running-session temp content changed: data=%q err=%v", data, err)
	}
	if strings.Contains(stdout.String(), "Herdr UI closed") {
		t.Fatalf("normal attachment exit reported a coordinated stop: %s", stdout.String())
	}
}

func TestFreshDetachedStartSkipsOrchestratorPicker(t *testing.T) {
	root := initializedProject(t)
	ignore := filepath.Join(root, ".fledge", ".gitignore")
	if err := os.WriteFile(ignore, []byte("/logs/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(project.TempDir(root), "nested", "stale")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("discard"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	session := project.SessionName(root)
	binary, attachLog, workspaceLog := fakeStartBinary(t, root, session, startOptions{})

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
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh start preserved stale temp content: %v", err)
	}
	info, err := os.Stat(project.TempDir(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("fresh temp directory mode = %v", info.Mode())
	}
	if !strings.Contains(stdout.String(), "Temp cleanup: cleaned before start\n") {
		t.Fatalf("fresh cleanup was not reported: %s", stdout.String())
	}
	ignoreData, err := os.ReadFile(ignore)
	if err != nil || string(ignoreData) != "/logs/\n/tmp/\n" {
		t.Fatalf("fresh start did not update runtime ignores: %q, %v", ignoreData, err)
	}
}

func TestFreshDetachedStartJSONReportsTempCleanup(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	session := project.SessionName(root)
	binary, _, _ := fakeStartBinary(t, root, session, startOptions{})

	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), []string{
		"start", "--detach", "--json", "--herdr-bin", binary, "--timeout", "2s",
	}, bytes.NewBuffer(nil), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var envelope struct {
		SchemaVersion int                 `json:"schema_version"`
		OK            bool                `json:"ok"`
		Data          fledgeStartEnvelope `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != 1 || !envelope.OK || envelope.Data.Session != session ||
		!envelope.Data.Started || !envelope.Data.TempCleaned || envelope.Data.TempDir != project.TempDir(root) {
		t.Fatalf("unexpected envelope: %#v", envelope)
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
	binary, attachLog, workspaceLog := fakeStartBinary(t, root, session, startOptions{})

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
	if strings.Contains(stderr.String(), "could not open orchestrator picker") {
		t.Fatalf("orchestrator picker was rejected: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Started Fledge session "+session+"\n") {
		t.Fatalf("unexpected diagnostics: %s", stdout.String())
	}
}

func TestFreshAttachedStartInjectsUsableOrchestratorProfile(t *testing.T) {
	root := initializedProject(t)
	store, err := agentprofile.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(agentprofile.Profile{
		Name: "orchestrator", SchemaVersion: 1, Instructions: "Use inherited Fledge only.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	session := project.SessionName(root)
	binary, _, workspaceLog := fakeStartBinary(t, root, session, startOptions{})
	env := &environment{
		in: bytes.NewBuffer(nil), out: &bytes.Buffer{}, errOut: &bytes.Buffer{},
		cwd: root, stateDir: t.TempDir(), herdrBin: binary,
		lookPath: func(name string) (string, error) {
			if name == "codex" {
				return "/bin/codex", nil
			}
			return "", os.ErrNotExist
		},
	}
	cmd := newStart(env)
	cmd.SetArgs([]string{"--timeout", "2s"})
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	command := readFile(t, workspaceLog+".picker")
	if !strings.Contains(command, " agent spawn orchestrator --name fledge-orchestrator|enter\n") {
		t.Fatalf("profile startup command = %q", command)
	}
}

func TestStartupOrchestratorProfileFallbacks(t *testing.T) {
	write := func(t *testing.T, root, contents string, mode os.FileMode) string {
		t.Helper()
		dir := filepath.Join(root, ".fledge", "profiles")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "orchestrator.toml")
		if err := os.WriteFile(path, []byte(contents), mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	valid := "schema_version = 1\ninstructions = \"Managed.\"\n"
	tests := []struct {
		name      string
		prepare   func(*testing.T, string)
		installed map[string]bool
		want      string
		wantWarn  bool
	}{
		{name: "missing"},
		{name: "usable", prepare: func(t *testing.T, root string) { write(t, root, valid, 0o600) },
			installed: map[string]bool{"claude": true}, want: "orchestrator"},
		{name: "malformed", prepare: func(t *testing.T, root string) { write(t, root, "schema_version = [\n", 0o600) }, wantWarn: true},
		{name: "unreadable", prepare: func(t *testing.T, root string) {
			path := write(t, root, valid, 0o600)
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
		}, installed: map[string]bool{"codex": true}, wantWarn: true},
		{name: "Pi-only instructions profile", prepare: func(t *testing.T, root string) { write(t, root, valid, 0o600) },
			installed: map[string]bool{"pi": true}, want: "orchestrator"},
		{name: "context-backed OpenCode", prepare: func(t *testing.T, root string) { write(t, root, valid, 0o600) },
			installed: map[string]bool{"opencode": true}, want: "orchestrator"},
		{name: "no installed harness", prepare: func(t *testing.T, root string) { write(t, root, valid, 0o600) }, wantWarn: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := initializedProject(t)
			if test.prepare != nil {
				test.prepare(t, root)
			}
			env := &environment{lookPath: func(name string) (string, error) {
				if test.installed[name] {
					return "/bin/" + name, nil
				}
				return "", os.ErrNotExist
			}}
			got, err := startupOrchestratorProfile(env, root)
			if got != test.want || (err != nil) != test.wantWarn {
				t.Fatalf("profile/warning = %q / %v, want %q / warning=%t", got, err, test.want, test.wantWarn)
			}
		})
	}
}

func TestFreshAttachedStartWarnsAndFallsBackForInvalidOrchestratorProfile(t *testing.T) {
	root := initializedProject(t)
	profiles := filepath.Join(root, ".fledge", "profiles")
	if err := os.MkdirAll(profiles, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profiles, "orchestrator.toml"), []byte("schema_version = [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := project.SessionName(root)
	binary, attachLog, workspaceLog := fakeStartBinary(t, root, session, startOptions{})
	var stdout, stderr bytes.Buffer
	env := &environment{
		in: bytes.NewBuffer(nil), out: &stdout, errOut: &stderr,
		cwd: root, stateDir: t.TempDir(), herdrBin: binary,
		lookPath: func(name string) (string, error) { return "/bin/" + name, nil },
	}
	cmd := newStart(env)
	cmd.SetArgs([]string{"--timeout", "2s"})
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, workspaceLog+".picker"); !strings.Contains(got, " agent spawn --name fledge-orchestrator|enter\n") ||
		strings.Contains(got, " agent spawn orchestrator ") {
		t.Fatalf("fallback startup command = %q", got)
	}
	if got := readFile(t, attachLog); got != root+"|session attach "+session+"\n" {
		t.Fatalf("attachment args = %q", got)
	}
	if !strings.Contains(stderr.String(), "Warning: could not use orchestrator profile; opening ad-hoc picker:") ||
		!strings.Contains(stderr.String(), "load orchestrator profile") {
		t.Fatalf("fallback warning = %q", stderr.String())
	}
}

func TestFreshStartPickerFailureWarnsAndStillAttaches(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	session := project.SessionName(root)
	binary, attachLog, _ := fakeStartBinary(t, root, session, startOptions{SetupFailure: "pane.send_input"})

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
			binary, attachLog, _ := fakeStartBinary(t, root, session,
				startOptions{Running: true, WorkspacePresent: true})
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
				envelope.Data.Session != session || envelope.Data.Started || envelope.Data.TempCleaned ||
				envelope.Data.TempDir != project.TempDir(root) {
				t.Fatalf("unexpected envelope: %#v", envelope)
			}
		})
	}
}

func TestStartReportsAttachFailureAfterDiagnostics(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	session := project.SessionName(root)
	binary, _, _ := fakeStartBinary(t, root, session,
		startOptions{Running: true, WorkspacePresent: true, AttachExit: 9})

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
	binary, attachLog, _ := fakeStartBinary(t, root, project.SessionName(root), startOptions{
		Running: true, WorkspacePresent: true, SetupFailure: "pane.split",
	})

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
	binary, attachLog, _ := fakeStartBinary(t, root, project.SessionName(root), startOptions{
		Running: true, WorkspacePresent: true, SetupFailure: "pane.split",
	})

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
	enableStopCleanupHelperProcess(t)
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
	enableStopCleanupHelperProcess(t)
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
