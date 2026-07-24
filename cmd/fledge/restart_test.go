package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/version"
)

type restartStub struct {
	running bool
	status  []protocol.Response

	statusNames   []string
	shutdownNames []string
	spawnRoot     string
	spawnName     string
	spawnSession  string
	spawnErr      error
}

func installRestartStub(t *testing.T, stub *restartStub) {
	t.Helper()
	origStatus := restartDaemonStatus
	origShutdown := restartDaemonShutdown
	origRunning := restartDaemonRunning
	origSpawn := restartSpawnDaemon
	origSleep := restartSleep
	origTimeout := restartWaitTimeout
	restartDaemonStatus = func(root, name string) (protocol.Response, error) {
		stub.statusNames = append(stub.statusNames, name)
		if len(stub.status) == 0 {
			return protocol.Response{}, errors.New("unexpected status call")
		}
		resp := stub.status[0]
		stub.status = stub.status[1:]
		return resp, nil
	}
	restartDaemonShutdown = func(root, name string) error {
		stub.shutdownNames = append(stub.shutdownNames, name)
		stub.running = false
		return nil
	}
	restartDaemonRunning = func(root, name string) bool {
		return stub.running
	}
	restartSpawnDaemon = func(root, name, session string) error {
		stub.spawnRoot = root
		stub.spawnName = name
		stub.spawnSession = session
		if stub.spawnErr != nil {
			return stub.spawnErr
		}
		stub.running = true
		return nil
	}
	restartSleep = func(time.Duration) {}
	restartWaitTimeout = time.Millisecond
	t.Cleanup(func() {
		restartDaemonStatus = origStatus
		restartDaemonShutdown = origShutdown
		restartDaemonRunning = origRunning
		restartSpawnDaemon = origSpawn
		restartSleep = origSleep
		restartWaitTimeout = origTimeout
	})
}

func installSpawnDaemonStatusStub(t *testing.T, replies []protocol.Response) (*int, *int) {
	t.Helper()
	origStatus := spawnDaemonStatus
	origSleep := spawnDaemonSleep
	calls := 0
	sleeps := 0
	spawnDaemonStatus = func(root, name string) (protocol.Response, error) {
		calls++
		if len(replies) == 0 {
			return protocol.Response{}, errors.New("unexpected status call")
		}
		resp := replies[0]
		replies = replies[1:]
		return resp, nil
	}
	spawnDaemonSleep = func(time.Duration) {
		sleeps++
	}
	t.Cleanup(func() {
		spawnDaemonStatus = origStatus
		spawnDaemonSleep = origSleep
	})
	return &calls, &sleeps
}

func TestSpawnDaemonReadinessWaitsForExactBoundSession(t *testing.T) {
	calls, sleeps := installSpawnDaemonStatusStub(t, []protocol.Response{
		{Session: ""},
		{Session: "other-session"},
		{Session: "expected-session"},
	})

	if err := waitSpawnDaemonReady("/workspace", "alpha", "expected-session", "/tmp/fledge.log"); err != nil {
		t.Fatal(err)
	}
	if *calls != 3 {
		t.Fatalf("status calls = %d, want 3", *calls)
	}
	if *sleeps != 2 {
		t.Fatalf("readiness sleeps = %d, want 2", *sleeps)
	}
}

func TestSpawnDaemonReadinessAcceptsUnboundSession(t *testing.T) {
	calls, sleeps := installSpawnDaemonStatusStub(t, []protocol.Response{
		{Session: "bound-session"},
		{Session: ""},
	})

	if err := waitSpawnDaemonReady("/workspace", "alpha", "", "/tmp/fledge.log"); err != nil {
		t.Fatal(err)
	}
	if *calls != 2 {
		t.Fatalf("status calls = %d, want 2", *calls)
	}
	if *sleeps != 1 {
		t.Fatalf("readiness sleeps = %d, want 1", *sleeps)
	}
}

func TestRestartHelpAndRejectsFlags(t *testing.T) {
	out, err := captureRun(t, "restart", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if out != helpPages["restart"] {
		t.Fatalf("restart help = %q, want %q", out, helpPages["restart"])
	}
	if !strings.Contains(rootHelp, "  restart ") {
		t.Fatalf("root help does not advertise restart:\n%s", rootHelp)
	}

	err = run([]string{"restart", "--json"})
	if err == nil || !strings.Contains(err.Error(), helpPages["restart"]) || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("restart flag error = %v, want usage error", err)
	}

	err = run([]string{"restart", "alpha", "bravo"})
	if err == nil || !strings.Contains(err.Error(), helpPages["restart"]) || !strings.Contains(err.Error(), `unexpected argument "bravo"`) {
		t.Fatalf("restart extra arg error = %v, want usage error", err)
	}
}

