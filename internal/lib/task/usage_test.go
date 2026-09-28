package task

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/usage"
)

const (
	from = "2026-09-23T10:00:00Z"
	to   = "2026-09-23T11:02:00.5Z"
)

// fakeReader records its call and answers with summary.
type fakeReader struct {
	kind   string
	ref    usage.Ref
	window usage.Window
	calls  int
	answer usage.Summary
}

func (f *fakeReader) read(_ context.Context, kind string, ref usage.Ref, w usage.Window) usage.Summary {
	f.calls++
	f.kind, f.ref, f.window = kind, ref, w
	return f.answer
}

func session(kind, value string) *herdr.AgentSession {
	return &herdr.AgentSession{Kind: &kind, Value: &value}
}

func TestCollectUsageReadsTheLiveSessionInsideTheWindow(t *testing.T) {
	harness, fg := "claude", "/work"
	cost := &usage.Cost{Amount: 0.5, Currency: "USD", Basis: "estimate", Source: "pi price table"}
	f := &fakeReader{answer: usage.Summary{Models: []string{"m1"}, Turns: 14, Tokens: usage.Tokens{Input: 1200, Output: 18400}, Cost: cost, Basis: usage.Measured}}
	rec := &identity.Record{ID: "0000aaaa", NativeSession: &identity.NativeSessionRef{Kind: "id", Value: "stale", Harness: "claude"}}
	live := &herdr.AgentDetails{Pane: herdr.Pane{Agent: &harness}, ForegroundCwd: &fg, AgentSession: session("path", "/s.jsonl")}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	got := CollectUsage(context.Background(), f.read, rec, live, "/repo", from, to, now)

	wantFrom, _ := time.Parse(time.RFC3339Nano, from)
	wantTo, _ := time.Parse(time.RFC3339Nano, to)
	if f.kind != "claude" || f.ref != (usage.Ref{Kind: "path", Value: "/s.jsonl", Cwd: "/work"}) || !f.window.From.Equal(wantFrom) || !f.window.To.Equal(wantTo) {
		t.Fatalf("reader got %q %+v %v..%v", f.kind, f.ref, f.window.From, f.window.To)
	}
	want := &UsageSnapshot{AgentID: &rec.ID, Harness: &harness, Session: &UsageSession{Kind: "path", Value: "/s.jsonl"}, Window: UsageWindow{From: from, To: to},
		ElapsedSeconds: 3720, Tokens: usage.Tokens{Input: 1200, Output: 18400}, Cost: cost, Models: []string{"m1"}, Turns: 14, Basis: usage.Measured, CollectedAt: "2026-09-23T12:00:00Z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	const wantJSON = `{"agent_id":"0000aaaa","harness":"claude","session":{"kind":"path","value":"/s.jsonl"},"window":{"from":"2026-09-23T10:00:00Z","to":"2026-09-23T11:02:00.5Z"},"elapsed_seconds":3720,"tokens":{"input":1200,"output":18400,"cache_read":0,"cache_write":0,"reasoning":0},"cost":{"amount":0.5,"currency":"USD","basis":"estimate","source":"pi price table"},"models":["m1"],"turns":14,"basis":"measured","reason":null,"collected_at":"2026-09-23T12:00:00Z"}`
	if string(b) != wantJSON {
		t.Fatalf("%s", b)
	}
}

// Without a live agent the persisted ref, record harness, and worktree are used.
func TestCollectUsageFallsBackToTheRecord(t *testing.T) {
	harness, tree := "codex", "/tree"
	f := &fakeReader{answer: usage.Summary{Basis: usage.Measured, Reason: "fallback to totals"}}
	rec := &identity.Record{ID: "0000aaaa", Harness: &harness, WorktreePath: &tree, NativeSession: &identity.NativeSessionRef{Kind: "id", Value: "abc"}}

	got := CollectUsage(context.Background(), f.read, rec, nil, "/repo", from, to, time.Now(), "window starts at created_at")

	if f.kind != "codex" || f.ref != (usage.Ref{Kind: "id", Value: "abc", Cwd: "/tree"}) {
		t.Fatalf("reader got %q %+v", f.kind, f.ref)
	}
	if got.Basis != usage.Measured || got.Reason == nil || *got.Reason != "window starts at created_at; fallback to totals" {
		t.Fatalf("%+v %v", got, got.Reason)
	}
}

func TestCollectUsageWithoutARefOrAgentIsUnavailable(t *testing.T) {
	f := &fakeReader{}
	rec := &identity.Record{ID: "0000aaaa"}
	got := CollectUsage(context.Background(), f.read, rec, &herdr.AgentDetails{}, "/repo", from, to, time.Now())
	if f.calls != 0 || got.Basis != usage.Unavailable || got.Reason == nil || *got.Reason != "no native session ref observed" || got.ElapsedSeconds != 3720 || *got.AgentID != rec.ID {
		t.Fatalf("calls=%d %+v", f.calls, got)
	}
	got = CollectUsage(context.Background(), f.read, nil, nil, "/repo", from, to, time.Now(), "the verifier is not a registered agent")
	if f.calls != 0 || got.AgentID != nil || got.Basis != usage.Unavailable || *got.Reason != "the verifier is not a registered agent" {
		t.Fatalf("calls=%d %+v", f.calls, got)
	}
}

func TestObserveStoresTheLiveSessionBestEffort(t *testing.T) {
	rec := identity.Record{ID: "0000aaaa"}
	stored := identity.Record{ID: "0000aaaa", NativeSession: &identity.NativeSessionRef{Value: "s1"}}
	now := time.Date(2026, 9, 24, 5, 0, 0, 0, time.UTC)
	var seen string
	ok := func(_ *state.Store, id string, s herdr.AgentSession, at time.Time) (identity.Record, bool, error) {
		seen = id + " " + *s.Value + " " + at.Format(time.RFC3339)
		return stored, true, nil
	}
	out := libagent.Outcome{}
	if got := Observe(nil, ok, rec, &herdr.AgentDetails{AgentSession: session("id", "s1")}, now, &out); !reflect.DeepEqual(got, stored) || seen != "0000aaaa s1 2026-09-24T05:00:00Z" ||
		!reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "updated", Kind: "native_session", ID: "0000aaaa"}}) {
		t.Fatalf("%+v %q %+v", got, seen, out.Effects)
	}
	failing := func(*state.Store, string, herdr.AgentSession, time.Time) (identity.Record, bool, error) {
		return identity.Record{}, false, errors.New("read-only")
	}
	out = libagent.Outcome{Status: "success"}
	if got := Observe(nil, failing, rec, &herdr.AgentDetails{AgentSession: session("id", "s1")}, now, &out); !reflect.DeepEqual(got, rec) || out.Error != nil ||
		!reflect.DeepEqual(out.Effects, []libagent.Effect{{Action: "warning", Kind: "native_session", ID: "0000aaaa"}}) {
		t.Fatalf("%+v %+v", got, out)
	}
	unchanged := func(*state.Store, string, herdr.AgentSession, time.Time) (identity.Record, bool, error) {
		return stored, false, nil
	}
	out = libagent.Outcome{}
	if got := Observe(nil, unchanged, rec, &herdr.AgentDetails{AgentSession: session("id", "s1")}, now, &out); !reflect.DeepEqual(got, stored) || len(out.Effects) != 0 {
		t.Fatalf("%+v %+v", got, out)
	}
	for _, live := range []*herdr.AgentDetails{nil, {}} {
		out = libagent.Outcome{}
		if got := Observe(nil, failing, rec, live, now, &out); !reflect.DeepEqual(got, rec) || len(out.Effects) != 0 {
			t.Fatalf("%+v %+v", got, out)
		}
	}
}

func TestRecordWithoutUsageUnmarshalsNil(t *testing.T) {
	var r Record
	if err := json.Unmarshal([]byte(`{"id":"0000aaaa","status":"completed"}`), &r); err != nil || r.Usage != nil {
		t.Fatalf("%+v %v", r.Usage, err)
	}
}
