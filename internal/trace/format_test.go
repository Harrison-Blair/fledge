package trace

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func local(second int) time.Time {
	return time.Date(2026, 8, 5, 20, 41, second, 0, time.Local)
}

func TestHumanRendersAlignedColumns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record Record
		want   string
	}{
		{
			name: "message shows its body",
			record: Record{At: local(9), Kind: "message", Origin: "coord", Target: "impl-worker",
				Ref: "m-9f2c", Pane: "%12", Body: "Please rerun the build"},
			want: "20:41:09 message       coord -> impl-worker      m-9f2c  Please rerun the build",
		},
		{
			name: "a queued wake shows how the dispatcher will route it",
			record: Record{At: local(9), Kind: "wake.queued", Origin: "ledger", Target: "impl-worker",
				Ref: "w-31a4", Rel: "m-9f2c", Pane: "%12", Note: "kind=message ref=m-9f2c"},
			want: "20:41:09 wake.queued   ledger -> impl-worker     w-31a4  kind=message ref=m-9f2c pane=%12",
		},
		{
			name:   "a pane event has no target and no reference",
			record: Record{At: local(9), Kind: "herdr.pane", Origin: "impl-worker", Pane: "%12", Status: "idle"},
			want:   "20:41:09 herdr.pane    impl-worker                       pane=%12 status=idle",
		},
		{
			name:   "a durable ID is shortened to stay in its column",
			record: Record{At: local(9), Kind: "task.progress", Origin: "impl-worker", Ref: "t-f5aecb1d445d6362e8b9ba1d75789072", Body: "green"},
			want:   "20:41:09 task.progress impl-worker               t-f5ae  green",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Human(test.record, false); got != test.want {
				t.Fatalf("Human() =\n%q\nwant\n%q", got, test.want)
			}
		})
	}
}

// A pasted stack trace or a multi-line report must stay one line, and stay
// short enough to read next to everything else happening in the session.
func TestHumanCollapsesAndTruncatesTheBody(t *testing.T) {
	t.Parallel()

	line := Human(Record{At: local(9), Kind: "message", Origin: "a", Target: "b", Body: "line one\n\tline  two"}, false)
	if !strings.HasSuffix(line, "line one line two") {
		t.Fatalf("collapsed line = %q", line)
	}
	long := strings.Repeat("é", 150)
	line = Human(Record{At: local(9), Kind: "message", Origin: "a", Target: "b", Body: long}, false)
	body := line[strings.LastIndex(line, "  ")+2:]
	if utf8.RuneCountInString(body) != 101 || !strings.HasSuffix(body, "…") {
		t.Fatalf("truncated body = %q (%d runes)", body, utf8.RuneCountInString(body))
	}
}

func TestHumanColorsOnlyTheKindColumn(t *testing.T) {
	t.Parallel()

	plain := Human(Record{At: local(9), Kind: "message", Origin: "coord", Target: "worker", Body: "hi"}, false)
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("uncolored render = %q", plain)
	}
	for kind, code := range map[string]string{
		"message":         "\x1b[36m",
		"reply":           "\x1b[36m",
		"task.completed":  "\x1b[35m",
		"wake.queued":     "\x1b[33m",
		"agent.status":    "\x1b[2m",
		"herdr.pane":      "\x1b[2m",
		"wake.failed":     "\x1b[31m",
		"dispatcher.exit": "\x1b[31m",
	} {
		line := Human(Record{At: local(9), Kind: kind, Origin: "coord", Target: "worker", Body: "hi"}, true)
		if !strings.Contains(line, code+kind) {
			t.Fatalf("Human(%s) = %q, want kind colored with %q", kind, line, code)
		}
		if !strings.HasSuffix(line, "hi") || strings.Count(line, "\x1b[0m") != 1 {
			t.Fatalf("Human(%s) = %q, want color confined to the kind column", kind, line)
		}
	}
}

func TestHumanSanitizesControlCharacters(t *testing.T) {
	t.Parallel()

	hostile := "start\x1bbare \x1b[2Jclear mid\x1b[31mred\x07 bell\r end"
	plain := Human(Record{At: local(9), Kind: "message", Origin: "coord", Target: "worker", Body: hostile}, false)
	assertNoC0(t, plain)

	colored := Human(Record{At: local(9), Kind: "message", Origin: "coord", Target: "worker", Body: hostile}, true)
	if strings.Count(colored, "\x1b") != 2 || !strings.Contains(colored, cyan+"message") ||
		strings.Count(colored, reset) != 1 {
		t.Fatalf("colored hostile render = %q, want color confined to the kind column", colored)
	}
	withoutKindColor := strings.Replace(colored, cyan, "", 1)
	withoutKindColor = strings.Replace(withoutKindColor, reset, "", 1)
	assertNoC0(t, withoutKindColor)

	metadata := Human(Record{At: local(9), Kind: "wake.failed", Origin: "dispatcher",
		Note: "bad\x1b[2J\x07", Pane: "%12\x1b", Status: "failed\rnow"}, false)
	assertNoC0(t, metadata)
}

func TestHumanSanitizesBeforeTruncating(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", bodyRunes-1) + "\x1b[31m" + strings.Repeat("z", bodyRunes)
	line := Human(Record{At: local(9), Kind: "message", Origin: "a", Target: "b", Body: body}, false)
	assertNoC0(t, line)
	if want := strings.Repeat("x", bodyRunes-1) + "[…"; !strings.HasSuffix(line, want) {
		t.Fatalf("truncated sanitized line = %q, want suffix %q", line, want)
	}
}

func assertNoC0(t *testing.T, value string) {
	t.Helper()
	for _, r := range value {
		if r < ' ' {
			t.Fatalf("render contains C0 control U+%04X: %q", r, value)
		}
	}
}

// The stored line is the machine-readable form: full body, full timestamp.
func TestJSONKeepsTheFullRecordAndDecodesBack(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", 500)
	record := Record{At: local(9), Kind: "message", Origin: "coord", Target: "worker",
		Actor: "coord", Pane: "%12", Ref: "m-9f2c", Rel: "m-0001", Status: "active", Body: body, Note: "n"}
	line, err := JSON(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(line), "\n") {
		t.Fatalf("JSON line contains a newline: %q", line)
	}
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["body"] != body {
		t.Fatalf("stored body was not kept whole: %v", raw["body"])
	}
	if _, err := time.Parse(time.RFC3339, raw["at"].(string)); err != nil {
		t.Fatalf("at = %v, want RFC3339: %v", raw["at"], err)
	}
	decoded, ok := Decode(line)
	if !ok || decoded.Kind != record.Kind || decoded.Body != record.Body || !decoded.At.Equal(record.At) {
		t.Fatalf("Decode() = %#v, %v", decoded, ok)
	}
}

// A log written by an older Fledge holds plain text, which the follower has to
// pass through rather than discard.
func TestDecodeRejectsLinesThatAreNotRecords(t *testing.T) {
	t.Parallel()

	for _, line := range []string{"dispatcher ready", "", "{}", `{"kind":""}`, "{not json"} {
		if _, ok := Decode([]byte(line)); ok {
			t.Fatalf("Decode(%q) accepted a non-record line", line)
		}
	}
}
