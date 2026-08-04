package wake

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

var foldTime = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

func queuedEntry(id string, kind Kind, key, reason string) entry {
	return entry{Kind: entryQueued, ID: id, WakeKind: kind, Key: key, Reason: reason, Time: foldTime}
}

func deliveredEntry(id string) entry {
	return entry{Kind: entryDelivered, ID: id, MessageID: "m-1", Time: foldTime}
}

type pendingSummary struct {
	ID       string
	IDs      []string
	WakeKind Kind
	Key      string
	Reason   string
}

func summarize(records []Record) []pendingSummary {
	summaries := make([]pendingSummary, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, pendingSummary{ID: record.ID, IDs: record.IDs, WakeKind: record.WakeKind, Key: record.Key, Reason: record.Reason})
	}
	return summaries
}

func TestFoldPending(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		entries []entry
		want    []pendingSummary
	}{
		{name: "empty", entries: nil, want: []pendingSummary{}},
		{
			name:    "distinct keys keep queue order",
			entries: []entry{queuedEntry("w-1", KindStatus, "alice", "blocked"), queuedEntry("w-2", KindStatus, "bob", "failed")},
			want: []pendingSummary{
				{ID: "w-1", IDs: []string{"w-1"}, WakeKind: KindStatus, Key: "alice", Reason: "blocked"},
				{ID: "w-2", IDs: []string{"w-2"}, WakeKind: KindStatus, Key: "bob", Reason: "failed"},
			},
		},
		{
			name:    "same kind and key collapses to the latest reason and names both entries",
			entries: []entry{queuedEntry("w-1", KindStatus, "alice", "first"), queuedEntry("w-2", KindStatus, "alice", "second")},
			want:    []pendingSummary{{ID: "w-2", IDs: []string{"w-1", "w-2"}, WakeKind: KindStatus, Key: "alice", Reason: "second"}},
		},
		{
			name: "collapsed record holds the position it was first seen at",
			entries: []entry{
				queuedEntry("w-1", KindStatus, "alice", "first"),
				queuedEntry("w-2", KindEvent, "%3", "event"),
				queuedEntry("w-3", KindStatus, "alice", "second"),
			},
			want: []pendingSummary{
				{ID: "w-3", IDs: []string{"w-1", "w-3"}, WakeKind: KindStatus, Key: "alice", Reason: "second"},
				{ID: "w-2", IDs: []string{"w-2"}, WakeKind: KindEvent, Key: "%3", Reason: "event"},
			},
		},
		{
			name:    "same key under different kinds stays separate",
			entries: []entry{queuedEntry("w-1", KindStatus, "alice", "status"), queuedEntry("w-2", KindDead, "alice", "dead")},
			want: []pendingSummary{
				{ID: "w-1", IDs: []string{"w-1"}, WakeKind: KindStatus, Key: "alice", Reason: "status"},
				{ID: "w-2", IDs: []string{"w-2"}, WakeKind: KindDead, Key: "alice", Reason: "dead"},
			},
		},
		{
			name: "every heartbeat collapses regardless of key",
			entries: []entry{
				queuedEntry("w-1", KindHeartbeat, "600", "first beat"),
				queuedEntry("w-2", KindHeartbeat, "1200", "second beat"),
				queuedEntry("w-3", KindHeartbeat, "", "third beat"),
			},
			want: []pendingSummary{{ID: "w-3", IDs: []string{"w-1", "w-2", "w-3"}, WakeKind: KindHeartbeat, Key: "", Reason: "third beat"}},
		},
		{
			name:    "delivered wakes are retired",
			entries: []entry{queuedEntry("w-1", KindStatus, "alice", "first"), queuedEntry("w-2", KindDead, "bob", "gone"), deliveredEntry("w-1")},
			want:    []pendingSummary{{ID: "w-2", IDs: []string{"w-2"}, WakeKind: KindDead, Key: "bob", Reason: "gone"}},
		},
		{
			name: "a wake queued after delivery survives",
			entries: []entry{
				queuedEntry("w-1", KindStatus, "alice", "first"),
				deliveredEntry("w-1"),
				queuedEntry("w-2", KindStatus, "alice", "second"),
			},
			want: []pendingSummary{{ID: "w-2", IDs: []string{"w-2"}, WakeKind: KindStatus, Key: "alice", Reason: "second"}},
		},
		{
			name: "delivering every ID of a collapsed wake retires the whole group",
			entries: []entry{
				queuedEntry("w-1", KindStatus, "alice", "working"),
				queuedEntry("w-2", KindStatus, "alice", "blocked"),
				deliveredEntry("w-1"),
				deliveredEntry("w-2"),
			},
			want: []pendingSummary{},
		},
		{
			name: "delivering every beat of a heartbeat run retires the group",
			entries: []entry{
				queuedEntry("w-1", KindHeartbeat, "600", "first beat"),
				queuedEntry("w-2", KindHeartbeat, "1200", "second beat"),
				queuedEntry("w-3", KindHeartbeat, "2400", "third beat"),
				deliveredEntry("w-1"),
				deliveredEntry("w-2"),
				deliveredEntry("w-3"),
			},
			want: []pendingSummary{},
		},
		{
			name: "delivery retires only its own group",
			entries: []entry{
				queuedEntry("w-1", KindStatus, "alice", "working"),
				queuedEntry("w-2", KindStatus, "bob", "working"),
				queuedEntry("w-3", KindStatus, "alice", "blocked"),
				deliveredEntry("w-1"),
				deliveredEntry("w-3"),
			},
			want: []pendingSummary{{ID: "w-2", IDs: []string{"w-2"}, WakeKind: KindStatus, Key: "bob", Reason: "working"}},
		},
		{
			name: "a wake queued while a delivery was in flight survives",
			entries: []entry{
				queuedEntry("w-1", KindStatus, "alice", "working"),
				queuedEntry("w-2", KindStatus, "alice", "blocked"),
				deliveredEntry("w-1"),
				deliveredEntry("w-2"),
				queuedEntry("w-3", KindStatus, "alice", "failed"),
			},
			want: []pendingSummary{{ID: "w-3", IDs: []string{"w-3"}, WakeKind: KindStatus, Key: "alice", Reason: "failed"}},
		},
		{
			// The hazard Record.IDs exists to prevent: retiring only the survivor
			// leaves the entries it collapsed queued, and they resurface carrying
			// the stale reasons they had already superseded.
			name: "delivering only the survivor ID leaves the superseded wake queued",
			entries: []entry{
				queuedEntry("w-1", KindStatus, "alice", "working"),
				queuedEntry("w-2", KindStatus, "alice", "blocked"),
				deliveredEntry("w-2"),
			},
			want: []pendingSummary{{ID: "w-1", IDs: []string{"w-1"}, WakeKind: KindStatus, Key: "alice", Reason: "working"}},
		},
		{
			name: "delivering a collapsed duplicate leaves its replacement queued",
			entries: []entry{
				queuedEntry("w-1", KindStatus, "alice", "first"),
				queuedEntry("w-2", KindStatus, "alice", "second"),
				deliveredEntry("w-1"),
			},
			want: []pendingSummary{{ID: "w-2", IDs: []string{"w-2"}, WakeKind: KindStatus, Key: "alice", Reason: "second"}},
		},
		{
			name:    "delivery of an unknown ID changes nothing",
			entries: []entry{queuedEntry("w-1", KindStatus, "alice", "first"), deliveredEntry("w-9")},
			want:    []pendingSummary{{ID: "w-1", IDs: []string{"w-1"}, WakeKind: KindStatus, Key: "alice", Reason: "first"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := summarize(foldPending(test.entries))
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("foldPending() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestValidKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind Kind
		want bool
	}{
		{kind: KindStatus, want: true},
		{kind: KindEvent, want: true},
		{kind: KindDead, want: true},
		{kind: KindHeartbeat, want: true},
		{kind: Kind(""), want: false},
		{kind: Kind("Status"), want: false},
		{kind: Kind("wake"), want: false},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			t.Parallel()
			if got := ValidKind(test.kind); got != test.want {
				t.Fatalf("ValidKind(%q) = %v, want %v", test.kind, got, test.want)
			}
		})
	}
}

func TestDecodeEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "queued", line: `{"kind":"queued","id":"w-1","wake_kind":"status","key":"alice","reason":"blocked","time":"2026-08-04T12:00:00Z"}`, want: true},
		{name: "delivered", line: `{"kind":"delivered","id":"w-1","message_id":"m-1","time":"2026-08-04T12:00:00Z"}`, want: true},
		{name: "blank", line: "  ", want: false},
		{name: "unknown field", line: `{"kind":"queued","id":"w-1","wake_kind":"status","time":"2026-08-04T12:00:00Z","extra":1}`, want: false},
		{name: "trailing value", line: `{"kind":"queued","id":"w-1","wake_kind":"status","time":"2026-08-04T12:00:00Z"} {}`, want: false},
		{name: "unknown entry kind", line: `{"kind":"maybe","id":"w-1","time":"2026-08-04T12:00:00Z"}`, want: false},
		{name: "unknown wake kind", line: `{"kind":"queued","id":"w-1","wake_kind":"nap","time":"2026-08-04T12:00:00Z"}`, want: false},
		{name: "missing ID", line: `{"kind":"queued","wake_kind":"status","time":"2026-08-04T12:00:00Z"}`, want: false},
		{name: "missing time", line: `{"kind":"queued","id":"w-1","wake_kind":"status"}`, want: false},
		{name: "queued with message ID", line: `{"kind":"queued","id":"w-1","wake_kind":"status","message_id":"m-1","time":"2026-08-04T12:00:00Z"}`, want: false},
		{name: "delivered without message ID", line: `{"kind":"delivered","id":"w-1","time":"2026-08-04T12:00:00Z"}`, want: false},
		{name: "delivered with wake fields", line: `{"kind":"delivered","id":"w-1","wake_kind":"status","message_id":"m-1","time":"2026-08-04T12:00:00Z"}`, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var e entry
			err := decodeEntry([]byte(test.line), &e)
			if (err == nil) != test.want {
				t.Fatalf("decodeEntry(%s) error = %v, want valid %v", test.line, err, test.want)
			}
		})
	}
}

func TestValidateEntryRejectsLineBreakInID(t *testing.T) {
	t.Parallel()
	err := validateEntry(entry{Kind: entryQueued, ID: "w-1\nw-2", WakeKind: KindStatus, Time: foldTime})
	if err == nil || !strings.Contains(err.Error(), "line break") {
		t.Fatalf("validateEntry() error = %v, want a line break rejection", err)
	}
}
