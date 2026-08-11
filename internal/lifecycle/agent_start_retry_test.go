package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

const (
	paneReadinessHelperModeEnv  = "FLEDGE_LIFECYCLE_PANE_READINESS_HELPER"
	paneReadinessHelperReadyEnv = "FLEDGE_LIFECYCLE_PANE_READINESS_READY"
)

func TestMain(m *testing.M) {
	switch os.Getenv(paneReadinessHelperModeEnv) {
	case "exit":
		os.Exit(23)
	case "block":
		if err := os.WriteFile(os.Getenv(paneReadinessHelperReadyEnv), nil, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		<-time.After(time.Hour)
	}
	os.Exit(m.Run())
}

type paneReadinessTriggeredContext struct {
	context.Context
	done  chan struct{}
	cause error
}

func (c *paneReadinessTriggeredContext) Done() <-chan struct{} { return c.done }
func (c *paneReadinessTriggeredContext) Err() error {
	select {
	case <-c.done:
		return c.cause
	default:
		return nil
	}
}

func paneReadinessHelperBinary(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test helper binary: %v", err)
	}
	return path
}

func realPaneReadinessExitError(t *testing.T) *exec.ExitError {
	t.Helper()
	command := exec.Command(paneReadinessHelperBinary(t))
	command.Env = append(os.Environ(), paneReadinessHelperModeEnv+"=exit")
	err := command.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("pane readiness helper error = %T %v, want exit status 23", err, err)
	}
	return exitErr
}

func realPaneReadinessDeadlineError(t *testing.T) error {
	t.Helper()
	ready := t.TempDir() + "/ready"
	binary := paneReadinessHelperBinary(t)
	t.Setenv(paneReadinessHelperModeEnv, "block")
	t.Setenv(paneReadinessHelperReadyEnv, ready)
	ctx := &paneReadinessTriggeredContext{
		Context: context.Background(),
		done:    make(chan struct{}),
		cause:   context.DeadlineExceeded,
	}
	result := make(chan error, 1)
	go func() {
		result <- herdr.NewClient(binary, nil, nil, nil).
			WaitPaneOutput(ctx, "session-name", "w1:p2", 5*time.Second)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("blocking pane readiness helper did not start")
		}
		time.Sleep(time.Millisecond)
	}
	close(ctx.done)

	select {
	case err := <-result:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("pane readiness helper did not stop after deadline")
		return nil
	}
}

func herdrCommandError(code string) error {
	return &herdr.CommandError{
		Code:      code,
		Message:   "scripted " + code,
		RequestID: "req-" + code,
		Err:       errors.New("exit status 1"),
	}
}

func transientSpawnManager(t *testing.T) (*Manager, *fakeHerdr, string) {
	t.Helper()
	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{
		sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot:    testSnapshot(),
		createdTab:  herdr.Tab{TabID: "t2", WorkspaceID: "w1", Label: "worker"},
		createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"},
	}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	return manager, client, root
}

func expirePaneReadinessImmediately(manager *Manager) {
	manager.paneReadinessContext = func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
		return context.WithDeadline(parent, time.Unix(1, 0))
	}
}

func countLifecycleCall(calls []string, target string) int {
	count := 0
	for _, call := range calls {
		if call == target {
			count++
		}
	}
	return count
}

func assertIdenticalAgentStarts(t *testing.T, calls []startAgentCall) {
	t.Helper()
	if len(calls) != 2 {
		t.Fatalf("StartAgent() calls = %#v, want exactly two", calls)
	}
	if !reflect.DeepEqual(calls[0], calls[1]) {
		t.Fatalf("StartAgent() retry changed arguments: first %#v, second %#v", calls[0], calls[1])
	}
}