func TestRestartUsesAmbientFlockAndAllowsUnboundSession(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "alpha")
	stub := &restartStub{
		running: true,
		status: []protocol.Response{
			{DaemonPID: 101, DaemonVersion: "old", Session: ""},
			{DaemonPID: 202, DaemonVersion: version.Get(), Session: ""},
		},
	}
	installRestartStub(t, stub)

	out, err := captureRun(t, "restart")
	if err != nil {
		t.Fatal(err)
	}
	if stub.spawnName != "alpha" || stub.spawnRoot != root || stub.spawnSession != "" {
		t.Fatalf("spawn = root %q name %q session %q, want root %q name alpha empty session",
			stub.spawnRoot, stub.spawnName, stub.spawnSession, root)
	}
	for _, want := range []string{"flock:   alpha", "session: (none)", "old:     pid 101 version old", "new:     pid 202 version " + version.Get()} {
		if !strings.Contains(out, want) {
			t.Errorf("restart output missing %q:\n%s", want, out)
		}
	}
}

func TestRestartExplicitNameOverridesEnvironment(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "ambient")
	stub := &restartStub{
		running: true,
		status: []protocol.Response{
			{DaemonPID: 11, DaemonVersion: "old", Session: "sess"},
			{DaemonPID: 12, DaemonVersion: version.Get(), Session: "sess"},
		},
	}
	installRestartStub(t, stub)

	if _, err := captureRun(t, "restart", "explicit"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(stub.statusNames, ","); got != "explicit,explicit" {
		t.Fatalf("status names = %q, want explicit twice", got)
	}
	if got := strings.Join(stub.shutdownNames, ","); got != "explicit" {
		t.Fatalf("shutdown names = %q, want explicit", got)
	}
	if stub.spawnName != "explicit" || stub.spawnSession != "sess" {
		t.Fatalf("spawn = name %q session %q, want explicit/sess", stub.spawnName, stub.spawnSession)
	}
}

func TestRestartDownReturnsClientNotRunningError(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "alpha")
	stub := &restartStub{running: false}
	installRestartStub(t, stub)

	_, err := captureRun(t, "restart")
	if !errors.Is(err, client.ErrNotRunning) {
		t.Fatalf("restart down error = %v, want %v", err, client.ErrNotRunning)
	}
	if len(stub.statusNames) != 0 || stub.spawnName != "" {
		t.Fatalf("down restart queried status %v or spawned %q", stub.statusNames, stub.spawnName)
	}
}

func TestRestartLegacyShutdownErrorLeavesDaemonRunningWithGuidance(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "alpha")
	stub := &restartStub{
		running: true,
		status:  []protocol.Response{{DaemonPID: 101, DaemonVersion: "old", Session: "sess"}},
	}
	installRestartStub(t, stub)
	restartDaemonShutdown = func(root, name string) error {
		stub.shutdownNames = append(stub.shutdownNames, name)
		return errors.New(`unknown op "shutdown"`)
	}

	_, err := captureRun(t, "restart")
	if err == nil || !strings.Contains(err.Error(), "daemon is still running") ||
		!strings.Contains(err.Error(), "fledge flock stop alpha") ||
		!strings.Contains(err.Error(), "fledge start --flock alpha") {
		t.Fatalf("legacy restart error = %v, want running guidance", err)
	}
	if !stub.running || stub.spawnName != "" {
		t.Fatalf("legacy restart running=%v spawn=%q, want daemon left running and no spawn", stub.running, stub.spawnName)
	}
}

func TestRestartReplacementFailureReportsLogAndLeavesSessionAlone(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "alpha")
	stub := &restartStub{
		running:  true,
		status:   []protocol.Response{{DaemonPID: 101, DaemonVersion: "old", Session: "sess"}},
		spawnErr: errors.New("boom"),
	}
	installRestartStub(t, stub)

	_, err := captureRun(t, "restart")
	wantLog := filepath.Join(flock.Dir(root, "alpha"), protocol.LogName)
	if err == nil || !strings.Contains(err.Error(), "replacement daemon failed") ||
		!strings.Contains(err.Error(), `herdr session "sess" was left running`) ||
		!strings.Contains(err.Error(), wantLog) {
		t.Fatalf("replacement error = %v, want preserved-session log guidance", err)
	}
	if stub.spawnSession != "sess" {
		t.Fatalf("spawn session = %q, want sess", stub.spawnSession)
	}
}

