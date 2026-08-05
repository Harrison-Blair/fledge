package agentcontext

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

const testHome = "/home/u"

// fixture wires an in-memory Deps. Transcript files are keyed by absolute path;
// OpenCode exports are keyed by session id. Glob uses filepath.Match, whose "*"
// matches a single path component — exactly how the real collectors glob.
type fixture struct {
	files   map[string]string
	exports map[string]string
	now     time.Time
}

func (f fixture) deps() Deps {
	return Deps{
		Home: testHome,
		Now:  func() time.Time { return f.now },
		ReadFile: func(path string) ([]byte, error) {
			contents, ok := f.files[path]
			if !ok {
				return nil, os.ErrNotExist
			}
			return []byte(contents), nil
		},
		Glob: func(pattern string) ([]string, error) {
			var matches []string
			for path := range f.files {
				if ok, err := filepath.Match(pattern, path); err == nil && ok {
					matches = append(matches, path)
				}
			}
			sort.Strings(matches)
			return matches, nil
		},
		OpenCodeExport: func(sessionID string) ([]byte, error) {
			contents, ok := f.exports[sessionID]
			if !ok {
				return nil, os.ErrNotExist
			}
			return []byte(contents), nil
		},
	}
}

func claudePath(id string) string {
	return filepath.Join(testHome, ".claude", "projects", "proj", id+".jsonl")
}

func codexPath(id string) string {
	return filepath.Join(testHome, ".codex", "sessions", "2026", "08", "04", "rollout-2026-08-04T19-50-16-"+id+".jsonl")
}

func piPath(id string) string {
	return filepath.Join(testHome, ".pi", "agent", "sessions", "proj", "2026-08-04T19-00-00-000Z_"+id+".jsonl")
}

// The Claude fixture carries two usage turns. The first is large and the second
// (post-compaction) is small, so a collector that reads anything but the latest
// turn fails. Output tokens are large on purpose: they must never appear in the
// used total.
const claudeTranscript = `{"type":"user","message":{"role":"user"}}
{"type":"assistant","message":{"model":"claude-opus-4-8","usage":{"input_tokens":5,"cache_creation_input_tokens":100,"cache_read_input_tokens":90000,"output_tokens":9999}},"timestamp":"2026-08-04T22:00:00Z"}
{"type":"assistant","message":{"model":"claude-opus-4-8","usage":{"input_tokens":2,"cache_creation_input_tokens":1000,"cache_read_input_tokens":20000,"output_tokens":8888}},"timestamp":"2026-08-04T23:00:00Z"}
`

func TestCollectClaudeUsesLatestInputAndCacheOnly(t *testing.T) {
	t.Parallel()
	fx := fixture{files: map[string]string{claudePath("sess-claude"): claudeTranscript}}
	got, err := collectClaude(Ref{Kind: "id", Value: "sess-claude"}, fx.deps())
	if err != nil {
		t.Fatalf("collectClaude() error = %v", err)
	}
	// 2 + 1000 + 20000 = 21002; the 8888 output tokens must be excluded.
	if got.used != 21002 {
		t.Errorf("used = %d, want 21002 (input+cache of the latest turn, no output)", got.used)
	}
	if !got.hasWindow || got.window != 200_000 {
		t.Errorf("window = %d (has=%v), want 200000", got.window, got.hasWindow)
	}
	if want := time.Date(2026, 8, 4, 23, 0, 0, 0, time.UTC); !got.observedAt.Equal(want) {
		t.Errorf("observedAt = %v, want %v", got.observedAt, want)
	}
}

func TestClaudeWindowDetectsMillionContext(t *testing.T) {
	t.Parallel()
	transcript := `{"type":"assistant","message":{"model":"claude-sonnet-5-1m","usage":{"input_tokens":10,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":0}}}` + "\n"
	fx := fixture{files: map[string]string{claudePath("big"): transcript}}
	got, err := collectClaude(Ref{Kind: "id", Value: "big"}, fx.deps())
	if err != nil {
		t.Fatalf("collectClaude() error = %v", err)
	}
	if got.window != 1_000_000 {
		t.Errorf("window = %d, want 1000000 for a 1m model", got.window)
	}
}