func TestSpawnRetriesOnlyTransientBusyAfterPaneOutput(t *testing.T) {
	t.Parallel()
	manager, client, root := transientSpawnManager(t)
	client.startErrs = []error{herdrCommandError("agent_pane_busy"), nil}
	records := captureSessionLog(manager)

	err := manager.Spawn(context.Background(), root, SpawnOptions{
		Timeout: 30 * time.Second, Name: "worker", Harness: "codex", Model: "gpt-custom", ModelSet: true,
		NativeArgs: []string{"--sandbox", "read-only"}, Task: "review the diff",
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	assertIdenticalAgentStarts(t, client.startAgentCalls)
	if len(client.waitPaneCalls) != 1 || client.waitPaneCalls[0] != (waitPaneOutputCall{
		session: testSessionName, pane: "w1:p2", timeout: 5 * time.Second,
	}) {
		t.Fatalf("WaitPaneOutput() calls = %#v", client.waitPaneCalls)
	}
	if countLifecycleCall(client.calls, "create-tab") != 1 || len(client.promptCalls) != 1 || len(client.closeCalls) != 0 {
		t.Fatalf("launch side effects: calls=%v prompts=%v closes=%v", client.calls, client.promptCalls, client.closeCalls)
	}
	wantCalls := []string{
		"check", "list", "snapshot", "create-tab", "rename-pane", "start-agent",
		"wait-pane-output", "start-agent", "focus-agent", "prompt-agent",
	}
	if !reflect.DeepEqual(client.calls, wantCalls) {
		t.Fatalf("launch call order = %v, want %v", client.calls, wantCalls)
	}

	store := messaging.New(root, testSessionName)
	agents, agentErr := store.Agents()
	tasks, taskErr := store.Tasks()
	wakes, wakeErr := store.PendingWakes()
	if agentErr != nil || taskErr != nil || wakeErr != nil {
		t.Fatalf("read launch ledger: agents=%v tasks=%v wakes=%v", agentErr, taskErr, wakeErr)
	}
	if len(agents) != 1 || !agents[0].Active || agents[0].Name != "worker" {
		t.Fatalf("agents = %#v, want one active worker", agents)
	}
	if len(tasks) != 1 || tasks[0].Description != "review the diff" || tasks[0].Status != messaging.TaskActive {
		t.Fatalf("tasks = %#v, want one active initial task", tasks)
	}
	if len(wakes) != 1 || wakes[0].Kind != "task-assigned" || wakes[0].ReferenceID != tasks[0].ID {
		t.Fatalf("wakes = %#v, want one assignment wake", wakes)
	}

	decoded := decodeLogRecords(t, records)
	if !hasLogRecord(decoded, "DEBUG", "agent start recovered after pane readiness wait") {
		t.Fatalf("recovery log is not debug-level: %v", decoded)
	}
	for _, record := range decoded {
		if record["msg"] == "agent start recovered after pane readiness wait" && record["level"] != "DEBUG" {
			t.Fatalf("recovery emitted at non-debug level: %v", record)
		}
	}
}

func TestSpawnReadinessTimeoutStillAllowsOneFinalStart(t *testing.T) {
	t.Parallel()
	manager, client, root := transientSpawnManager(t)
	client.startErrs = []error{herdrCommandError("agent_pane_busy"), nil}
	client.waitPaneErr = herdrCommandError("timeout")

	if err := manager.Spawn(context.Background(), root, SpawnOptions{
		Timeout: 4 * time.Second, Name: "worker", Harness: "codex",
	}); err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	assertIdenticalAgentStarts(t, client.startAgentCalls)
	if len(client.waitPaneCalls) != 1 || client.waitPaneCalls[0].timeout != 4*time.Second {
		t.Fatalf("WaitPaneOutput() calls = %#v, want startup-timeout budget", client.waitPaneCalls)
	}
}

func TestSpawnBusyReadinessFailureDoesNotRetry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		configure func(context.CancelFunc, *fakeHerdr)
	}{
		{
			name: "fatal readiness error",
			configure: func(_ context.CancelFunc, client *fakeHerdr) {
				client.waitPaneErr = errors.New("pane output unavailable")
			},
		},
		{
			name: "caller cancellation",
			configure: func(cancel context.CancelFunc, client *fakeHerdr) {
				client.waitPaneHook = func(ctx context.Context) error {
					cancel()
					return ctx.Err()
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, client, root := transientSpawnManager(t)
			client.startErrs = []error{herdrCommandError("agent_pane_busy"), nil}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			test.configure(cancel, client)

			err := manager.Spawn(ctx, root, SpawnOptions{Timeout: 30 * time.Second, Name: "worker", Harness: "codex"})
			if err == nil {
				t.Fatal("Spawn() error = nil")
			}
			if test.name == "caller cancellation" && !errors.Is(err, context.Canceled) {
				t.Fatalf("Spawn() error = %v, want context.Canceled in chain", err)
			}
			if len(client.startAgentCalls) != 1 || len(client.waitPaneCalls) != 1 || len(client.closeCalls) != 1 {
				t.Fatalf("calls after readiness failure: starts=%#v waits=%#v closes=%#v", client.startAgentCalls, client.waitPaneCalls, client.closeCalls)
			}
			if len(client.promptCalls) != 0 {
				t.Fatalf("PromptAgent() calls = %#v, want none", client.promptCalls)
			}
		})
	}
}

func TestSpawnChildReadinessDeadlineDoesNotMaskFatalWaitErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		waitErr        error
		structuredCode string
	}{
		{name: "plain fatal", waitErr: errors.New("pane output unavailable")},
		{name: "structured fatal", waitErr: herdrCommandError("input_failed"), structuredCode: "input_failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, client, root := transientSpawnManager(t)
			expirePaneReadinessImmediately(manager)
			client.startErrs = []error{herdrCommandError("agent_pane_busy"), nil}
			client.waitPaneHook = func(ctx context.Context) error {
				<-ctx.Done()
				return test.waitErr
			}

			err := manager.Spawn(context.Background(), root, SpawnOptions{
				Timeout: 4 * time.Second, Name: "worker", Harness: "codex",
			})
			if err == nil || !strings.Contains(err.Error(), test.waitErr.Error()) {
				t.Fatalf("Spawn() error = %v, want fatal wait error %v", err, test.waitErr)
			}
			if len(client.startAgentCalls) != 1 || len(client.waitPaneCalls) != 1 || len(client.closeCalls) != 1 {
				t.Fatalf("calls after child deadline: starts=%#v waits=%#v closes=%#v", client.startAgentCalls, client.waitPaneCalls, client.closeCalls)
			}
			if test.structuredCode != "" {
				var commandErr *herdr.CommandError
				if !errors.As(err, &commandErr) || !herdr.IsErrorCode(err, test.structuredCode) {
					t.Fatalf("Spawn() error = %T %v, want structured code %q", err, err, test.structuredCode)
				}
			}
		})
	}
}

