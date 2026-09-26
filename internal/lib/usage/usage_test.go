package usage

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	outputs map[string]string
	calls   []string
	ctxs    []context.Context
}

func (f *fakeRunner) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	f.ctxs = append(f.ctxs, ctx)
	out, ok := f.outputs[key]
	if !ok {
		return nil, errors.New("executable not found")
	}
	return []byte(out), nil
}

// noRun fails the test if a file-based reader executes a command.
func noRun(t *testing.T) Runner {
	return func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("file readers must not run commands")
		return nil, nil
	}
}

func fixture(t *testing.T) Discovery {
	t.Helper()
	home, err := filepath.Abs(filepath.Join("testdata", "home"))
	if err != nil {
		t.Fatal(err)
	}
	return Discovery{Home: home, Run: noRun(t)}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func at(s string) *time.Time {
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &v
}

func window(from, to string) Window { return Window{From: at(from), To: at(to)} }

func assertTokens(t *testing.T, got, want Tokens) {
	t.Helper()
	if got != want {
		t.Fatalf("tokens %+v want %+v", got, want)
	}
}

func assertBasis(t *testing.T, s Summary, basis string) {
	t.Helper()
	if s.Basis != basis {
		t.Fatalf("basis %q want %q (reason %q)", s.Basis, basis, s.Reason)
	}
}

func TestReadUnknownHarnessIsUnavailable(t *testing.T) {
	s := Read(context.Background(), fixture(t), "nope", Ref{Kind: "id", Value: "x"}, Window{})
	assertBasis(t, s, Unavailable)
	if !strings.Contains(s.Reason, "unknown harness") {
		t.Fatalf("reason %q", s.Reason)
	}
}

func TestReadHarnessWithoutUsageIsUnavailable(t *testing.T) {
	s := Read(context.Background(), fixture(t), "cursor", Ref{Kind: "id", Value: "chat-1"}, Window{})
	assertBasis(t, s, Unavailable)
	if s.Reason != "chat store is encrypted" {
		t.Fatalf("reason %q", s.Reason)
	}
	if s.Harness != "cursor" || s.SessionKind != "id" || s.SessionValue != "chat-1" {
		t.Fatalf("identity %+v", s)
	}
}

func TestReadHarnessWithoutReaderIsUnavailable(t *testing.T) {
	s := Read(context.Background(), fixture(t), "gemini", Ref{Kind: "id", Value: "x"}, Window{})
	assertBasis(t, s, Unavailable)
	if !strings.Contains(s.Reason, "no usage reader") {
		t.Fatalf("reason %q", s.Reason)
	}
}

func TestReadMissingFileIsUnavailable(t *testing.T) {
	d := fixture(t)
	for kind, ref := range map[string]Ref{
		"claude": {Kind: "id", Value: "missing", Cwd: "/home/user/proj.x/app"},
		"codex":  {Kind: "id", Value: "missing"},
		"pi":     {Kind: "path", Value: filepath.Join(d.Home, "nothing.jsonl")},
	} {
		s := Read(context.Background(), d, kind, ref, Window{})
		assertBasis(t, s, Unavailable)
		if s.Reason == "" {
			t.Fatalf("%s: empty reason", kind)
		}
	}
}

func TestReadMalformedLinesAreCountedAndValidLinesSummed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, `{"type":"session","version":3,"id":"s"}
{"type":"message","timestamp":"2026-01-03T10:00:05.000Z","message":{"role":"assistant","model":"m","provider":"p","usage":{"input":1,"output":2,"cacheRead":3,"cacheWrite":4,"reasoning":0,"cost":{"total":0.5}}}}
{not json
{"type":"message","timestamp":"2026-01-03T10:00:06.000Z","message":{"role":"assistant","usage":{"input":"many"}}}

{"type":"message","timestamp":"2026-01-03T10:00:07.000Z","message":{"role":"assistant","model":"m","provider":"p","usage":{"input":10,"output":20,"cacheRead":0,"cacheWrite":0,"reasoning":0,"cost":{"total":0.25}}}}
`)
	s := Read(context.Background(), Discovery{Run: noRun(t)}, "pi", Ref{Kind: "path", Value: path}, Window{})
	assertBasis(t, s, Measured)
	assertTokens(t, s.Tokens, Tokens{Input: 11, Output: 22, CacheRead: 3, CacheWrite: 4})
	if !strings.Contains(s.Reason, "2 malformed entries") {
		t.Fatalf("reason %q", s.Reason)
	}
}

func TestReadOnlyMalformedLinesIsUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, "{broken\nalso broken\n")
	s := Read(context.Background(), Discovery{Run: noRun(t)}, "pi", Ref{Kind: "path", Value: path}, Window{})
	assertBasis(t, s, Unavailable)
	if !strings.Contains(s.Reason, "2 malformed entries") {
		t.Fatalf("reason %q", s.Reason)
	}
}

func TestClaudeSumsDedupedRequestsAndSubagents(t *testing.T) {
	d := fixture(t)
	s := Read(context.Background(), d, "claude", Ref{Kind: "id", Value: "sess-1", Cwd: "/home/user/proj.x/app"}, Window{})
	assertBasis(t, s, Measured)
	// req-a keeps its last line (output 7); req-b; req-c once; sub-agent req-s.
	assertTokens(t, s.Tokens, Tokens{Input: 64, Output: 73, CacheRead: 300, CacheWrite: 25})
	if s.Subagents == nil {
		t.Fatal("no sub-agent totals")
	}
	assertTokens(t, *s.Subagents, Tokens{Input: 50, Output: 60, CacheWrite: 5})
	if s.Turns != 4 {
		t.Fatalf("turns %d", s.Turns)
	}
	if want := []string{"claude-opus-5", "claude-haiku-4-5"}; !reflect.DeepEqual(s.Models, want) {
		t.Fatalf("models %q", s.Models)
	}
	if s.Cost != nil {
		t.Fatalf("claude records no cost, got %+v", s.Cost)
	}
	if !s.First.Equal(*at("2026-01-02T10:00:01Z")) || !s.Last.Equal(*at("2026-01-02T11:00:01Z")) {
		t.Fatalf("first %v last %v", s.First, s.Last)
	}
	if len(s.Sources) != 2 {
		t.Fatalf("sources %q", s.Sources)
	}
}

func TestClaudePathRefFindsSubagents(t *testing.T) {
	d := fixture(t)
	path := filepath.Join(d.Home, ".claude", "projects", "-home-user-proj-x-app", "sess-1.jsonl")
	s := Read(context.Background(), d, "claude", Ref{Kind: "path", Value: path}, Window{})
	assertBasis(t, s, Measured)
	assertTokens(t, s.Tokens, Tokens{Input: 64, Output: 73, CacheRead: 300, CacheWrite: 25})
}

