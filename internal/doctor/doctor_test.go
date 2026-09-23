package doctor_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/doctor"
	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/models"
)

// fakeProber answers the two Herdr calls and records each call in order.
type fakeProber struct {
	pong    herdr.PongResult
	pingErr error
	list    herdr.IntegrationListResult
	listErr error
	calls   *[]string
}

func (f fakeProber) record(method string) {
	if f.calls != nil {
		*f.calls = append(*f.calls, method)
	}
}

func (f fakeProber) Ping(context.Context) (herdr.PongResult, error) {
	f.record("ping")
	return f.pong, f.pingErr
}

func (f fakeProber) IntegrationList(context.Context) (herdr.IntegrationListResult, error) {
	f.record("integration.list")
	return f.list, f.listErr
}

type fakeInfo struct{ mode fs.FileMode }

func (fakeInfo) Name() string        { return "s" }
func (fakeInfo) Size() int64         { return 0 }
func (f fakeInfo) Mode() fs.FileMode { return f.mode }
func (fakeInfo) ModTime() time.Time  { return time.Time{} }
func (fakeInfo) IsDir() bool         { return false }
func (fakeInfo) Sys() any            { return nil }

// integrationList builds a result whose listed targets are marked available.
func integrationList(available ...string) herdr.IntegrationListResult {
	set := map[string]bool{}
	for _, t := range available {
		set[t] = true
	}
	var infos []herdr.IntegrationInfo
	for _, kind := range append(models.ModelHarnesses(), "grok") {
		command := kind
		if kind == "cursor" {
			command = "cursor-agent"
		}
		state := "not_installed"
		if set[kind] {
			state = "current"
		}
		infos = append(infos, herdr.IntegrationInfo{Target: kind, Label: kind, Command: command, Available: set[kind], State: state})
	}
	return herdr.IntegrationListResult{Type: "integration_list", Integrations: infos}
}

func pong(protocol uint32) herdr.PongResult {
	return herdr.PongResult{Type: "pong", Version: "0.9.1", Protocol: protocol, Capabilities: &herdr.Capabilities{LiveHandoff: true, HealthCheck: true}}
}

// u32 returns a pointer to a uint32 for optional capability fields.
func u32(v uint32) *uint32 { return &v }

// discovery points at an empty home; its Run errors so any accidental command
// execution by doctor is caught. Doctor must only read local cache files.
func discovery() models.Discovery {
	return models.Discovery{Home: "/nonexistent-home-fledge", Run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("doctor must not execute harness commands")
	}}
}

// brokenPiDiscovery reads a fixture home whose pi model store is corrupt and
// whose other model caches were never created.
func brokenPiDiscovery() models.Discovery {
	d := discovery()
	d.Home = filepath.Join("testdata", "broken-model-cache")
	return d
}

func env(vars map[string]string, mode fs.FileMode, statErr error) checks.Environment {
	return checks.Environment{
		Getenv: func(k string) string { return vars[k] },
		Stat: func(string) (fs.FileInfo, error) {
			if statErr != nil {
				return nil, statErr
			}
			return fakeInfo{mode: mode}, nil
		},
		Getwd: func() (string, error) { return "/work/dir", nil },
	}
}

func healthyEnv() checks.Environment {
	return env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock", "HERDR_PANE_ID": "w1:p2", "HERDR_SESSION": "main"}, fs.ModeSocket|0o600, nil)
}

func healthy() doctor.Options {
	return doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()}
}

func outsideHerdr() doctor.Options {
	return doctor.Options{Herdr: fakeProber{pingErr: errors.New("dial unix /tmp/missing.sock: no such file"), listErr: errors.New("down")}, Discovery: discovery(), Env: env(map[string]string{}, 0, errors.New("unset"))}
}

func statuses(r report.Report) map[string]report.Status {
	got := map[string]report.Status{}
	for _, c := range r.Checks {
		got[c.Name] = c.Status
	}
	return got
}

func TestDiagnoseCallsHerdrOnceInOrder(t *testing.T) {
	for name, p := range map[string]fakeProber{
		"healthy":     {pong: pong(22), list: integrationList("pi", "codex", "claude")},
		"unreachable": {pingErr: errors.New("x"), listErr: errors.New("x")},
	} {
		t.Run(name, func(t *testing.T) {
			var calls []string
			p.calls = &calls
			doctor.Diagnose(context.Background(), doctor.Options{Herdr: p, Discovery: discovery(), Env: healthyEnv()})
			if !reflect.DeepEqual(calls, []string{"ping", "integration.list"}) {
				t.Fatalf("calls = %q, want one ping then one integration.list", calls)
			}
		})
	}
}

func TestDiagnoseFixedCheckOrder(t *testing.T) {
	for name, o := range map[string]doctor.Options{"healthy": healthy(), "outside": outsideHerdr()} {
		t.Run(name, func(t *testing.T) {
			r := doctor.Diagnose(context.Background(), o)
			var names []string
			for _, c := range r.Checks {
				names = append(names, c.Name)
			}
			want := []string{"herdr_connectivity", "herdr_compatibility", "harness_installations", "model_discovery", "configuration"}
			if !reflect.DeepEqual(names, want) {
				t.Fatalf("checks = %q, want %q", names, want)
			}
		})
	}
}

func TestDiagnoseHealthy(t *testing.T) {
	r := doctor.Diagnose(context.Background(), healthy())
	want := map[string]report.Status{"herdr_connectivity": report.OK, "herdr_compatibility": report.OK, "harness_installations": report.OK, "model_discovery": report.Warn, "configuration": report.OK}
	if got := statuses(r); !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if r.Summary != (report.Summary{OK: 4, Warn: 1}) {
		t.Fatalf("summary = %+v", r.Summary)
	}
}