func TestPaneReadinessTimeoutClassificationIsJoinOrderSafe(t *testing.T) {
	t.Parallel()
	timeoutErr := herdrCommandError("timeout")
	fatalErr := herdrCommandError("input_failed")
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "exact Herdr timeout", err: timeoutErr, want: true},
		{name: "bare deadline lacks command provenance", err: context.DeadlineExceeded},
		{name: "wrapped deadline lacks command provenance", err: fmt.Errorf("wait command: %w", context.DeadlineExceeded)},
		{name: "wrong case Herdr timeout", err: herdrCommandError("Timeout")},
		{name: "parent-style cancellation", err: context.Canceled},
		{name: "plain fatal", err: errors.New("pane output unavailable")},
		{name: "deadline before plain fatal", err: errors.Join(context.DeadlineExceeded, errors.New("pane output unavailable"))},
		{name: "plain fatal before deadline", err: errors.Join(errors.New("pane output unavailable"), context.DeadlineExceeded)},
		{name: "deadline before structured fatal", err: errors.Join(context.DeadlineExceeded, fatalErr)},
		{name: "structured fatal before deadline", err: errors.Join(fatalErr, context.DeadlineExceeded)},
		{name: "timeout before structured fatal", err: errors.Join(timeoutErr, fatalErr)},
		{name: "structured fatal before timeout", err: errors.Join(fatalErr, timeoutErr)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := paneReadinessTimedOut(test.err); got != test.want {
				t.Fatalf("paneReadinessTimedOut(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}

func TestIndependentProcessExitNeverAuthorizesPaneReadinessRetry(t *testing.T) {
	exitErr := realPaneReadinessExitError(t)
	timeoutErr := herdrCommandError("timeout")
	tests := []struct {
		name string
		err  error
	}{
		{name: "exit before deadline", err: errors.Join(exitErr, context.DeadlineExceeded)},
		{name: "deadline before exit", err: errors.Join(context.DeadlineExceeded, exitErr)},
		{name: "exit before structured timeout", err: errors.Join(exitErr, timeoutErr)},
		{name: "structured timeout before exit", err: errors.Join(timeoutErr, exitErr)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if paneReadinessTimedOut(test.err) {
				t.Fatalf("paneReadinessTimedOut(%v) = true, want false", test.err)
			}

			manager, client, root := transientSpawnManager(t)
			client.startErrs = []error{herdrCommandError("agent_pane_busy"), nil}
			client.waitPaneErr = test.err
			err := manager.Spawn(context.Background(), root, SpawnOptions{
				Timeout: 4 * time.Second, Name: "worker", Harness: "codex",
			})
			if err == nil {
				t.Fatal("Spawn() error = nil")
			}
			if len(client.startAgentCalls) != 1 {
				t.Fatalf("StartAgent() calls = %#v, want exactly one", client.startAgentCalls)
			}
		})
	}
}

func TestSpawnBusyTwiceReturnsContextAndOneJoinedCleanup(t *testing.T) {
	t.Parallel()
	manager, client, root := transientSpawnManager(t)
	client.startErrs = []error{herdrCommandError("agent_pane_busy"), herdrCommandError("agent_pane_busy")}
	cleanupErr := errors.New("close failed")
	client.closeErr = cleanupErr

	err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: 30 * time.Second, Name: "worker", Harness: "codex"})
	if err == nil || !strings.Contains(err.Error(), `agent "worker" pane "w1:p2" remained unavailable after one readiness retry`) {
		t.Fatalf("Spawn() error = %v, want final retry context", err)
	}
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("Spawn() error = %v, want joined cleanup error %v", err, cleanupErr)
	}
	assertIdenticalAgentStarts(t, client.startAgentCalls)
	if len(client.waitPaneCalls) != 1 || len(client.closeCalls) != 1 {
		t.Fatalf("wait/cleanup calls = %#v/%#v, want one each", client.waitPaneCalls, client.closeCalls)
	}
}

