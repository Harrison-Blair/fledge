package doctor_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agent"
	"github.com/Harrison-Blair/fledge/internal/doctor"
	"github.com/Harrison-Blair/fledge/internal/herdr"
)

type fakeProber struct {
	pong    herdr.PongResult
	pingErr error
	list    herdr.IntegrationListResult
	listErr error
}

func (f fakeProber) Ping(context.Context) (herdr.PongResult, error) { return f.pong, f.pingErr }
func (f fakeProber) IntegrationList(context.Context) (herdr.IntegrationListResult, error) {
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
	for _, kind := range append(agent.ModelHarnesses(), "grok") {
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

// discovery points at an empty home; its Run errors so any accidental command
// execution by doctor is caught. Doctor must only read local cache files.
func discovery() agent.Discovery {
	return agent.Discovery{Home: "/nonexistent-home-fledge", Run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("doctor must not execute harness commands")
	}}
}

// writeCache writes a model cache fixture file.
func writeCache(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// homeWithPi returns a home whose pi model store holds the given JSON.
func homeWithPi(t *testing.T, store string) string {
	t.Helper()
	home := t.TempDir()
	writeCache(t, filepath.Join(home, ".pi", "agent", "models-store.json"), store)
	return home
}

// fatalRunDiscovery fails the test if doctor ever executes a command.
func fatalRunDiscovery(t *testing.T, home string) agent.Discovery {
	return agent.Discovery{Home: home, Run: func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("doctor must not execute harness commands")
		return nil, nil
	}}
}

func env(vars map[string]string, mode fs.FileMode, statErr error) doctor.Environment {
	return doctor.Environment{
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

func healthyEnv() doctor.Environment {
	return env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock", "HERDR_PANE_ID": "w1:p2", "HERDR_SESSION": "main"}, fs.ModeSocket|0o600, nil)
}

func find(t *testing.T, r doctor.Report, name string) doctor.Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q missing from %+v", name, r.Checks)
	return doctor.Check{}
}

func diagnose(o doctor.Options) doctor.Report {
	return doctor.Diagnose(context.Background(), o)
}

func TestDiagnoseHealthy(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()})
	if len(r.Checks) != 5 {
		t.Fatalf("want 5 checks, got %d", len(r.Checks))
	}
	if find(t, r, "herdr_connectivity").Status != doctor.OK {
		t.Fatalf("connectivity %+v", find(t, r, "herdr_connectivity"))
	}
	if find(t, r, "herdr_compatibility").Status != doctor.OK {
		t.Fatalf("compatibility %+v", find(t, r, "herdr_compatibility"))
	}
	if find(t, r, "harness_installations").Status != doctor.OK {
		t.Fatalf("harness %+v", find(t, r, "harness_installations"))
	}
	if find(t, r, "configuration").Status != doctor.OK {
		t.Fatalf("configuration %+v", find(t, r, "configuration"))
	}
	if r.Summary.Fail != 0 {
		t.Fatalf("unexpected failures: %+v", r)
	}
}

func TestConnectivityFailAlsoWarnsCompatibility(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pingErr: errors.New("connection refused"), listErr: errors.New("connection refused")}, Discovery: discovery(), Env: healthyEnv()})
	conn := find(t, r, "herdr_connectivity")
	if conn.Status != doctor.Fail || !strings.Contains(conn.Detail, "connection refused") {
		t.Fatalf("connectivity %+v", conn)
	}
	compat := find(t, r, "herdr_compatibility")
	if compat.Status != doctor.Warn {
		t.Fatalf("compatibility %+v", compat)
	}
	if r.Summary.Fail == 0 {
		t.Fatalf("expected a failure: %+v", r)
	}
}

func TestCompatibilityMismatchIsWarnNotFail(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(21), list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()})
	if find(t, r, "herdr_connectivity").Status != doctor.OK {
		t.Fatal("connectivity should be ok")
	}
	compat := find(t, r, "herdr_compatibility")
	if compat.Status != doctor.Warn || !strings.Contains(compat.Detail, "22") {
		t.Fatalf("compatibility %+v", compat)
	}
}