func TestDiagnoseFeedsHerdrResultsToDependentChecks(t *testing.T) {
	// A failed ping fails connectivity and leaves compatibility unknown; a failed
	// list fails harnesses and leaves model availability unknown (never a failure).
	r := doctor.Diagnose(context.Background(), outsideHerdr())
	want := map[string]report.Status{"herdr_connectivity": report.Fail, "herdr_compatibility": report.Warn, "harness_installations": report.Fail, "model_discovery": report.Warn, "configuration": report.Fail}
	if got := statuses(r); !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if r.Summary != (report.Summary{Warn: 2, Fail: 3}) {
		t.Fatalf("summary = %+v", r.Summary)
	}
	if !strings.Contains(r.Checks[0].Detail, "missing.sock") || !strings.Contains(r.Checks[2].Detail, "down") {
		t.Fatalf("errors not threaded into checks: %+v", r.Checks)
	}
}

// TestRunGolden pins the full rendered output, byte for byte, against fixtures
// captured before doctor was split into packages. The scenarios cover
// every check verdict, human and verbose rendering, home shortening, and the
// JSON document.
func TestRunGolden(t *testing.T) {
	t.Setenv("HOME", "/home/golden")
	full := &herdr.Capabilities{LiveHandoff: true, DetachedServerDaemon: true, HealthCheck: true, SurfaceInterest: true, EndpointProtocolGeneration: u32(1)}
	homeEnv := checks.Environment{
		Getenv: func(k string) string {
			return map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/home/golden/.config/herdr/herdr.sock", "HERDR_PANE_ID": "w1:p2"}[k]
		},
		Stat:  func(string) (fs.FileInfo, error) { return fakeInfo{mode: fs.ModeSocket | 0o644}, nil },
		Getwd: func() (string, error) { return "/home/golden", nil },
	}
	scenarios := map[string]doctor.Options{
		"healthy":  healthy(),
		"outside":  outsideHerdr(),
		"mismatch": {Herdr: fakeProber{pong: herdr.PongResult{Type: "pong", Version: "0.9.1", Protocol: 21, Capabilities: full}, list: integrationList("pi", "codex", "claude")}, Discovery: brokenPiDiscovery(), Env: env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock"}, 0, errors.New("permission denied"))},
		"home":     {Herdr: fakeProber{pong: pong(22)}, Discovery: discovery(), Env: homeEnv},
	}
	for name, o := range scenarios {
		for _, mode := range []struct {
			suffix        string
			json, verbose bool
		}{{"human", false, false}, {"verbose", false, true}, {"json", true, false}} {
			t.Run(name+"."+mode.suffix, func(t *testing.T) {
				var b strings.Builder
				o.Out, o.JSON, o.Verbose = &b, mode.json, mode.verbose
				_ = doctor.Run(context.Background(), o)
				path := filepath.Join("testdata", name+"."+mode.suffix+".golden")
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if b.String() != string(want) {
					t.Fatalf("output differs from %s:\n--- got\n%s\n--- want\n%s", path, b.String(), want)
				}
			})
		}
	}
}

func TestRunExitCodes(t *testing.T) {
	ok := healthy()
	var b1 strings.Builder
	ok.Out = &b1
	if err := doctor.Run(context.Background(), ok); err != nil {
		t.Fatalf("healthy run returned error: %v", err)
	}

	failing := outsideHerdr()
	var b2 strings.Builder
	failing.Out = &b2
	err := doctor.Run(context.Background(), failing)
	if err == nil {
		t.Fatal("failing run should return an error")
	}
	var status interface{ ExitCode() int }
	if !errors.As(err, &status) || status.ExitCode() != 1 {
		t.Fatalf("want exit code 1, got %#v", err)
	}
	// The report was printed once already, so the root must not print the error.
	var failed *report.ReportError
	if !errors.As(err, &failed) || failed.Report.Summary.Fail != 3 || !cli.IsRendered(err) {
		t.Fatalf("want a rendered report error, got %#v", err)
	}
	if strings.Count(b2.String(), "herdr_connectivity") != 1 {
		t.Fatalf("report not written exactly once: %s", b2.String())
	}
}

func TestRunWarnDoesNotFail(t *testing.T) {
	// A pure-warn report (mismatched protocol, no pane) still exits 0.
	o := doctor.Options{Herdr: fakeProber{pong: pong(21), list: integrationList("opencode")}, Discovery: discovery(),
		Env: env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock"}, fs.ModeSocket|0o600, nil)}
	var b strings.Builder
	o.Out = &b
	if err := doctor.Run(context.Background(), o); err != nil {
		t.Fatalf("warn-only run must not fail: %v", err)
	}
}

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestRunOutputFailureIsRenderedWithoutSecondWrite(t *testing.T) {
	sentinel := errors.New("stdout closed")
	for name, o := range map[string]doctor.Options{"healthy": healthy(), "failing": outsideHerdr()} {
		for _, asJSON := range []bool{false, true} {
			o.Out, o.JSON = failingWriter{sentinel}, asJSON
			err := doctor.Run(context.Background(), o)
			var output *cli.OutputError
			if !errors.As(err, &output) || !errors.Is(err, sentinel) || !cli.IsRendered(err) {
				t.Fatalf("%s json=%v: want a rendered output error wrapping the cause, got %#v", name, asJSON, err)
			}
			var status interface{ ExitCode() int }
			if !errors.As(err, &status) || status.ExitCode() != 1 {
				t.Fatalf("%s json=%v: want exit code 1, got %#v", name, asJSON, err)
			}
		}
	}
}