func TestSpawnFinalFailureRetainsReadinessTimeoutDiagnostic(t *testing.T) {
	t.Parallel()
	manager, client, root := transientSpawnManager(t)
	client.startErrs = []error{herdrCommandError("agent_pane_busy"), errors.New("final launch failed")}
	client.waitPaneErr = herdrCommandError("timeout")

	err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: 30 * time.Second, Name: "worker", Harness: "codex"})
	if err == nil || !strings.Contains(err.Error(), "remained unavailable after one readiness retry") ||
		!strings.Contains(err.Error(), "pane readiness wait reached its 5s deadline") ||
		!strings.Contains(err.Error(), "final launch failed") {
		t.Fatalf("Spawn() error = %v, want final failure and readiness-timeout diagnostic", err)
	}
	if len(client.startAgentCalls) != 2 || len(client.waitPaneCalls) != 1 || len(client.closeCalls) != 1 {
		t.Fatalf("calls = starts %#v, waits %#v, closes %#v", client.startAgentCalls, client.waitPaneCalls, client.closeCalls)
	}
}

func TestSpawnRetriesProvenanceMarkedLocalReadinessDeadline(t *testing.T) {
	readinessErr := realPaneReadinessDeadlineError(t)
	if !paneReadinessTimedOut(readinessErr) {
		t.Fatalf("paneReadinessTimedOut(%v) = false, want true for marked local deadline", readinessErr)
	}
	var exitErr *exec.ExitError
	if !errors.Is(readinessErr, context.DeadlineExceeded) || !errors.As(readinessErr, &exitErr) {
		t.Fatalf("local readiness error = %T %v, want deadline and raw process error", readinessErr, readinessErr)
	}

	manager, client, root := transientSpawnManager(t)
	client.startErrs = []error{herdrCommandError("agent_pane_busy"), nil}
	client.waitPaneErr = readinessErr
	if err := manager.Spawn(context.Background(), root, SpawnOptions{
		Timeout: 4 * time.Second, Name: "worker", Harness: "codex",
	}); err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	assertIdenticalAgentStarts(t, client.startAgentCalls)
}