func TestHarnessListFailure(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), listErr: errors.New("boom")}, Discovery: discovery(), Env: healthyEnv()})
	h := find(t, r, "harness_installations")
	if h.Status != doctor.Fail || !strings.Contains(h.Detail, "boom") {
		t.Fatalf("harness %+v", h)
	}
}

func TestModelAvailableDiscoveryErrorFails(t *testing.T) {
	// pi is available but its store is malformed -> fail, without running commands.
	home := homeWithPi(t, `{"broken":`)
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("pi")}, Discovery: fatalRunDiscovery(t, home), Env: healthyEnv()})
	m := find(t, r, "model_discovery")
	if m.Status != doctor.Fail {
		t.Fatalf("model %+v", m)
	}
	if !strings.Contains(m.Detail, "pi:fail") {
		t.Fatalf("detail should name pi: %+v", m)
	}
}

func TestModelAvailableWithModelsIsOK(t *testing.T) {
	// pi is available and returns models; the other read-only kinds are unavailable.
	home := homeWithPi(t, `{"prov":{"models":[{"id":"m1","name":"M1"}]}}`)
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("pi")}, Discovery: fatalRunDiscovery(t, home), Env: healthyEnv()})
	m := find(t, r, "model_discovery")
	// pi ok, codex/claude warn (unavailable) -> overall warn, never fail.
	if m.Status != doctor.Warn || !strings.Contains(m.Detail, "pi:ok(1)") {
		t.Fatalf("model %+v", m)
	}
}

func TestModelDiscoveryIsReadOnlyAndExcludesCommandKinds(t *testing.T) {
	// Even with every kind marked available, doctor covers only the read-only
	// file sources and never invokes the command runner.
	disc := fatalRunDiscovery(t, homeWithPi(t, `{"prov":{"models":[{"id":"m1"}]}}`))
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("pi", "codex", "claude", "opencode", "cursor")}, Discovery: disc, Env: healthyEnv()})
	m := find(t, r, "model_discovery")
	var kinds []string
	for _, h := range m.Data.(doctor.ModelData).Harnesses {
		kinds = append(kinds, h.Harness)
	}
	if !reflect.DeepEqual(kinds, []string{"pi", "codex", "claude"}) {
		t.Fatalf("model kinds = %q, want pi/codex/claude only", kinds)
	}
}

func TestModelUnavailableIsWarnNotFail(t *testing.T) {
	// Nothing available: every model kind is warn, none fail.
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList()}, Discovery: discovery(), Env: healthyEnv()})
	m := find(t, r, "model_discovery")
	if m.Status != doctor.Warn {
		t.Fatalf("model %+v", m)
	}
}

func TestModelAvailabilityUnknownWhenListFails(t *testing.T) {
	// integration.list failed: availability is unknown, discovery still attempted.
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), listErr: errors.New("down")}, Discovery: discovery(), Env: healthyEnv()})
	m := find(t, r, "model_discovery")
	if !strings.Contains(m.Detail, "pi") {
		t.Fatalf("model %+v", m)
	}
	// With availability unknown, a discovery error is a warning, never a failure.
	if m.Status == doctor.Fail {
		t.Fatalf("unknown availability must not fail: %+v", m)
	}
}

func TestConfigurationSocketModeWarn(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("opencode")}, Discovery: discovery(),
		Env: env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock", "HERDR_PANE_ID": "w1:p2"}, fs.ModeSocket|0o644, nil)})
	c := find(t, r, "configuration")
	if c.Status != doctor.Warn || !strings.Contains(c.Detail, "644") {
		t.Fatalf("configuration %+v", c)
	}
}

func TestConfigurationMissingSocketFails(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pingErr: errors.New("x"), listErr: errors.New("x")}, Discovery: discovery(),
		Env: env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/gone.sock", "HERDR_PANE_ID": "w1:p2"}, 0, fs.ErrNotExist)})
	c := find(t, r, "configuration")
	if c.Status != doctor.Fail || !strings.Contains(c.Detail, "missing") {
		t.Fatalf("configuration %+v", c)
	}
}