func TestClaudeAwaitingFirstResponseWhenNoUsage(t *testing.T) {
	t.Parallel()
	fx := fixture{files: map[string]string{claudePath("new"): `{"type":"user","message":{"role":"user"}}` + "\n"}}
	_, err := collectClaude(Ref{Kind: "id", Value: "new"}, fx.deps())
	if !errors.Is(err, errAwaitingFirstResponse) {
		t.Fatalf("collectClaude() error = %v, want errAwaitingFirstResponse", err)
	}
}

// A compaction boundary after the last usage turn makes the figure stale.
const claudeCompactedTranscript = `{"type":"assistant","message":{"model":"claude-opus-4-8","usage":{"input_tokens":2,"cache_creation_input_tokens":1000,"cache_read_input_tokens":20000,"output_tokens":10}},"timestamp":"2026-08-04T23:00:00Z"}
{"type":"system","subtype":"compact_boundary","content":"Conversation compacted","timestamp":"2026-08-04T23:05:00Z"}
`

func TestClaudeAfterCompactionSuppressesStaleFigure(t *testing.T) {
	t.Parallel()
	fx := fixture{files: map[string]string{claudePath("c"): claudeCompactedTranscript}}
	_, err := collectClaude(Ref{Kind: "id", Value: "c"}, fx.deps())
	if !errors.Is(err, errAfterCompaction) {
		t.Fatalf("collectClaude() error = %v, want errAfterCompaction", err)
	}
}

func TestClaudeCompletedResponseAfterCompactionIsAvailable(t *testing.T) {
	t.Parallel()
	// A fresh usage turn lands after the boundary: the figure is authoritative again.
	transcript := claudeCompactedTranscript +
		`{"type":"assistant","message":{"model":"claude-opus-4-8","usage":{"input_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":500,"output_tokens":3}},"timestamp":"2026-08-04T23:10:00Z"}` + "\n"
	fx := fixture{files: map[string]string{claudePath("c2"): transcript}}
	got, err := collectClaude(Ref{Kind: "id", Value: "c2"}, fx.deps())
	if err != nil {
		t.Fatalf("collectClaude() error = %v, want a reading", err)
	}
	if got.used != 501 {
		t.Errorf("used = %d, want 501 (post-compaction turn)", got.used)
	}
}

// The Codex fixture proves used comes from the latest token_count's
// last_token_usage.input_tokens and the model_context_window — never the
// cumulative total_tokens and never output/reasoning.
const codexTranscript = `{"type":"event_msg","timestamp":"2026-08-04T22:00:00.000Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":90000,"output_tokens":500},"model_context_window":258400,"total_token_usage":{"total_tokens":150000}}}}
{"type":"response_item","payload":{"type":"message"}}
{"type":"event_msg","timestamp":"2026-08-04T23:30:00.000Z","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":18194,"cached_input_tokens":17152,"output_tokens":92,"reasoning_output_tokens":15},"model_context_window":258400,"total_token_usage":{"total_tokens":178124}}}}
`

func TestCollectCodexUsesLastTokenUsageInputAndWindow(t *testing.T) {
	t.Parallel()
	fx := fixture{files: map[string]string{codexPath("sess-codex"): codexTranscript}}
	got, err := collectCodex(Ref{Kind: "id", Value: "sess-codex"}, fx.deps())
	if err != nil {
		t.Fatalf("collectCodex() error = %v", err)
	}
	if got.used != 18194 {
		t.Errorf("used = %d, want 18194 (last_token_usage.input_tokens, not total 178124 or output)", got.used)
	}
	if got.window != 258400 {
		t.Errorf("window = %d, want 258400 (model_context_window)", got.window)
	}
	if want := time.Date(2026, 8, 4, 23, 30, 0, 0, time.UTC); !got.observedAt.Equal(want) {
		t.Errorf("observedAt = %v, want %v", got.observedAt, want)
	}
}