func TestLocalReadinessDeadlineDoesNotMaskJoinedFatalSibling(t *testing.T) {
	readinessErr := realPaneReadinessDeadlineError(t)
	if !paneReadinessTimedOut(readinessErr) {
		t.Fatalf("paneReadinessTimedOut(%v) = false, want true for marked local deadline", readinessErr)
	}

	genericFatal := errors.New("pane output unavailable")
	structuredFatal := herdrCommandError("input_failed")
	tests := []struct {
		name           string
		fatal          error
		structuredCode string
	}{
		{name: "generic fatal", fatal: genericFatal},
		{name: "structured non-timeout fatal", fatal: structuredFatal, structuredCode: "input_failed"},
	}
	orders := []struct {
		name string
		join func(error, error) error
	}{
		{name: "readiness marker before fatal", join: func(readiness, fatal error) error { return errors.Join(readiness, fatal) }},
		{name: "fatal before readiness marker", join: func(readiness, fatal error) error { return errors.Join(fatal, readiness) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, order := range orders {
				t.Run(order.name, func(t *testing.T) {
					waitErr := order.join(readinessErr, test.fatal)
					if paneReadinessTimedOut(waitErr) {
						t.Fatalf("paneReadinessTimedOut(%v) = true, want false", waitErr)
					}

					manager, client, root := transientSpawnManager(t)
					client.startErrs = []error{herdrCommandError("agent_pane_busy"), nil}
					client.waitPaneErr = waitErr
					err := manager.Spawn(context.Background(), root, SpawnOptions{
						Timeout: 4 * time.Second, Name: "worker", Harness: "codex",
					})
					if err == nil {
						t.Fatal("Spawn() error = nil")
					}
					if len(client.startAgentCalls) != 1 {
						t.Fatalf("StartAgent() calls = %#v, want exactly one", client.startAgentCalls)
					}
					if !errors.Is(err, test.fatal) {
						t.Fatalf("Spawn() error = %v, want fatal cause %v discoverable", err, test.fatal)
					}
					if test.structuredCode != "" {
						var commandErr *herdr.CommandError
						if !errors.As(err, &commandErr) || !herdr.IsErrorCode(err, test.structuredCode) {
							t.Fatalf("Spawn() error = %T %v, want structured code %q", err, err, test.structuredCode)
						}
					}
				})
			}
		})
	}
}