func TestConfigurationStatErrorWarns(t *testing.T) {
	// A non-not-exist stat error leaves the mode unknown: warn, not fail.
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("pi")}, Discovery: discovery(),
		Env: env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock", "HERDR_PANE_ID": "w1:p2"}, 0, errors.New("permission denied"))})
	c := find(t, r, "configuration")
	if c.Status != doctor.Warn || !strings.Contains(c.Detail, "permission denied") {
		t.Fatalf("configuration %+v", c)
	}
}

func TestConfigurationNotASocketFails(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pingErr: errors.New("x"), listErr: errors.New("x")}, Discovery: discovery(),
		Env: env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/etc/hosts", "HERDR_PANE_ID": "w1:p2"}, 0o600, nil)})
	c := find(t, r, "configuration")
	if c.Status != doctor.Fail || !strings.Contains(c.Detail, "socket") {
		t.Fatalf("configuration %+v", c)
	}
}

func TestConfigurationOutsideHerdrFails(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pingErr: errors.New("x"), listErr: errors.New("x")}, Discovery: discovery(),
		Env: env(map[string]string{}, 0, errors.New("unset"))})
	if find(t, r, "configuration").Status != doctor.Fail {
		t.Fatal("configuration should fail outside Herdr")
	}
	if find(t, r, "herdr_connectivity").Status != doctor.Fail {
		t.Fatal("connectivity should fail outside Herdr")
	}
	if r.Summary.Fail == 0 {
		t.Fatal("expected failures outside Herdr")
	}
}

func TestConfigurationPaneAbsentIsWarn(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("opencode")}, Discovery: discovery(),
		Env: env(map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "/run/herdr.sock"}, fs.ModeSocket|0o600, nil)})
	c := find(t, r, "configuration")
	if c.Status != doctor.Warn {
		t.Fatalf("configuration %+v", c)
	}
	if !strings.Contains(c.Detail, "cwd=/work/dir") {
		t.Fatalf("detail should include cwd: %+v", c)
	}
}

// humanReport renders the healthy scenario's human report for structure checks.
func humanReport(t *testing.T, verbose bool) string {
	t.Helper()
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()})
	var b strings.Builder
	if err := r.Write(&b, false, verbose); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestWriteHumanGroupedBlocks(t *testing.T) {
	out := humanReport(t, false)
	if !strings.HasPrefix(out, "fledge doctor\n\n") {
		t.Fatalf("missing header:\n%s", out)
	}
	// The check name column is padded to the longest of the five names plus a
	// fixed gap, so every status starts in the same column.
	wantLines := []string{
		"  herdr_connectivity      ok",
		"    socket responded to ping",
		"  herdr_compatibility     ok",
		"    version 0.9.1 · protocol 22 (expected 22)",
		"  harness_installations   ok",
		"    6 targets, 1 available",
		"  model_discovery         warn",
		"  configuration           ok",
		// The configuration block always shows an aligned labeled key column.
		"    HERDR_ENV      1",
		"    socket         /run/herdr.sock (0600)",
		"    pane           w1:p2",
		"    session        main",
		"    cwd            /work/dir",
		"  4 ok · 1 warn · 0 fail",
	}
	for _, want := range wantLines {
		if !strings.Contains(out, want+"\n") {
			t.Fatalf("missing line %q in:\n%s", want, out)
		}
	}
	// No leftover table header from the old renderer.
	for _, gone := range []string{"CHECK\tSTATUS", "Summary:"} {
		if strings.Contains(out, gone) {
			t.Fatalf("unexpected old-style output %q in:\n%s", gone, out)
		}
	}
	// One blank line separates each check block and precedes the summary.
	if !strings.Contains(out, "    socket responded to ping\n\n  herdr_compatibility") {
		t.Fatalf("missing blank line between blocks:\n%s", out)
	}
}