func TestClaudeIDResolvesThroughCwdSlug(t *testing.T) {
	d := fixture(t)
	got, err := Locate(d, "claude", Ref{Kind: "id", Value: "sess-1", Cwd: "/home/user/proj.x/app"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(d.Home, ".claude", "projects", "-home-user-proj-x-app", "sess-1.jsonl")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Claude Code 2026-09 named the project of
// ".../probe_dir with space+plus~t@x" "...-probe-dir-with-space-plus-t-x":
// every character but an ASCII letter or digit becomes '-'.
func TestClaudeSlugReplacesEveryNonAlphanumeric(t *testing.T) {
	if got, want := claudeSlug("/home/u/.x/probe_dir with space+plus~t@x"), "-home-u--x-probe-dir-with-space-plus-t-x"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// A cwd whose slug names no transcript falls back to the search of every
// project, since the slug is Claude's to choose.
func TestClaudeIDWithCwdFallsBackToAnyProjectSlug(t *testing.T) {
	d := fixture(t)
	got, err := Locate(d, "claude", Ref{Kind: "id", Value: "sess-1", Cwd: "/elsewhere"})
	if want := filepath.Join(d.Home, ".claude", "projects", "-home-user-proj-x-app", "sess-1.jsonl"); err != nil || got != want {
		t.Fatalf("got %q (%v) want %q", got, err, want)
	}
}

// An id ref without a cwd, as for an ended agent, resolves under any project
// slug when exactly one transcript has that id, sub-agents included.
func TestClaudeIDWithoutCwdResolvesUnderAnyProjectSlug(t *testing.T) {
	d := fixture(t)
	got, err := Locate(d, "claude", Ref{Kind: "id", Value: "sess-1"})
	if want := filepath.Join(d.Home, ".claude", "projects", "-home-user-proj-x-app", "sess-1.jsonl"); err != nil || got != want {
		t.Fatalf("got %q (%v) want %q", got, err, want)
	}
	s := Read(context.Background(), d, "claude", Ref{Kind: "id", Value: "sess-1"}, Window{})
	assertBasis(t, s, Measured)
	assertTokens(t, s.Tokens, Tokens{Input: 64, Output: 73, CacheRead: 300, CacheWrite: 25})
	if s.Subagents == nil {
		t.Fatal("no sub-agent totals")
	}
}

func TestClaudeIDWithoutCwdMissingIsUnavailable(t *testing.T) {
	s := Read(context.Background(), fixture(t), "claude", Ref{Kind: "id", Value: "sess-none"}, Window{})
	assertBasis(t, s, Unavailable)
	if !strings.Contains(s.Reason, "no claude session file for id sess-none") {
		t.Fatalf("reason %q", s.Reason)
	}
}

// Two projects holding the same session id cannot be told apart without a cwd.
func TestClaudeIDWithoutCwdAmbiguousIsUnavailable(t *testing.T) {
	d := Discovery{Home: t.TempDir(), Run: noRun(t)}
	line := `{"type":"assistant","sessionId":"dup","requestId":"r","timestamp":"2026-01-02T10:00:00.000Z","message":{"model":"m","usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"
	for _, slug := range []string{"-a", "-b"} {
		write(t, filepath.Join(d.Home, ".claude", "projects", slug, "dup.jsonl"), line)
	}
	s := Read(context.Background(), d, "claude", Ref{Kind: "id", Value: "dup"}, Window{})
	assertBasis(t, s, Unavailable)
	if !strings.Contains(s.Reason, "ambiguous") || !strings.Contains(s.Reason, "2 claude session files") {
		t.Fatalf("reason %q", s.Reason)
	}
}

func TestClaudeWindow(t *testing.T) {
	s := Read(context.Background(), fixture(t), "claude", Ref{Kind: "id", Value: "sess-1", Cwd: "/home/user/proj.x/app"},
		window("2026-01-02T10:04:00Z", "2026-01-02T10:40:00Z"))
	assertBasis(t, s, Measured)
	// req-b inside; req-a and req-c outside; sub-agent req-s inside.
	assertTokens(t, s.Tokens, Tokens{Input: 53, Output: 64, CacheRead: 200, CacheWrite: 5})
	if s.Turns != 2 {
		t.Fatalf("turns %d", s.Turns)
	}
}

func TestCodexSumsDedupedRecords(t *testing.T) {
	s := Read(context.Background(), fixture(t), "codex", Ref{Kind: "id", Value: "cdx-1"}, Window{})
	assertBasis(t, s, Measured)
	assertTokens(t, s.Tokens, Tokens{Input: 980, Output: 80, CacheRead: 500, CacheWrite: 20, Reasoning: 10})
	if s.Turns != 2 {
		t.Fatalf("turns %d", s.Turns)
	}
	if want := []string{"gpt-5.3-codex", "gpt-5.6-sol"}; !reflect.DeepEqual(s.Models, want) {
		t.Fatalf("models %q", s.Models)
	}
	if s.Cost != nil || s.Reason != "" {
		t.Fatalf("cost %+v reason %q", s.Cost, s.Reason)
	}
}

func TestCodexIDResolvesThroughRolloutGlob(t *testing.T) {
	d := fixture(t)
	got, err := Locate(d, "codex", Ref{Kind: "id", Value: "cdx-1"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(d.Home, ".codex", "sessions", "2026", "01", "02", "rollout-2026-01-02T10-00-00-cdx-1.jsonl")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCodexWindow(t *testing.T) {
	s := Read(context.Background(), fixture(t), "codex", Ref{Kind: "id", Value: "cdx-1"},
		window("2026-01-02T10:30:00Z", "2026-01-02T12:00:00Z"))
	assertTokens(t, s.Tokens, Tokens{Input: 380, Output: 30, CacheRead: 100, CacheWrite: 20})
	if want := []string{"gpt-5.6-sol"}; !reflect.DeepEqual(s.Models, want) {
		t.Fatalf("models %q", s.Models)
	}
}

func TestCodexFallsBackToLastTokenCount(t *testing.T) {
	s := Read(context.Background(), fixture(t), "codex", Ref{Kind: "id", Value: "cdx-old"}, Window{})
	assertBasis(t, s, Measured)
	assertTokens(t, s.Tokens, Tokens{Input: 200, Output: 40, CacheRead: 100, Reasoning: 5})
	if !strings.Contains(s.Reason, "token_count") {
		t.Fatalf("reason %q", s.Reason)
	}
}

func TestCodexFallbackWindowSubtractsEarlierTotal(t *testing.T) {
	s := Read(context.Background(), fixture(t), "codex", Ref{Kind: "id", Value: "cdx-old"},
		window("2026-01-02T12:30:00Z", "2026-01-02T14:00:00Z"))
	assertBasis(t, s, Measured)
	// 300/100/40/5 at 13:00 minus 120/20/15/2 before the window.
	assertTokens(t, s.Tokens, Tokens{Input: 100, Output: 25, CacheRead: 80, Reasoning: 3})
}

func TestPiSumsUsageAndCost(t *testing.T) {
	s := Read(context.Background(), fixture(t), "pi", Ref{Kind: "id", Value: "pi-1"}, Window{})
	assertBasis(t, s, Measured)
	assertTokens(t, s.Tokens, Tokens{Input: 150, Output: 30, CacheRead: 300, CacheWrite: 10, Reasoning: 5})
	if s.Turns != 2 {
		t.Fatalf("turns %d", s.Turns)
	}
	if want := []string{"prov-a/model-one", "prov-b/model-two"}; !reflect.DeepEqual(s.Models, want) {
		t.Fatalf("models %q", s.Models)
	}
	if s.Cost == nil || math.Abs(s.Cost.Amount-0.015) > 1e-9 || s.Cost.Basis != "estimate" || s.Cost.Source != "pi price table" || s.Cost.Currency != "USD" {
		t.Fatalf("cost %+v", s.Cost)
	}
}

func TestPiIDResolvesUnderAnyProjectSlug(t *testing.T) {
	d := fixture(t)
	got, err := Locate(d, "pi", Ref{Kind: "id", Value: "pi-1"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(d.Home, ".pi", "agent", "sessions", "--home-user-proj--", "2026-01-03T10-00-00-000Z_pi-1.jsonl")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPiWindow(t *testing.T) {
	s := Read(context.Background(), fixture(t), "pi", Ref{Kind: "id", Value: "pi-1"},
		window("2026-01-03T10:05:00Z", "2026-01-03T11:00:00Z"))
	assertTokens(t, s.Tokens, Tokens{Input: 50, Output: 10})
	if s.Cost == nil || math.Abs(s.Cost.Amount-0.005) > 1e-9 {
		t.Fatalf("cost %+v", s.Cost)
	}
}

func opencodeRunner(t *testing.T) *fakeRunner {
	t.Helper()
	out, err := os.ReadFile(filepath.Join("testdata", "opencode-export.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &fakeRunner{outputs: map[string]string{"opencode export oc-1": string(out)}}
}

// lockedHome is a home no reader can enter, holding an opencode database.
func lockedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	write(t, filepath.Join(home, ".local", "share", "opencode", "opencode.db"), "synthetic")
	if err := os.Chmod(home, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o755) })
	return home
}

func TestOpencodeReadsExportAndNeverTheDatabase(t *testing.T) {
	r := opencodeRunner(t)
	s := Read(context.Background(), Discovery{Home: lockedHome(t), Run: r.run}, "opencode", Ref{Kind: "id", Value: "oc-1"}, Window{})
	assertBasis(t, s, Measured)
	if want := []string{"opencode export oc-1"}; !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls %q", r.calls)
	}
	if _, ok := r.ctxs[0].Deadline(); !ok {
		t.Fatal("opencode export must be time-boxed")
	}
	assertTokens(t, s.Tokens, Tokens{Input: 110, Output: 22, CacheRead: 300, CacheWrite: 10, Reasoning: 5})
	if s.Turns != 2 {
		t.Fatalf("turns %d", s.Turns)
	}
	if want := []string{"prov-a/model-one", "prov-b/model-two"}; !reflect.DeepEqual(s.Models, want) {
		t.Fatalf("models %q", s.Models)
	}
	if s.Cost == nil || math.Abs(s.Cost.Amount-0.03) > 1e-9 || s.Cost.Basis != "estimate" || s.Cost.Source != "opencode" {
		t.Fatalf("cost %+v", s.Cost)
	}
	if !reflect.DeepEqual(s.Sources, []string{"opencode export oc-1"}) {
		t.Fatalf("sources %q", s.Sources)
	}
	// User messages carry no tokens and are not usage records.
	if s.Reason != "" {
		t.Fatalf("reason %q", s.Reason)
	}
}

func TestOpencodeWindow(t *testing.T) {
	r := opencodeRunner(t)
	s := Read(context.Background(), Discovery{Run: r.run}, "opencode", Ref{Kind: "id", Value: "oc-1"},
		window("2026-01-03T10:05:00Z", "2026-01-03T11:00:00Z"))
	assertTokens(t, s.Tokens, Tokens{Input: 10, Output: 2})
	if s.Cost == nil || math.Abs(s.Cost.Amount-0.01) > 1e-9 {
		t.Fatalf("cost %+v", s.Cost)
	}
}

func TestOpencodeExportFailureIsUnavailable(t *testing.T) {
	s := Read(context.Background(), Discovery{Run: (&fakeRunner{}).run}, "opencode", Ref{Kind: "id", Value: "oc-1"}, Window{})
	assertBasis(t, s, Unavailable)
	if !strings.Contains(s.Reason, "opencode export") {
		t.Fatalf("reason %q", s.Reason)
	}
}

func TestClaudeSkipsSyntheticMessages(t *testing.T) {
	// Claude writes zero-usage "<synthetic>" assistant lines that no model produced.
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, `{"type":"assistant","requestId":"req-a","timestamp":"2026-01-02T10:00:00.000Z","message":{"model":"claude-opus-5","usage":{"input_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":2}}}
{"type":"assistant","timestamp":"2026-01-02T10:01:00.000Z","message":{"model":"<synthetic>","usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0}}}
`)
	s := Read(context.Background(), Discovery{Run: noRun(t)}, "claude", Ref{Kind: "path", Value: path}, Window{})
	if s.Turns != 1 || !reflect.DeepEqual(s.Models, []string{"claude-opus-5"}) {
		t.Fatalf("turns %d models %q", s.Turns, s.Models)
	}
}

func TestUnrecognizedOrUnreadableSessionIsUnavailable(t *testing.T) {
	for name, content := range map[string]string{
		"empty object":             "{}\n",
		"metadata masks malformed": "{\"type\":\"session\"}\n{broken\n",
		"foreign type only":        "{\"type\":\"other\",\"payload\":{}}\n",
	} {
		for _, kind := range []string{"claude", "codex", "pi"} {
			path := filepath.Join(t.TempDir(), "s.jsonl")
			write(t, path, content)
			s := Read(context.Background(), Discovery{Run: noRun(t)}, kind, Ref{Kind: "path", Value: path}, Window{})
			if s.Basis != Unavailable || s.Reason == "" {
				t.Errorf("%s %s: basis %q reason %q", name, kind, s.Basis, s.Reason)
			}
			if strings.Contains(s.Reason, "token_count") {
				t.Errorf("%s %s: claims a token_count fallback: %q", name, kind, s.Reason)
			}
		}
	}
}

func TestClaudeUnrecognizedSubagentFileIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	write(t, path, `{"type":"assistant","requestId":"r","timestamp":"2026-01-02T10:00:00.000Z","message":{"model":"m","usage":{"input_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":2}}}
`)
	write(t, filepath.Join(dir, "s", "subagents", "agent-x.jsonl"), "{}\n")
	s := Read(context.Background(), Discovery{Run: noRun(t)}, "claude", Ref{Kind: "path", Value: path}, Window{})
	assertBasis(t, s, Unavailable)
}

func TestValidSessionWithoutResponsesIsMeasuredZero(t *testing.T) {
	late := window("2030-01-01T00:00:00Z", "2030-01-02T00:00:00Z")
	d := fixture(t)
	for kind, ref := range map[string]Ref{
		"claude": {Kind: "id", Value: "sess-1", Cwd: "/home/user/proj.x/app"},
		"codex":  {Kind: "id", Value: "cdx-1"},
		"pi":     {Kind: "id", Value: "pi-1"},
	} {
		s := Read(context.Background(), d, kind, ref, late)
		if s.Basis != Measured || s.Tokens != (Tokens{}) || s.Turns != 0 {
			t.Errorf("%s: %+v", kind, s)
		}
	}
	// A session that has not answered yet is a successful zero read.
	for kind, content := range map[string]string{
		"claude": `{"type":"user","timestamp":"2026-01-02T10:00:00.000Z","message":{"role":"user","content":"hi"}}` + "\n",
		// Claude writes metadata-only transcripts, each line naming the session, before any prompt.
		"claude-stub": `{"type":"mode","mode":"default","sessionId":"s"}` + "\n" + `{"type":"last-prompt","leafUuid":"u","sessionId":"s"}` + "\n",
		"codex":       `{"timestamp":"2026-01-02T10:00:00.000Z","type":"session_meta","payload":{"session_id":"c","id":"c"}}` + "\n",
		"pi":          `{"type":"session","version":3,"id":"p","timestamp":"2026-01-03T10:00:00.000Z"}` + "\n",
	} {
		path := filepath.Join(t.TempDir(), "s.jsonl")
		write(t, path, content)
		s := Read(context.Background(), Discovery{Run: noRun(t)}, strings.TrimSuffix(kind, "-stub"), Ref{Kind: "path", Value: path}, Window{})
		if s.Basis != Measured || s.Tokens != (Tokens{}) || s.Reason != "" {
			t.Errorf("%s header only: %+v", kind, s)
		}
	}
}

func TestOpencodeRejectsUnrecognizedExport(t *testing.T) {
	for _, out := range []string{`{"error":"session missing"}`, `{}`, `{"info":{"id":"oc-1"}}`, `{"messages":[]}`, `[]`} {
		r := &fakeRunner{outputs: map[string]string{"opencode export oc-1": out}}
		s := Read(context.Background(), Discovery{Run: r.run}, "opencode", Ref{Kind: "id", Value: "oc-1"}, Window{})
		if s.Basis != Unavailable || s.Cost != nil {
			t.Errorf("%s: %+v", out, s)
		}
	}
}

func TestOpencodeEmptySessionIsMeasuredZero(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{"opencode export oc-1": `{"info":{"id":"oc-1"},"messages":[]}`}}
	s := Read(context.Background(), Discovery{Run: r.run}, "opencode", Ref{Kind: "id", Value: "oc-1"}, Window{})
	assertBasis(t, s, Measured)
	if s.Tokens != (Tokens{}) || s.Cost == nil || s.Cost.Amount != 0 {
		t.Fatalf("%+v", s)
	}
}

// readFile reads content as a kind's session file.
func readFile(t *testing.T, kind, content string) Summary {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, content)
	return Read(context.Background(), Discovery{Run: noRun(t)}, kind, Ref{Kind: "path", Value: path}, Window{})
}

const (
	codexMeta   = `{"type":"session_meta","payload":{"id":"c"}}` + "\n"
	codexRecord = `{"type":"token_usage_record","timestamp":"2026-01-02T10:00:05Z","payload":{"response_id":"ok","usage":{"input_tokens":10,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":3,"reasoning_output_tokens":0}}}` + "\n"
	claudeValid = `{"type":"assistant","requestId":"ok","timestamp":"2026-01-02T10:00:00Z","message":{"model":"m","usage":{"input_tokens":10,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":3}}}` + "\n"
	piHeader    = `{"type":"session","version":3,"id":"p"}` + "\n"
	piValid     = `{"type":"message","timestamp":"2026-01-03T10:00:05Z","message":{"role":"assistant","model":"m","provider":"p","usage":{"input":10,"output":3,"cacheRead":0,"cacheWrite":0,"reasoning":0,"cost":{"total":0.1}}}}` + "\n"
)

// Absent or null usage on a relevant record is malformed, never a zero measurement.
func TestMissingOrNullUsageIsMalformed(t *testing.T) {
	cases := []struct {
		name, kind, header, valid string
		bad                       []string
	}{
		{"codex record", "codex", codexMeta, codexRecord, []string{
			`{"type":"token_usage_record","timestamp":"2026-01-02T10:00:05Z","payload":{"response_id":"r","usage":null}}`,
			`{"type":"token_usage_record","timestamp":"2026-01-02T10:00:05Z","payload":{"response_id":"r"}}`,
		}},
		{"codex token_count", "codex", codexMeta, "", []string{
			`{"type":"event_msg","timestamp":"2026-01-02T10:00:05Z","payload":{"type":"token_count","info":{}}}`,
			`{"type":"event_msg","timestamp":"2026-01-02T10:00:05Z","payload":{"type":"token_count","info":{"total_token_usage":null}}}`,
		}},
		{"claude", "claude", "", claudeValid, []string{
			`{"type":"assistant","requestId":"r","sessionId":"s","timestamp":"2026-01-02T10:00:00Z","message":{"model":"m"}}`,
			`{"type":"assistant","requestId":"r","sessionId":"s","timestamp":"2026-01-02T10:00:00Z","message":{"model":"m","usage":null}}`,
		}},
		{"pi", "pi", piHeader, piValid, []string{
			`{"type":"message","timestamp":"2026-01-03T10:00:05Z","message":{"role":"assistant","model":"m","provider":"p"}}`,
			`{"type":"message","timestamp":"2026-01-03T10:00:05Z","message":{"role":"assistant","model":"m","provider":"p","usage":null}}`,
		}},
	}
	for _, c := range cases {
		for _, bad := range c.bad {
			s := readFile(t, c.kind, c.header+bad+"\n")
			if s.Basis != Unavailable || strings.Contains(s.Reason, "token_count") {
				t.Errorf("%s alone %s: basis %q reason %q turns %d", c.name, bad, s.Basis, s.Reason, s.Turns)
			}
			if c.valid == "" {
				continue
			}
			s = readFile(t, c.kind, c.header+c.valid+bad+"\n")
			if s.Basis != Measured || s.Turns != 1 || s.Tokens.Input != 10 || !strings.Contains(s.Reason, "1 malformed entries") {
				t.Errorf("%s mixed %s: %+v", c.name, bad, s)
			}
		}
	}
}

func TestExplicitZeroUsageAndNullCodexInfoAreMeasured(t *testing.T) {
	for kind, content := range map[string]string{
		"codex":  codexMeta + `{"type":"token_usage_record","timestamp":"2026-01-02T10:00:05Z","payload":{"response_id":"r","usage":{"input_tokens":0,"output_tokens":0}}}` + "\n",
		"claude": `{"type":"assistant","requestId":"r","timestamp":"2026-01-02T10:00:00Z","message":{"model":"m","usage":{"input_tokens":0,"output_tokens":0}}}` + "\n",
		"pi":     piHeader + `{"type":"message","timestamp":"2026-01-03T10:00:05Z","message":{"role":"assistant","model":"m","usage":{"input":0,"output":0}}}` + "\n",
	} {
		s := readFile(t, kind, content)
		if s.Basis != Measured || s.Turns != 1 || s.Reason != "" {
			t.Errorf("%s: %+v", kind, s)
		}
	}
	// Codex writes token_count with a null info before any usage exists.
	s := readFile(t, "codex", codexMeta+`{"type":"event_msg","timestamp":"2026-01-02T10:00:05Z","payload":{"type":"token_count","info":null}}`+"\n")
	if s.Basis != Measured || s.Reason != "" || s.Tokens != (Tokens{}) {
		t.Errorf("null info: %+v", s)
	}
}

func TestOpencodeMissingOrNullTokensIsMalformed(t *testing.T) {
	const valid = `{"info":{"role":"assistant","modelID":"m","providerID":"p","cost":0.25,"tokens":{"input":10,"output":3,"reasoning":0,"cache":{"read":0,"write":0}},"time":{"created":1767434405000}}}`
	for _, bad := range []string{
		`{"info":{"role":"assistant","modelID":"m","cost":0.5,"time":{"created":1767434405000}}}`,
		`{"info":{"role":"assistant","modelID":"m","cost":0.5,"tokens":null,"time":{"created":1767434405000}}}`,
	} {
		r := &fakeRunner{outputs: map[string]string{"opencode export s": `{"info":{"id":"s"},"messages":[` + bad + `]}`}}
		s := Read(context.Background(), Discovery{Run: r.run}, "opencode", Ref{Kind: "id", Value: "s"}, Window{})
		if s.Basis != Unavailable || s.Cost != nil {
			t.Errorf("alone %s: %+v", bad, s)
		}
		r = &fakeRunner{outputs: map[string]string{"opencode export s": `{"info":{"id":"s"},"messages":[` + valid + `,` + bad + `]}`}}
		s = Read(context.Background(), Discovery{Run: r.run}, "opencode", Ref{Kind: "id", Value: "s"}, Window{})
		if s.Basis != Measured || s.Turns != 1 || s.Tokens.Input != 10 || s.Cost == nil || s.Cost.Amount != 0.25 || !strings.Contains(s.Reason, "1 malformed entries") {
			t.Errorf("mixed %s: %+v", bad, s)
		}
	}
}

// Count rounds, so 99,960 is 100k, not 99.9k.
func TestCount(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1k", 1250: "1.2k", 34000: "34k", 99960: "100k", 410_400: "410k", 999999: "1M", 1000000: "1M", 2500000: "2.5M", 12345678: "12.3M"} {
		if got := Count(n); got != want {
			t.Errorf("Count(%d) = %q, want %q", n, got, want)
		}
	}
}