func TestSpawnLocalReadinessDeadlineAndFinalFailureRetainBothCauses(t *testing.T) {
	manager, client, root := transientSpawnManager(t)
	readinessErr := realPaneReadinessDeadlineError(t)
	if !paneReadinessTimedOut(readinessErr) {
		t.Fatalf("paneReadinessTimedOut(%v) = false, want true for marked local deadline", readinessErr)
	}
	var exitErr *exec.ExitError
	if !errors.Is(readinessErr, context.DeadlineExceeded) || !errors.As(readinessErr, &exitErr) {
		t.Fatalf("local readiness error = %T %v, want deadline and raw process error", readinessErr, readinessErr)
	}
	finalErr := errors.New("final launch failed")
	client.startErrs = []error{herdrCommandError("agent_pane_busy"), finalErr}
	client.waitPaneErr = readinessErr

	err := manager.Spawn(context.Background(), root, SpawnOptions{
		Timeout: 4 * time.Second, Name: "worker", Harness: "codex",
	})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, finalErr) {
		t.Fatalf("Spawn() error = %v, want DeadlineExceeded and final launch failure in chain", err)
	}
	if len(client.startAgentCalls) != 2 || len(client.waitPaneCalls) != 1 || len(client.closeCalls) != 1 {
		t.Fatalf("calls = starts %#v, waits %#v, closes %#v", client.startAgentCalls, client.waitPaneCalls, client.closeCalls)
	}
}

func TestSpawnDoesNotRetryNonBusyStartFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
	}{
		{name: "generic", err: errors.New("launch failed")},
		{name: "name taken", err: herdrCommandError("agent_name_taken")},
		{name: "input failure", err: herdrCommandError("input_failed")},
		{name: "transport timeout", err: context.DeadlineExceeded},
		{name: "ambiguous plain text", err: errors.New("agent_pane_busy: pane is busy")},
		{
			name: "busy before conflicting structured failure",
			err:  errors.Join(herdrCommandError("agent_pane_busy"), herdrCommandError("input_failed")),
		},
		{
			name: "conflicting structured failure before busy",
			err:  errors.Join(herdrCommandError("input_failed"), herdrCommandError("agent_pane_busy")),
		},
		{
			name: "busy before conflicting generic failure",
			err:  errors.Join(herdrCommandError("agent_pane_busy"), errors.New("transport failed")),
		},
		{
			name: "conflicting generic failure before busy",
			err:  errors.Join(errors.New("transport failed"), herdrCommandError("agent_pane_busy")),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, client, root := transientSpawnManager(t)
			client.startErr = test.err

			if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: 30 * time.Second, Name: "worker", Harness: "codex"}); err == nil {
				t.Fatal("Spawn() error = nil")
			}
			if len(client.startAgentCalls) != 1 || len(client.waitPaneCalls) != 0 || len(client.closeCalls) != 1 {
				t.Fatalf("calls for non-busy failure: starts=%#v waits=%#v closes=%#v", client.startAgentCalls, client.waitPaneCalls, client.closeCalls)
			}
		})
	}
}

func TestRecoveredSpawnRegistrationFailureRollsBackWithoutLedgerArtifacts(t *testing.T) {
	t.Parallel()
	manager, client, root := transientSpawnManager(t)
	client.startErrs = []error{herdrCommandError("agent_pane_busy"), nil}

	err := manager.Spawn(context.Background(), root, SpawnOptions{
		Timeout: 30 * time.Second, Name: "worker", Harness: "codex", ParentTask: "missing-task", Task: "review",
	})
	if err == nil || !errors.Is(err, messaging.ErrTaskNotFound) {
		t.Fatalf("Spawn() error = %v, want missing parent task", err)
	}
	assertIdenticalAgentStarts(t, client.startAgentCalls)
	if len(client.closeCalls) != 1 || len(client.promptCalls) != 0 {
		t.Fatalf("registration failure side effects: closes=%v prompts=%v", client.closeCalls, client.promptCalls)
	}
	store := messaging.New(root, testSessionName)
	agents, _ := store.Agents()
	tasks, _ := store.Tasks()
	wakes, _ := store.PendingWakes()
	if len(agents) != 0 || len(tasks) != 0 || len(wakes) != 0 {
		t.Fatalf("ledger after registration failure: agents=%#v tasks=%#v wakes=%#v", agents, tasks, wakes)
	}
}

