package herdr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	processInfoJSON  = `{"type":"pane_process_info","process_info":{"pane_id":"w1:p1","shell_pid":4242,"tty":"/dev/pts/7","foreground_process_group_id":4242,"foreground_processes":[]}}`
	agentStartedJSON = `{"type":"agent_started","agent":` + agentJSON + `,"argv":["claude"]}`
)

var (
	processInfoArgv = []string{"pane", "process-info", "--pane", "w1:p1"}
	bareStartArgv   = []string{"agent", "start", "reviewer", "--kind", "claude", "--pane", "w1:p1"}
)

// gateHerdr installs a fake herdr that answers pane process-info and agent
// start, recording every invocation's argument vector byte for byte. The
// returned function reads the recorded invocations back in call order.
func gateHerdr(t *testing.T, processInfo, agentStart string) func() [][]string {
	t.Helper()
	calls := filepath.Join(t.TempDir(), "calls")
	if err := os.Mkdir(calls, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeHerdr(t, `
i=0
while ! mkdir "$HERDR_FAKE_CALLS/$i" 2>/dev/null; do i=$((i+1)); done
if [ "$#" -gt 0 ]; then printf '%s\0' "$@" >"$HERDR_FAKE_CALLS/$i/argv"; else : >"$HERDR_FAKE_CALLS/$i/argv"; fi
if [ "${1-}" = --session ]; then shift 2; fi
case "${1-} ${2-}" in
"pane process-info") `+processInfo+` ;;
"agent start") `+agentStart+` ;;
*) exit 2 ;;
esac
`)
	t.Setenv("HERDR_FAKE_CALLS", calls)

	return func() [][]string {
		t.Helper()
		var recorded [][]string
		for i := 0; ; i++ {
			data, err := os.ReadFile(filepath.Join(calls, strconv.Itoa(i), "argv"))
			if os.IsNotExist(err) {
				return recorded
			}
			if err != nil {
				t.Fatal(err)
			}
			var argv []string
			if len(data) > 0 {
				argv = strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
			}
			recorded = append(recorded, argv)
		}
	}
}

func printEnv(name string) string {
	return `printf '%s' "$` + name + `"`
}

// happyHerdr answers process-info and agent start with fixed successful
// results.
func happyHerdr(t *testing.T) func() [][]string {
	t.Helper()
	calls := gateHerdr(t, printEnv("HERDR_FAKE_PROCESS_INFO"), printEnv("HERDR_FAKE_AGENT_STARTED"))
	t.Setenv("HERDR_FAKE_PROCESS_INFO", envelope(processInfoJSON))
	t.Setenv("HERDR_FAKE_AGENT_STARTED", envelope(agentStartedJSON))
	return calls
}

// gatedClient returns a client whose observation seam replays samples in
// order, repeating the last one, with fast readiness timing.
func gatedClient(t *testing.T, samples ...func() (bool, error)) *Client {
	t.Helper()
	client := New(nil, nil, nil)
	client.readiness.interval = time.Millisecond
	client.readiness.timeout = 2 * time.Second
	var seen []ProcessInfo
	client.readiness.observe = func(info ProcessInfo) (bool, error) {
		seen = append(seen, info)
		sample := samples[min(len(seen), len(samples))-1]
		return sample()
	}
	t.Cleanup(func() {
		for _, info := range seen {
			if info.PaneID != "w1:p1" || info.ShellPID == nil || *info.ShellPID != 4242 {
				t.Errorf("observation received %#v", info)
			}
		}
	})
	return client
}

func ready() (bool, error)    { return true, nil }
func notReady() (bool, error) { return false, nil }

func TestStartAgentGatesEveryHarnessAndArgument(t *testing.T) {
	tests := []struct {
		name      string
		options   StartAgentOptions
		wantStart []string
	}{
		{
			name:      "pi with args",
			options:   StartAgentOptions{Name: "reviewer", Kind: "pi", PaneID: "w1:p1", Args: []string{"--model", "opus"}},
			wantStart: []string{"agent", "start", "reviewer", "--kind", "pi", "--pane", "w1:p1", "--", "--model", "opus"},
		},
		{
			name:      "claude with timeout",
			options:   StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1", Args: []string{"--model", "opus"}, TimeoutMS: 60000},
			wantStart: []string{"agent", "start", "reviewer", "--kind", "claude", "--pane", "w1:p1", "--timeout", "60000", "--", "--model", "opus"},
		},
		{
			name:      "codex nil args",
			options:   StartAgentOptions{Name: "reviewer", Kind: "codex", PaneID: "w1:p1"},
			wantStart: []string{"agent", "start", "reviewer", "--kind", "codex", "--pane", "w1:p1"},
		},
		{
			name:      "claude empty args",
			options:   StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1", Args: []string{}},
			wantStart: bareStartArgv,
		},
		{
			name:      "pi single tiny arg",
			options:   StartAgentOptions{Name: "reviewer", Kind: "pi", PaneID: "w1:p1", Args: []string{"x"}},
			wantStart: []string{"agent", "start", "reviewer", "--kind", "pi", "--pane", "w1:p1", "--", "x"},
		},
		{
			name:      "unknown future harness",
			options:   StartAgentOptions{Name: "reviewer", Kind: "future", PaneID: "w1:p1"},
			wantStart: []string{"agent", "start", "reviewer", "--kind", "future", "--pane", "w1:p1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := happyHerdr(t)
			got, err := gatedClient(t, ready).StartAgent(context.Background(), tc.options)
			if err != nil {
				t.Fatalf("StartAgent: %v", err)
			}
			if got != wantAgent {
				t.Errorf("agent = %#v, want %#v", got, wantAgent)
			}
			want := [][]string{processInfoArgv, processInfoArgv, tc.wantStart}
			if recorded := calls(); !reflect.DeepEqual(recorded, want) {
				t.Errorf("calls = %q, want %q", recorded, want)
			}
		})
	}
}

func TestStartAgentWithSessionKeepsReadinessSeam(t *testing.T) {
	calls := happyHerdr(t)
	client := gatedClient(t, ready).WithSession("managed")
	if _, err := client.StartAgent(context.Background(), StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1"}); err != nil {
		t.Fatalf("StartAgent: %v", err)
	}
	prefixed := func(argv []string) []string { return append([]string{"--session", "managed"}, argv...) }
	want := [][]string{prefixed(processInfoArgv), prefixed(processInfoArgv), prefixed(bareStartArgv)}
	if recorded := calls(); !reflect.DeepEqual(recorded, want) {
		t.Fatalf("calls = %q, want %q", recorded, want)
	}
}

func TestStartAgentRequiresConsecutiveReadySamples(t *testing.T) {
	failing := func() (bool, error) { return true, errors.New("terminal attributes: transient") }
	tests := []struct {
		name        string
		samples     []func() (bool, error)
		wantSamples int
	}{
		{name: "ready immediately", samples: []func() (bool, error){ready}, wantSamples: 2},
		{name: "not ready first", samples: []func() (bool, error){notReady, ready}, wantSamples: 3},
		{name: "flap resets", samples: []func() (bool, error){ready, notReady, ready, ready}, wantSamples: 4},
		{name: "observation error resets", samples: []func() (bool, error){ready, failing, ready, ready}, wantSamples: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := happyHerdr(t)
			if _, err := gatedClient(t, tc.samples...).StartAgent(context.Background(), StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1"}); err != nil {
				t.Fatalf("StartAgent: %v", err)
			}
			recorded := calls()
			if len(recorded) != tc.wantSamples+1 {
				t.Fatalf("calls = %q, want %d process-info calls then agent start", recorded, tc.wantSamples)
			}
			for _, argv := range recorded[:tc.wantSamples] {
				if !reflect.DeepEqual(argv, processInfoArgv) {
					t.Errorf("call before start = %q", argv)
				}
			}
			if !reflect.DeepEqual(recorded[tc.wantSamples], bareStartArgv) {
				t.Errorf("final call = %q, want agent start", recorded[tc.wantSamples])
			}
		})
	}
}