func TestCollectCodexUnsupportedFormatWhenTokenCountLacksFields(t *testing.T) {
	t.Parallel()
	transcript := `{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":10}}}}` + "\n"
	fx := fixture{files: map[string]string{codexPath("weird"): transcript}}
	_, err := collectCodex(Ref{Kind: "id", Value: "weird"}, fx.deps())
	if !errors.Is(err, errUnsupportedFormat) {
		t.Fatalf("collectCodex() error = %v, want errUnsupportedFormat", err)
	}
}

func TestCodexAfterCompactionSuppressesStaleFigure(t *testing.T) {
	t.Parallel()
	transcript := codexTranscript +
		`{"type":"compacted","timestamp":"2026-08-04T23:40:00.000Z","payload":{"message":""}}` + "\n" +
		`{"type":"event_msg","timestamp":"2026-08-04T23:40:00.100Z","payload":{"type":"context_compacted"}}` + "\n"
	fx := fixture{files: map[string]string{codexPath("cc"): transcript}}
	_, err := collectCodex(Ref{Kind: "id", Value: "cc"}, fx.deps())
	if !errors.Is(err, errAfterCompaction) {
		t.Fatalf("collectCodex() error = %v, want errAfterCompaction", err)
	}
}

// The Pi fixture proves used = input + cacheRead + cacheWrite, excluding output
// and reasoning, and that Pi leaves the window unknown.
const piTranscript = `{"type":"message","message":{"role":"user","content":"hi"}}
{"type":"message","message":{"role":"assistant","usage":{"input":40000,"cacheRead":0,"cacheWrite":0,"output":11,"reasoning":9,"totalTokens":40020},"timestamp":1785880800000}}
{"type":"message","message":{"role":"assistant","usage":{"input":1124,"cacheRead":200,"cacheWrite":50,"output":31,"reasoning":24,"totalTokens":1429},"timestamp":1785884400000}}
`

func TestCollectPiUsesInputAndCacheExcludingOutput(t *testing.T) {
	t.Parallel()
	fx := fixture{files: map[string]string{piPath("sess-pi"): piTranscript}}
	got, err := collectPi(Ref{Kind: "id", Value: "sess-pi"}, fx.deps())
	if err != nil {
		t.Fatalf("collectPi() error = %v", err)
	}
	// 1124 + 200 + 50 = 1374; totalTokens 1429 (which folds in output+reasoning) must not be used.
	if got.used != 1374 {
		t.Errorf("used = %d, want 1374 (input+cacheRead+cacheWrite, not totalTokens)", got.used)
	}
	if got.hasWindow {
		t.Errorf("window should be unknown for Pi, got %d", got.window)
	}
	if want := time.UnixMilli(1785884400000).UTC(); !got.observedAt.Equal(want) {
		t.Errorf("observedAt = %v, want %v", got.observedAt, want)
	}
}

func TestCollectPiAcceptsLegacyRFC3339Timestamp(t *testing.T) {
	t.Parallel()
	transcript := `{"type":"message","message":{"role":"assistant","usage":{"input":1},"timestamp":"2026-08-04T23:00:00Z"}}` + "\n"
	fx := fixture{files: map[string]string{piPath("legacy-time"): transcript}}
	got, err := collectPi(Ref{Kind: "id", Value: "legacy-time"}, fx.deps())
	if err != nil {
		t.Fatalf("collectPi() error = %v", err)
	}
	if want := time.Date(2026, 8, 4, 23, 0, 0, 0, time.UTC); !got.observedAt.Equal(want) {
		t.Errorf("observedAt = %v, want %v", got.observedAt, want)
	}
}