func TestRecoveredSpawnPromptFailurePreservesAuditSemantics(t *testing.T) {
	t.Parallel()
	manager, client, root := transientSpawnManager(t)
	client.startErrs = []error{herdrCommandError("agent_pane_busy"), nil}
	client.promptErr = errors.New("prompt failed")

	err := manager.Spawn(context.Background(), root, SpawnOptions{
		Timeout: 30 * time.Second, Name: "worker", Harness: "codex", Task: "review",
	})
	if err == nil || !strings.Contains(err.Error(), "initial prompt failed") {
		t.Fatalf("Spawn() error = %v", err)
	}
	assertIdenticalAgentStarts(t, client.startAgentCalls)
	if len(client.promptCalls) != 1 || len(client.closeCalls) != 1 {
		t.Fatalf("prompt failure calls: prompts=%v closes=%v", client.promptCalls, client.closeCalls)
	}
	store := messaging.New(root, testSessionName)
	agents, agentErr := store.Agents()
	tasks, taskErr := store.Tasks()
	if agentErr != nil || taskErr != nil {
		t.Fatalf("read prompt-failure audit: agents=%v tasks=%v", agentErr, taskErr)
	}
	if len(agents) != 1 || agents[0].Active || agents[0].Status != "stopped" {
		t.Fatalf("agents = %#v, want one audited stopped worker", agents)
	}
	if len(tasks) != 1 || tasks[0].Status != messaging.TaskOrphaned {
		t.Fatalf("tasks = %#v, want one audited orphaned task", tasks)
	}
}

func TestInitialOrchestratorUsesTransientPaneReadinessPolicy(t *testing.T) {
	t.Parallel()
	t.Run("recovers and registers once", func(t *testing.T) {
		root := t.TempDir()
		initTestProject(t, root)
		client := &fakeHerdr{snapshot: testSnapshot(), startErrs: []error{herdrCommandError("agent_pane_busy"), nil}}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness

		if err := manager.Start(context.Background(), root, StartOptions{Timeout: 30 * time.Second, Harness: "codex", HarnessSet: true}); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		assertIdenticalAgentStarts(t, client.startAgentCalls)
		if len(client.waitPaneCalls) != 1 || countLifecycleCall(client.calls, "start-server") != 1 || countLifecycleCall(client.calls, "split-pane") != 1 {
			t.Fatalf("orchestrator launch calls = %v, waits=%v", client.calls, client.waitPaneCalls)
		}
		record, found, err := readRecord(root)
		if err != nil || !found {
			t.Fatalf("readRecord() = %#v, %v, %v", record, found, err)
		}
		agents, err := messaging.New(root, record.SessionName).Agents()
		if err != nil || len(agents) != 1 || agents[0].Name != orchestratorIdentity || !agents[0].Active {
			t.Fatalf("orchestrator agents = %#v, %v", agents, err)
		}
	})

	t.Run("final failure rolls session back once", func(t *testing.T) {
		root := t.TempDir()
		initTestProject(t, root)
		client := &fakeHerdr{snapshot: testSnapshot(), startErrs: []error{
			herdrCommandError("agent_pane_busy"), herdrCommandError("agent_pane_busy"),
		}}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness

		err := manager.Start(context.Background(), root, StartOptions{Timeout: 30 * time.Second, Harness: "codex", HarnessSet: true})
		if err == nil || !strings.Contains(err.Error(), "remained unavailable after one readiness retry") {
			t.Fatalf("Start() error = %v", err)
		}
		assertIdenticalAgentStarts(t, client.startAgentCalls)
		if len(client.stopCalls) != 1 || len(client.deleteCalls) != 1 || len(client.waitPaneCalls) != 1 {
			t.Fatalf("orchestrator rollback calls: stop=%v delete=%v wait=%v", client.stopCalls, client.deleteCalls, client.waitPaneCalls)
		}
	})
}