func TestWriteHumanHidesLongDetailByDefault(t *testing.T) {
	out := humanReport(t, false)
	// The capabilities list and the available-name list are verbose-only.
	for _, hidden := range []string{"capabilities:", "available:", "live_handoff", "opencode\n"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("default output leaked verbose detail %q:\n%s", hidden, out)
		}
	}
}

func TestWriteHumanVerboseRevealsLongDetail(t *testing.T) {
	out := humanReport(t, true)
	for _, want := range []string{
		"    version 0.9.1 · protocol 22 (expected 22)",
		"    capabilities: live_handoff, health_check",
		"    6 targets, 1 available",
		"    available: opencode",
	} {
		if !strings.Contains(out, want+"\n") {
			t.Fatalf("verbose output missing %q in:\n%s", want, out)
		}
	}
}

func TestWriteHumanWarnAndFailAlwaysShowDetail(t *testing.T) {
	// Connectivity fails (no ping) and compatibility warns on a mismatch; both
	// diagnostics must appear with and without --verbose.
	for _, verbose := range []bool{false, true} {
		r := diagnose(doctor.Options{Herdr: fakeProber{pingErr: errors.New("dial unix /tmp/missing.sock: no such file"), listErr: errors.New("down")}, Discovery: discovery(), Env: healthyEnv()})
		var b strings.Builder
		if err := r.Write(&b, false, verbose); err != nil {
			t.Fatal(err)
		}
		out := b.String()
		if !strings.Contains(out, "  herdr_connectivity      fail\n") {
			t.Fatalf("verbose=%v: missing fail status line:\n%s", verbose, out)
		}
		if !strings.Contains(out, "    ping failed: dial unix /tmp/missing.sock: no such file\n") {
			t.Fatalf("verbose=%v: missing connectivity diagnostic:\n%s", verbose, out)
		}
	}

	// A protocol mismatch is a warn whose diagnostic detail is always shown.
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(21), list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()})
	var b strings.Builder
	if err := r.Write(&b, false, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "  herdr_compatibility     warn\n") || !strings.Contains(b.String(), "!= pinned 22") {
		t.Fatalf("mismatch warn detail missing:\n%s", b.String())
	}
}

// u32 returns a pointer to a uint32 for optional capability fields.
func u32(v uint32) *uint32 { return &v }

func TestConnectivityDetailWording(t *testing.T) {
	// The stored JSON detail keeps the original wording; only the human line is
	// shortened. The two are separated so --json content is unchanged.
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()})
	conn := find(t, r, "herdr_connectivity")
	if conn.Detail != "Herdr socket responded to ping" {
		t.Fatalf("connectivity JSON detail = %q, want %q", conn.Detail, "Herdr socket responded to ping")
	}

	// JSON output carries the stored (unshortened) detail.
	var j strings.Builder
	if err := r.Write(&j, true, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j.String(), `"detail":"Herdr socket responded to ping"`) {
		t.Fatalf("JSON detail wording changed:\n%s", j.String())
	}

	// Human output shows the shortened phrasing and never the JSON wording.
	var b strings.Builder
	if err := r.Write(&b, false, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "    socket responded to ping\n") || strings.Contains(b.String(), "Herdr socket responded") {
		t.Fatalf("human wording wrong:\n%s", b.String())
	}
}

func TestWriteHumanShortensHomePathsButJSONKeepsAbsolute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sock := filepath.Join(home, ".config", "herdr", "herdr.sock")
	proj := filepath.Join(home, "source", "fledge")
	e := doctor.Environment{
		Getenv: func(k string) string {
			return map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": sock, "HERDR_PANE_ID": "w1:p2", "HERDR_SESSION": "main"}[k]
		},
		Stat:  func(string) (fs.FileInfo, error) { return fakeInfo{mode: fs.ModeSocket | 0o600}, nil },
		Getwd: func() (string, error) { return proj, nil },
	}
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("opencode")}, Discovery: discovery(), Env: e})

	var human strings.Builder
	if err := r.Write(&human, false, false); err != nil {
		t.Fatal(err)
	}
	h := human.String()
	if !strings.Contains(h, "~/.config/herdr/herdr.sock (0600)") {
		t.Fatalf("socket path not shortened to ~:\n%s", h)
	}
	if !strings.Contains(h, "~/source/fledge") {
		t.Fatalf("cwd not shortened to ~:\n%s", h)
	}
	if strings.Contains(h, home) {
		t.Fatalf("human output leaked absolute home %q:\n%s", home, h)
	}

	var jsonOut strings.Builder
	if err := r.Write(&jsonOut, true, false); err != nil {
		t.Fatal(err)
	}
	j := jsonOut.String()
	if !strings.Contains(j, sock) || !strings.Contains(j, proj) {
		t.Fatalf("json paths must stay absolute:\n%s", j)
	}
	if strings.Contains(j, "~/.config") || strings.Contains(j, "~/source") {
		t.Fatalf("json paths must not be shortened:\n%s", j)
	}
}

