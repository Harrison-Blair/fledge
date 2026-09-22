package report_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// u32 returns a pointer to a uint32 for optional capability fields.
func u32(v uint32) *uint32 { return &v }

func fullCapabilities() *herdr.Capabilities {
	return &herdr.Capabilities{LiveHandoff: true, DetachedServerDaemon: true, HealthCheck: true, SurfaceInterest: true, EndpointProtocolGeneration: u32(1)}
}

// integrations lists six targets; the named ones are available.
func integrations(available ...string) []herdr.IntegrationInfo {
	set := map[string]bool{}
	for _, t := range available {
		set[t] = true
	}
	var infos []herdr.IntegrationInfo
	for _, kind := range []string{"pi", "codex", "claude", "opencode", "cursor", "grok"} {
		infos = append(infos, herdr.IntegrationInfo{Target: kind, Label: kind, Command: kind, Available: set[kind]})
	}
	return infos
}

func notInstalled() report.ModelData {
	no := false
	var hs []report.ModelHarness
	for _, kind := range []string{"pi", "codex", "claude"} {
		hs = append(hs, report.ModelHarness{Harness: kind, Available: &no, Status: report.Warn, Detail: "harness not installed"})
	}
	return report.ModelData{Harnesses: hs}
}

func connectivityOK() report.Check {
	return report.Check{Name: "herdr_connectivity", Status: report.OK, Detail: "Herdr socket responded to ping", Human: "socket responded to ping"}
}

func compatibility(caps *herdr.Capabilities) report.Check {
	data := report.CompatibilityData{Version: "0.9.1", Protocol: 22, Expected: 22, Capabilities: caps}
	return report.Check{Name: "herdr_compatibility", Status: report.OK, Detail: "version=0.9.1 protocol=22 capabilities=...", Data: data}
}

// mismatch mirrors the checks package's protocol-mismatch warning with every
// capability enabled: the JSON detail is comma-joined, the human one spaced.
func mismatch() report.Check {
	data := report.CompatibilityData{Version: "0.9.1", Protocol: 21, Expected: 22, Capabilities: fullCapabilities()}
	return report.Check{
		Name: "herdr_compatibility", Status: report.Warn, Data: data,
		Detail: "protocol 21 != pinned 22; version=0.9.1 protocol=21 capabilities=live_handoff,detached_server_daemon,health_check,surface_interest,endpoint_protocol_generation=1",
		Human:  "protocol 21 != pinned 22; version=0.9.1 protocol=21 capabilities=live_handoff, detached_server_daemon, health_check, surface_interest, endpoint_protocol_generation=1",
	}
}

func harnessOK() report.Check {
	return report.Check{Name: "harness_installations", Status: report.OK, Detail: "6 targets, 1 available: opencode", Data: report.HarnessData{Integrations: integrations("opencode")}}
}

func modelsWarn() report.Check {
	return report.Check{Name: "model_discovery", Status: report.Warn, Detail: "pi:warn(0) codex:warn(0) claude:warn(0)", Data: notInstalled()}
}

func config(socketPath, socketField, cwd string) report.Check {
	data := report.ConfigData{HerdrEnv: "1", SocketPath: socketPath, SocketMode: "0600", PaneID: "w1:p2", Session: "main", Cwd: cwd, SocketField: socketField}
	return report.Check{Name: "configuration", Status: report.OK, Detail: "HERDR_ENV=1 socket=" + socketField + " pane=w1:p2 session=main cwd=" + cwd, Data: data}
}

// healthy is the report the healthy doctor scenario produces.
func healthy(caps *herdr.Capabilities) report.Report {
	return report.New([]report.Check{connectivityOK(), compatibility(caps), harnessOK(), modelsWarn(), config("/run/herdr.sock", "/run/herdr.sock (0600)", "/work/dir")})
}

func render(t *testing.T, r report.Report, asJSON, verbose bool) string {
	t.Helper()
	var b strings.Builder
	if err := r.Write(&b, asJSON, verbose); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func assertWithinWidth(t *testing.T, out string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 80 {
			t.Fatalf("rendered line exceeds 80 columns (%d): %q", len(line), line)
		}
	}
}

func TestNewTalliesSummary(t *testing.T) {
	r := report.New([]report.Check{{Status: report.OK}, {Status: report.Warn}, {Status: report.Fail}, {Status: report.Fail}})
	if r.Summary != (report.Summary{OK: 1, Warn: 1, Fail: 2}) {
		t.Fatalf("summary = %+v", r.Summary)
	}
}