func TestRestartPostSpawnStatusFailureReportsLogAndLeavesSessionAlone(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "alpha")
	stub := &restartStub{running: true}
	installRestartStub(t, stub)
	calls := 0
	restartDaemonStatus = func(root, name string) (protocol.Response, error) {
		calls++
		if calls == 1 {
			return protocol.Response{DaemonPID: 101, DaemonVersion: "old", Session: "sess"}, nil
		}
		return protocol.Response{}, errors.New("status failed")
	}

	_, err := captureRun(t, "restart")
	wantLog := filepath.Join(flock.Dir(root, "alpha"), protocol.LogName)
	if err == nil || !strings.Contains(err.Error(), "replacement daemon failed") ||
		!strings.Contains(err.Error(), `herdr session "sess" was left running`) ||
		!strings.Contains(err.Error(), wantLog) ||
		!strings.Contains(err.Error(), "status failed") {
		t.Fatalf("post-spawn status error = %v, want preserved-session log guidance", err)
	}
	if stub.spawnSession != "sess" {
		t.Fatalf("spawn session = %q, want sess", stub.spawnSession)
	}
}

func TestRestartSamePIDVerificationFailureReportsLogAndLeavesSessionAlone(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "alpha")
	stub := &restartStub{
		running: true,
		status: []protocol.Response{
			{DaemonPID: 101, DaemonVersion: "old", Session: "sess"},
			{DaemonPID: 101, DaemonVersion: version.Get(), Session: "sess"},
		},
	}
	installRestartStub(t, stub)

	_, err := captureRun(t, "restart")
	wantLog := filepath.Join(flock.Dir(root, "alpha"), protocol.LogName)
	if err == nil || !strings.Contains(err.Error(), "replacement daemon failed") ||
		!strings.Contains(err.Error(), `herdr session "sess" was left running`) ||
		!strings.Contains(err.Error(), wantLog) ||
		!strings.Contains(err.Error(), "restart kept daemon pid 101") {
		t.Fatalf("same-pid verification error = %v, want preserved-session log guidance", err)
	}
}

func TestRestartWrongVersionVerificationFailureReportsLogAndLeavesSessionAlone(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "alpha")
	stub := &restartStub{
		running: true,
		status: []protocol.Response{
			{DaemonPID: 101, DaemonVersion: "old", Session: "sess"},
			{DaemonPID: 202, DaemonVersion: "stale", Session: "sess"},
		},
	}
	installRestartStub(t, stub)

	_, err := captureRun(t, "restart")
	wantLog := filepath.Join(flock.Dir(root, "alpha"), protocol.LogName)
	if err == nil || !strings.Contains(err.Error(), "replacement daemon failed") ||
		!strings.Contains(err.Error(), `herdr session "sess" was left running`) ||
		!strings.Contains(err.Error(), wantLog) ||
		!strings.Contains(err.Error(), "new daemon version stale does not match current fledge "+version.Get()) {
		t.Fatalf("wrong-version verification error = %v, want preserved-session log guidance", err)
	}
}

func TestRestartChangedSessionVerificationFailureReportsOldSessionAndLog(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	t.Chdir(root)
	t.Setenv(flock.Env, "alpha")
	stub := &restartStub{
		running: true,
		status: []protocol.Response{
			{DaemonPID: 101, DaemonVersion: "old", Session: "old-sess"},
			{DaemonPID: 202, DaemonVersion: version.Get(), Session: "new-sess"},
		},
	}
	installRestartStub(t, stub)

	_, err := captureRun(t, "restart")
	wantLog := filepath.Join(flock.Dir(root, "alpha"), protocol.LogName)
	if err == nil || !strings.Contains(err.Error(), "replacement daemon failed") ||
		!strings.Contains(err.Error(), `herdr session "old-sess" was left running`) ||
		!strings.Contains(err.Error(), wantLog) ||
		!strings.Contains(err.Error(), `restart changed herdr session from "old-sess" to "new-sess"`) {
		t.Fatalf("changed-session verification error = %v, want old-session log guidance", err)
	}
}