func TestWriteHumanVerboseWrapsListsWithinWidth(t *testing.T) {
	full := &herdr.Capabilities{LiveHandoff: true, DetachedServerDaemon: true, HealthCheck: true, SurfaceInterest: true, EndpointProtocolGeneration: u32(1)}
	p := herdr.PongResult{Type: "pong", Version: "0.9.1", Protocol: 22, Capabilities: full}
	// opencode is the only available target, so the model harnesses report short
	// "not installed" lines and the only wrapping candidate is the long
	// capabilities list. (Unbreakable diagnostics such as long paths are
	// deliberately excluded: a token is never split to fit 80 columns.)
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: p, list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()})
	var b strings.Builder
	if err := r.Write(&b, false, true); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 80 {
			t.Fatalf("rendered line exceeds 80 columns (%d): %q", len(line), line)
		}
	}
	// Every capability token must survive intact (no mid-token break).
	for _, tok := range []string{"live_handoff", "detached_server_daemon", "health_check", "surface_interest", "endpoint_protocol_generation=1"} {
		if !strings.Contains(out, tok) {
			t.Fatalf("token %q split or missing:\n%s", tok, out)
		}
	}
	// The long capabilities list must actually wrap onto an 8-space continuation.
	if !strings.Contains(out, "\n        ") {
		t.Fatalf("expected a wrapped continuation line:\n%s", out)
	}
}

func TestWriteHumanVerboseCapabilitiesShownOnCompatWarn(t *testing.T) {
	// Protocol mismatch → warn diagnostic AND, under --verbose, the capabilities.
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(21), list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()})
	var b strings.Builder
	if err := r.Write(&b, false, true); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "  herdr_compatibility     warn\n") {
		t.Fatalf("expected compatibility warn:\n%s", out)
	}
	if !strings.Contains(out, "!= pinned 22") {
		t.Fatalf("warn diagnostic missing:\n%s", out)
	}
	if !strings.Contains(out, "capabilities: live_handoff, health_check") {
		t.Fatalf("capabilities line missing on warn:\n%s", out)
	}
	// The mismatch diagnostic itself must wrap to stay within 80 columns.
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 80 {
			t.Fatalf("compatibility verbose line exceeds 80 columns (%d): %q", len(line), line)
		}
	}
	// The pinned-protocol value must remain a whole token (never split).
	if !strings.Contains(out, "pinned 22") {
		t.Fatalf("mismatch token was split:\n%s", out)
	}
}