func TestStartAgentReadinessTimeout(t *testing.T) {
	calls := happyHerdr(t)
	client := gatedClient(t, notReady)
	client.readiness.timeout = 40 * time.Millisecond

	_, err := client.StartAgent(context.Background(), StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1"})
	var readiness *ReadinessError
	if !errors.As(err, &readiness) {
		t.Fatalf("StartAgent error = %v, want *ReadinessError", err)
	}
	if readiness.Reason != ReadinessTimeout || readiness.PaneID != "w1:p1" || readiness.Samples < 1 || readiness.Elapsed < 40*time.Millisecond {
		t.Fatalf("readiness error = %#v", readiness)
	}
	if ContextCause(err) != nil {
		t.Fatalf("ContextCause = %v, want nil for a readiness timeout", ContextCause(err))
	}
	assertNoAgentStart(t, calls())
}

func TestStartAgentHonoursShorterParentDeadline(t *testing.T) {
	calls := happyHerdr(t)
	client := gatedClient(t, notReady)
	client.readiness.timeout = readinessTimeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.StartAgent(ctx, StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1"})
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(ContextCause(err), context.DeadlineExceeded) {
		t.Fatalf("StartAgent error = %v (cause %v), want parent deadline", err, ContextCause(err))
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("StartAgent waited %s past the parent deadline", elapsed)
	}
	assertNoAgentStart(t, calls())
}

func TestStartAgentPreservesParentCancellation(t *testing.T) {
	calls := happyHerdr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := gatedClient(t, func() (bool, error) {
		cancel()
		return false, nil
	})

	_, err := client.StartAgent(ctx, StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1"})
	if !errors.Is(err, context.Canceled) || !errors.Is(ContextCause(err), context.Canceled) {
		t.Fatalf("StartAgent error = %v (cause %v), want context.Canceled", err, ContextCause(err))
	}
	if !strings.Contains(err.Error(), "pane w1:p1 readiness") {
		t.Fatalf("StartAgent error = %v", err)
	}
	assertNoAgentStart(t, calls())
}

func TestStartAgentPreservesStructuredProcessInfoError(t *testing.T) {
	calls := gateHerdr(t, `printf '%s' '{"error":{"code":"pane_not_found","message":"pane w1:p1 not found"},"id":"cli:pane:process_info"}' >&2; exit 1`, "exit 3")

	_, err := gatedClient(t, ready).WithSession("managed").StartAgent(context.Background(), StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1"})
	var reported *Error
	if !errors.As(err, &reported) {
		t.Fatalf("StartAgent error = %v, want *Error", err)
	}
	want := Error{Operation: "herdr --session managed pane process-info --pane w1:p1", Code: "pane_not_found", Message: "pane w1:p1 not found"}
	if *reported != want {
		t.Fatalf("error = %#v, want %#v", *reported, want)
	}
	assertNoAgentStart(t, calls())
}

func TestStartAgentRejectsProcessInfoForOtherPane(t *testing.T) {
	calls := gateHerdr(t, printEnv("HERDR_FAKE_PROCESS_INFO"), "exit 3")
	t.Setenv("HERDR_FAKE_PROCESS_INFO", envelope(strings.Replace(processInfoJSON, `"pane_id":"w1:p1"`, `"pane_id":"w1:p2"`, 1)))

	_, err := gatedClient(t, ready).StartAgent(context.Background(), StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1"})
	if err == nil || !strings.Contains(err.Error(), "result describes pane w1:p2, want w1:p1") {
		t.Fatalf("StartAgent error = %v", err)
	}
	assertNoAgentStart(t, calls())
}

func TestStartAgentFailsClosedWhenObservationUnsupported(t *testing.T) {
	calls := happyHerdr(t)
	client := gatedClient(t, func() (bool, error) { return false, errReadinessUnsupported })

	_, err := client.StartAgent(context.Background(), StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1"})
	var readiness *ReadinessError
	if !errors.As(err, &readiness) || readiness.Reason != ReadinessUnsupported || readiness.Samples != 1 || readiness.PaneID != "w1:p1" {
		t.Fatalf("StartAgent error = %v, want unsupported readiness error", err)
	}
	assertNoAgentStart(t, calls())
}

func TestStartAgentPassesArgvByteIdentical(t *testing.T) {
	huge := strings.Repeat("q", 10000)
	args := []string{"\xff\xfe", "a\x01b\nc", "", huge, "é​"}
	for i := 0; i < 100; i++ {
		args = append(args, "arg"+strconv.Itoa(i))
	}

	t.Run("success", func(t *testing.T) {
		calls := happyHerdr(t)
		if _, err := gatedClient(t, ready).StartAgent(context.Background(), StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1", Args: args}); err != nil {
			t.Fatalf("StartAgent: %v", err)
		}
		recorded := calls()
		want := append(append([]string{}, bareStartArgv...), "--")
		want = append(want, args...)
		if len(recorded) != 3 || !reflect.DeepEqual(recorded[2], want) {
			t.Fatalf("agent start argv differs from the requested argv")
		}
	})

	t.Run("failure diagnostics are bounded", func(t *testing.T) {
		gateHerdr(t, printEnv("HERDR_FAKE_PROCESS_INFO"), `printf '%s' "$HERDR_FAKE_STDERR" >&2; exit 1`)
		t.Setenv("HERDR_FAKE_PROCESS_INFO", envelope(processInfoJSON))
		t.Setenv("HERDR_FAKE_STDERR", strings.Repeat("e", 20000))
		_, err := gatedClient(t, ready).StartAgent(context.Background(), StartAgentOptions{Name: "reviewer", Kind: "claude", PaneID: "w1:p1", Args: args})
		if err == nil {
			t.Fatal("StartAgent succeeded")
		}
		text := err.Error()
		if len(text) > maxOperationBytes+maxOutputBytes+128 {
			t.Fatalf("error text is %d bytes", len(text))
		}
		if strings.Contains(text, huge[:16]) || strings.Contains(text, "arg99") {
			t.Fatalf("error text leaks argument content: %q", text)
		}
		if !strings.Contains(text, "…+") || !strings.HasPrefix(text, "herdr agent start reviewer --kind claude --pane w1:p1 -- ") {
			t.Fatalf("error text = %q", text)
		}
	})
}

func assertNoAgentStart(t *testing.T, calls [][]string) {
	t.Helper()
	if len(calls) == 0 {
		t.Fatal("no herdr calls recorded")
	}
	for _, argv := range calls {
		if len(argv) > 0 && argv[0] == "--session" {
			argv = argv[2:]
		}
		if len(argv) < 2 || argv[0] != "pane" || argv[1] != "process-info" {
			t.Fatalf("unexpected call %q", argv)
		}
	}
}

func TestReadinessErrorText(t *testing.T) {
	err := &ReadinessError{PaneID: "w1 p1", Reason: ReadinessTimeout, Samples: 7, Elapsed: 5001 * time.Millisecond}
	want := `herdr agent start: pane "w1 p1" not ready: timeout after 7 samples in 5.001s`
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestObservePaneReadyPlatformSeam(t *testing.T) {
	ready, err := observePaneReady(ProcessInfo{PaneID: "w1:p1"})
	if ready {
		t.Fatal("observation without a shell pid reported ready")
	}
	if runtime.GOOS == "linux" {
		if err == nil || errors.Is(err, errReadinessUnsupported) {
			t.Fatalf("linux observation error = %v", err)
		}
		return
	}
	if !errors.Is(err, errReadinessUnsupported) {
		t.Fatalf("observation error = %v, want unsupported", err)
	}
}