func TestFileCollectorsReadExactHerdrPathReferences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		path       string
		transcript string
		collect    collector
		wantUsed   int
	}{
		{name: "claude", path: "/custom/claude/session.jsonl", transcript: claudeTranscript, collect: collectClaude, wantUsed: 21002},
		{name: "codex", path: "/custom/codex/rollout.jsonl", transcript: codexTranscript, collect: collectCodex, wantUsed: 18194},
		{name: "pi", path: "/custom/pi/session.jsonl", transcript: piTranscript, collect: collectPi, wantUsed: 1374},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := fixture{files: map[string]string{tc.path: tc.transcript}}
			got, err := tc.collect(Ref{Kind: "path", Value: tc.path}, fx.deps())
			if err != nil {
				t.Fatalf("collector(path) error = %v", err)
			}
			if got.used != tc.wantUsed {
				t.Errorf("used = %d, want %d", got.used, tc.wantUsed)
			}
		})
	}
}

func TestPathReferenceNeverFallsBackToIDLookup(t *testing.T) {
	t.Parallel()
	fx := fixture{files: map[string]string{piPath("session"): piTranscript}}
	_, err := collectPi(Ref{Kind: "path", Value: "/missing/session"}, fx.deps())
	if !errors.Is(err, errTranscriptNotFound) {
		t.Fatalf("collectPi(path) error = %v, want transcript not found without id fallback", err)
	}
}

func TestUnsafeIDCannotWidenTranscriptGlob(t *testing.T) {
	t.Parallel()
	fx := fixture{files: map[string]string{piPath("session"): piTranscript}}
	_, err := collectPi(Ref{Kind: "id", Value: "*"}, fx.deps())
	if !errors.Is(err, errNativeSession) {
		t.Fatalf("collectPi(wildcard id) error = %v, want unusable native session", err)
	}
}

// The OpenCode export proves used = input + cache.read + cache.write, that only
// completed assistant messages count, and that the latest wins.
const openCodeExport = `{"info":{"id":"ses"},"messages":[
{"info":{"role":"user","time":{"created":1}},"parts":[]},
{"info":{"role":"assistant","finish":"stop","tokens":{"total":99999,"input":50000,"output":9,"reasoning":0,"cache":{"read":0,"write":0}},"time":{"created":10,"completed":20}},"parts":[]},
{"info":{"role":"assistant","tokens":{"total":1,"input":1,"output":0,"reasoning":0,"cache":{"read":0,"write":0}},"time":{"created":30}},"parts":[]},
{"info":{"role":"assistant","finish":"stop","tokens":{"total":8756,"input":8603,"output":5,"reasoning":0,"cache":{"read":128,"write":20}},"time":{"created":40,"completed":1785625937310}},"parts":[]}
]}`

func TestCollectOpenCodeUsesInputAndCacheFromLatestCompleted(t *testing.T) {
	t.Parallel()
	fx := fixture{exports: map[string]string{"ses-oc": openCodeExport}}
	got, err := collectOpenCode(Ref{Kind: "id", Value: "ses-oc"}, fx.deps())
	if err != nil {
		t.Fatalf("collectOpenCode() error = %v", err)
	}
	// 8603 + 128 + 20 = 8751; output 5 excluded, the incomplete middle message ignored.
	if got.used != 8751 {
		t.Errorf("used = %d, want 8751 (input+cache.read+cache.write of latest completed)", got.used)
	}
	if got.hasWindow {
		t.Errorf("window should be unknown for OpenCode, got %d", got.window)
	}
	if want := time.UnixMilli(1785625937310).UTC(); !got.observedAt.Equal(want) {
		t.Errorf("observedAt = %v, want %v", got.observedAt, want)
	}
}

func TestCollectOpenCodeUnsupportedFormatOnUnfamiliarSchema(t *testing.T) {
	t.Parallel()
	fx := fixture{exports: map[string]string{"ses-oc": `{"info":{}}`}}
	_, err := collectOpenCode(Ref{Kind: "id", Value: "ses-oc"}, fx.deps())
	if !errors.Is(err, errUnsupportedFormat) {
		t.Fatalf("collectOpenCode() error = %v, want errUnsupportedFormat", err)
	}
}