func TestWriteHumanVerboseCompatWarnFullCapabilitiesWithinWidth(t *testing.T) {
	// Protocol mismatch with every capability: the warn diagnostic embeds the
	// capability list, which must break at the commas so no verbose line exceeds
	// 80 columns, while the mismatch info and the separate capabilities: line
	// both remain present.
	full := &herdr.Capabilities{LiveHandoff: true, DetachedServerDaemon: true, HealthCheck: true, SurfaceInterest: true, EndpointProtocolGeneration: u32(1)}
	p := herdr.PongResult{Type: "pong", Version: "0.9.1", Protocol: 21, Capabilities: full}
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: p, list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()})
	var b strings.Builder
	if err := r.Write(&b, false, true); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 80 {
			t.Fatalf("verbose line exceeds 80 columns (%d): %q", len(line), line)
		}
	}
	if !strings.Contains(out, "  herdr_compatibility     warn\n") || !strings.Contains(out, "!= pinned 22") {
		t.Fatalf("mismatch info missing:\n%s", out)
	}
	if !strings.Contains(out, "capabilities: live_handoff, detached_server_daemon, health_check,") {
		t.Fatalf("separate capabilities list missing:\n%s", out)
	}
	// No capability name may be split across a wrap.
	for _, tok := range []string{"detached_server_daemon", "surface_interest", "endpoint_protocol_generation=1"} {
		if !strings.Contains(out, tok) {
			t.Fatalf("token %q was split:\n%s", tok, out)
		}
	}
	// The JSON detail stays comma-joined (byte-identical), unaffected by the
	// spaced human rendering.
	var j strings.Builder
	if err := r.Write(&j, true, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j.String(), `capabilities=live_handoff,detached_server_daemon,health_check,surface_interest,endpoint_protocol_generation=1`) {
		t.Fatalf("JSON detail must remain comma-joined:\n%s", j.String())
	}
}

func TestWriteHumanWrapsModelDiagnosticWithinWidth(t *testing.T) {
	// pi/codex/claude are available but their caches are absent, so each reports
	// a discovery-error diagnostic. Word-wrapping keeps every line within 80
	// columns because the only long token (the cache path) is itself under 80.
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("pi", "codex", "claude")}, Discovery: discovery(), Env: healthyEnv()})
	var b strings.Builder
	if err := r.Write(&b, false, false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "discovery error") {
		t.Fatalf("expected a discovery-error diagnostic:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 80 {
			t.Fatalf("model diagnostic line exceeds 80 columns (%d): %q", len(line), line)
		}
	}
	// The cache path token must survive intact (never split across lines).
	if !strings.Contains(out, "models-store.json") {
		t.Fatalf("path token was split:\n%s", out)
	}
}

func TestWriteJSONStableShape(t *testing.T) {
	r := diagnose(doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()})
	var b strings.Builder
	if err := r.Write(&b, true, false); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Checks []struct {
			Name, Status, Detail string
		} `json:"checks"`
		Summary struct{ OK, Warn, Fail int } `json:"summary"`
	}
	if err := json.Unmarshal([]byte(b.String()), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, b.String())
	}
	names := map[string]bool{}
	for _, c := range doc.Checks {
		names[c.Name] = true
		if c.Status == "" {
			t.Fatalf("empty status for %q", c.Name)
		}
	}
	for _, want := range []string{"herdr_connectivity", "herdr_compatibility", "harness_installations", "model_discovery", "configuration"} {
		if !names[want] {
			t.Fatalf("missing check %q", want)
		}
	}
	if doc.Summary.OK+doc.Summary.Warn+doc.Summary.Fail != 5 {
		t.Fatalf("summary does not sum to 5: %+v", doc.Summary)
	}

	// --verbose must not alter the JSON document at all.
	var vb strings.Builder
	if err := r.Write(&vb, true, true); err != nil {
		t.Fatal(err)
	}
	if vb.String() != b.String() {
		t.Fatalf("verbose changed JSON output:\n%s\nvs\n%s", vb.String(), b.String())
	}
}

func TestRunExitCodes(t *testing.T) {
	healthy := doctor.Options{Herdr: fakeProber{pong: pong(22), list: integrationList("opencode")}, Discovery: discovery(), Env: healthyEnv()}
	var b1 strings.Builder
	healthy.Out = &b1
	if err := doctor.Run(context.Background(), healthy); err != nil {
		t.Fatalf("healthy run returned error: %v", err)
	}

	failing := doctor.Options{Herdr: fakeProber{pingErr: errors.New("x"), listErr: errors.New("x")}, Discovery: discovery(), Env: env(map[string]string{}, 0, errors.New("unset"))}
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
	// The report must be printed exactly once; the error carries no second copy.
	if !strings.Contains(b2.String(), "herdr_connectivity") {
		t.Fatalf("report not written: %s", b2.String())
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