func TestWorst(t *testing.T) {
	for _, tc := range []struct{ a, b, want report.Status }{
		{report.OK, report.OK, report.OK},
		{report.OK, report.Warn, report.Warn},
		{report.Warn, report.OK, report.Warn},
		{report.Warn, report.Fail, report.Fail},
		{report.Fail, report.Warn, report.Fail},
	} {
		if got := report.Worst(tc.a, tc.b); got != tc.want {
			t.Fatalf("Worst(%s, %s) = %s, want %s", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCapabilityNames(t *testing.T) {
	if got := (report.CompatibilityData{}).CapabilityNames(); len(got) != 0 {
		t.Fatalf("nil capabilities = %q", got)
	}
	want := []string{"live_handoff", "detached_server_daemon", "health_check", "surface_interest", "endpoint_protocol_generation=1"}
	if got := (report.CompatibilityData{Capabilities: fullCapabilities()}).CapabilityNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %q, want %q", got, want)
	}
}

func TestReportErrorIsRendered(t *testing.T) {
	err := error(&report.ReportError{Report: report.Report{Summary: report.Summary{Fail: 3}}})
	if err.Error() != "3 checks failed" {
		t.Fatalf("message = %q", err.Error())
	}
	if !cli.IsRendered(err) {
		t.Fatal("a report error was already written and must be marked rendered")
	}
	var status interface{ ExitCode() int }
	if !errors.As(err, &status) || status.ExitCode() != 1 {
		t.Fatalf("want exit code 1, got %#v", err)
	}
}

func TestWriteHumanGroupedBlocks(t *testing.T) {
	out := render(t, healthy(&herdr.Capabilities{LiveHandoff: true, HealthCheck: true}), false, false)
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
	out := render(t, healthy(&herdr.Capabilities{LiveHandoff: true, HealthCheck: true}), false, false)
	// The capabilities list and the available-name list are verbose-only.
	for _, hidden := range []string{"capabilities:", "available:", "live_handoff", "opencode\n"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("default output leaked verbose detail %q:\n%s", hidden, out)
		}
	}
}

func TestWriteHumanVerboseRevealsLongDetail(t *testing.T) {
	out := render(t, healthy(&herdr.Capabilities{LiveHandoff: true, HealthCheck: true}), false, true)
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
	// Connectivity fails (no ping) and compatibility warns without data; both
	// diagnostics must appear with and without --verbose.
	failing := report.New([]report.Check{
		{Name: "herdr_connectivity", Status: report.Fail, Detail: "ping failed: dial unix /tmp/missing.sock: no such file"},
		{Name: "herdr_compatibility", Status: report.Warn, Detail: "protocol unknown: Herdr did not respond to ping"},
	})
	for _, verbose := range []bool{false, true} {
		out := render(t, failing, false, verbose)
		if !strings.Contains(out, "  herdr_connectivity    fail\n") {
			t.Fatalf("verbose=%v: missing fail status line:\n%s", verbose, out)
		}
		if !strings.Contains(out, "    ping failed: dial unix /tmp/missing.sock: no such file\n") {
			t.Fatalf("verbose=%v: missing connectivity diagnostic:\n%s", verbose, out)
		}
		if !strings.Contains(out, "    protocol unknown: Herdr did not respond to ping\n") {
			t.Fatalf("verbose=%v: missing compatibility diagnostic:\n%s", verbose, out)
		}
	}

	// A protocol mismatch is a warn whose diagnostic detail is always shown.
	out := render(t, report.New([]report.Check{connectivityOK(), mismatch()}), false, false)
	if !strings.Contains(out, "  herdr_compatibility   warn\n") || !strings.Contains(out, "!= pinned 22") {
		t.Fatalf("mismatch warn detail missing:\n%s", out)
	}
}

func TestHumanOverrideAndJSONDetail(t *testing.T) {
	// The stored JSON detail keeps the original wording; only the human line is
	// shortened. The two are separated so --json content is unchanged.
	r := report.New([]report.Check{connectivityOK()})
	if j := render(t, r, true, false); !strings.Contains(j, `"detail":"Herdr socket responded to ping"`) || strings.Contains(j, "Human") {
		t.Fatalf("JSON detail wording changed:\n%s", j)
	}
	// Human output shows the shortened phrasing and never the JSON wording.
	if h := render(t, r, false, false); !strings.Contains(h, "    socket responded to ping\n") || strings.Contains(h, "Herdr socket responded") {
		t.Fatalf("human wording wrong:\n%s", h)
	}
}

func TestWriteHumanShortensHomePathsButJSONKeepsAbsolute(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sock := filepath.Join(home, ".config", "herdr", "herdr.sock")
	proj := filepath.Join(home, "source", "fledge")
	r := report.New([]report.Check{config(sock, sock+" (0600)", proj)})

	h := render(t, r, false, false)
	if !strings.Contains(h, "~/.config/herdr/herdr.sock (0600)") {
		t.Fatalf("socket path not shortened to ~:\n%s", h)
	}
	if !strings.Contains(h, "~/source/fledge") {
		t.Fatalf("cwd not shortened to ~:\n%s", h)
	}
	if strings.Contains(h, home) {
		t.Fatalf("human output leaked absolute home %q:\n%s", home, h)
	}

	j := render(t, r, true, false)
	if !strings.Contains(j, sock) || !strings.Contains(j, proj) {
		t.Fatalf("json paths must stay absolute:\n%s", j)
	}
	if strings.Contains(j, "~/.config") || strings.Contains(j, "~/source") {
		t.Fatalf("json paths must not be shortened:\n%s", j)
	}
}

func TestWriteHumanShortensBareHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	out := render(t, report.New([]report.Check{config("/run/herdr.sock", "/run/herdr.sock (0600)", home)}), false, false)
	if !strings.Contains(out, "    cwd            ~\n") {
		t.Fatalf("bare home not shortened:\n%s", out)
	}
}

func TestWriteHumanVerboseWrapsListsWithinWidth(t *testing.T) {
	// The only wrapping candidate is the long capabilities list. (Unbreakable
	// diagnostics such as long paths are deliberately excluded: a token is never
	// split to fit 80 columns.)
	out := render(t, healthy(fullCapabilities()), false, true)
	assertWithinWidth(t, out)
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
	c := mismatch()
	data := c.Data.(report.CompatibilityData)
	data.Capabilities = &herdr.Capabilities{LiveHandoff: true, HealthCheck: true}
	c.Data = data
	c.Human = "protocol 21 != pinned 22; version=0.9.1 protocol=21 capabilities=live_handoff, health_check"
	out := render(t, report.New([]report.Check{c}), false, true)
	if !strings.Contains(out, "  herdr_compatibility   warn\n") {
		t.Fatalf("expected compatibility warn:\n%s", out)
	}
	if !strings.Contains(out, "!= pinned 22") {
		t.Fatalf("warn diagnostic missing:\n%s", out)
	}
	if !strings.Contains(out, "capabilities: live_handoff, health_check") {
		t.Fatalf("capabilities line missing on warn:\n%s", out)
	}
	// The mismatch diagnostic itself must wrap to stay within 80 columns.
	assertWithinWidth(t, out)
	// The pinned-protocol value must remain a whole token (never split).
	if !strings.Contains(out, "pinned 22") {
		t.Fatalf("mismatch token was split:\n%s", out)
	}
}

func TestWriteHumanVerboseCompatWarnFullCapabilitiesWithinWidth(t *testing.T) {
	// The warn diagnostic embeds the capability list, which must break at the
	// commas so no verbose line exceeds 80 columns, while the mismatch info and
	// the separate capabilities: line both remain present.
	r := report.New([]report.Check{connectivityOK(), mismatch()})
	out := render(t, r, false, true)
	assertWithinWidth(t, out)
	if !strings.Contains(out, "  herdr_compatibility   warn\n") || !strings.Contains(out, "!= pinned 22") {
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
	if !strings.Contains(render(t, r, true, true), `capabilities=live_handoff,detached_server_daemon,health_check,surface_interest,endpoint_protocol_generation=1`) {
		t.Fatal("JSON detail must remain comma-joined")
	}
}

func TestWriteHumanWrapsModelDiagnosticWithinWidth(t *testing.T) {
	// Word-wrapping keeps every line within 80 columns because the only long
	// token (the cache path) is itself under 80.
	yes := true
	var hs []report.ModelHarness
	for kind, path := range map[string]string{"pi": ".pi/agent/models-store.json", "codex": ".codex/models_cache.json", "claude": ".claude/cache/model-catalog"} {
		hs = append(hs, report.ModelHarness{Harness: kind, Available: &yes, Status: report.Fail, Detail: "discovery error: open /nonexistent-home-fledge/" + path + ": no such file or directory"})
	}
	out := render(t, report.New([]report.Check{{Name: "model_discovery", Status: report.Fail, Data: report.ModelData{Harnesses: hs}}}), false, false)
	if !strings.Contains(out, "discovery error") {
		t.Fatalf("expected a discovery-error diagnostic:\n%s", out)
	}
	assertWithinWidth(t, out)
	// The cache path token must survive intact (never split across lines).
	if !strings.Contains(out, "models-store.json") {
		t.Fatalf("path token was split:\n%s", out)
	}
}

func TestWriteJSONStableShape(t *testing.T) {
	r := healthy(&herdr.Capabilities{LiveHandoff: true, HealthCheck: true})
	out := render(t, r, true, false)
	var doc struct {
		Checks []struct {
			Name, Status, Detail string
		} `json:"checks"`
		Summary struct{ OK, Warn, Fail int } `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
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
	// Human-only fields never reach the JSON document.
	if strings.Contains(out, "SocketField") || strings.Contains(out, "socket_field") {
		t.Fatalf("human-only field serialized:\n%s", out)
	}

	// --verbose must not alter the JSON document at all.
	if verbose := render(t, r, true, true); verbose != out {
		t.Fatalf("verbose changed JSON output:\n%s\nvs\n%s", verbose, out)
	}
}

// failingWriter fails every write.
type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestWritePropagatesWriterFailure(t *testing.T) {
	sentinel := errors.New("write failed")
	r := healthy(nil)
	for _, asJSON := range []bool{false, true} {
		if err := r.Write(failingWriter{sentinel}, asJSON, false); !errors.Is(err, sentinel) {
			t.Fatalf("json=%v: got %v, want %v", asJSON, err, sentinel)
		}
	}
}
