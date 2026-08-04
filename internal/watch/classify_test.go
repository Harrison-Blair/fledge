package watch

import (
	"strconv"
	"strings"
	"testing"
)

func TestParseStatusLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		line       string
		wantVerb   string
		wantDetail string
		wantOK     bool
	}{
		{name: "verb and detail", line: "blocked: waiting on approval", wantVerb: "blocked", wantDetail: "waiting on approval", wantOK: true},
		{name: "surrounding whitespace", line: "   working: refactoring pass 2   ", wantVerb: "working", wantDetail: "refactoring pass 2", wantOK: true},
		{name: "empty detail", line: "done:", wantVerb: "done", wantDetail: "", wantOK: true},
		{name: "detail keeps inner colons", line: "failed: build: exit 1", wantVerb: "failed", wantDetail: "build: exit 1", wantOK: true},
		{name: "unknown verb still parses", line: "musing: about nothing", wantVerb: "musing", wantDetail: "about nothing", wantOK: true},
		{name: "verb not leading", line: "I am working: yes", wantOK: false},
		{name: "no colon", line: "done", wantOK: false},
		{name: "indented without colon", line: "  done", wantOK: false},
		{name: "missing verb", line: ": waiting on approval", wantOK: false},
		{name: "blank line", line: "   ", wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			verb, detail, ok := ParseStatusLine(test.line)
			if ok != test.wantOK {
				t.Fatalf("ParseStatusLine(%q) ok = %t, want %t", test.line, ok, test.wantOK)
			}
			if !test.wantOK {
				return
			}
			if verb != test.wantVerb {
				t.Errorf("ParseStatusLine(%q) verb = %q, want %q", test.line, verb, test.wantVerb)
			}
			if detail != test.wantDetail {
				t.Errorf("ParseStatusLine(%q) detail = %q, want %q", test.line, detail, test.wantDetail)
			}
		})
	}
}

func TestClassifyStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		verb string
		want Action
	}{
		{verb: "blocked", want: ActionWake},
		{verb: "needs-decision", want: ActionWake},
		{verb: "failed", want: ActionWake},
		{verb: "working", want: ActionAbsorb},
		{verb: "paused", want: ActionAbsorb},
		{verb: "done", want: ActionWakeAfterGrace},
		{verb: "idle", want: ActionIgnore},
		{verb: "Blocked", want: ActionIgnore},
		{verb: "", want: ActionIgnore},
	}

	for _, test := range tests {
		t.Run(test.verb, func(t *testing.T) {
			t.Parallel()

			if got := ClassifyStatus(test.verb); got != test.want {
				t.Errorf("ClassifyStatus(%q) = %v, want %v", test.verb, got, test.want)
			}
		})
	}
}

func TestClassifyTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status string
		want   TransitionAction
	}{
		{status: "blocked", want: TransitionWake},
		{status: "working", want: TransitionClear},
		{status: "idle", want: TransitionIgnore},
		{status: "done", want: TransitionIgnore},
		{status: "unknown", want: TransitionIgnore},
		{status: "", want: TransitionIgnore},
	}

	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			t.Parallel()

			if got := ClassifyTransition(test.status); got != test.want {
				t.Errorf("ClassifyTransition(%q) = %v, want %v", test.status, got, test.want)
			}
		})
	}
}

func TestComposeWakeBodyRendersEveryReason(t *testing.T) {
	t.Parallel()

	body := ComposeWakeBody([]string{"reviewer blocked: permission dialog", "migrator failed: exit 1"})
	want := strings.Join([]string{
		"Watcher: 2 worker events need attention:",
		"- reviewer blocked: permission dialog",
		"- migrator failed: exit 1",
		"Check each worker (fledge agent message send <name> <text>) or inspect its pane.",
		"Automated watcher notification — do not reply to this message ID.",
	}, "\n")
	if body != want {
		t.Errorf("ComposeWakeBody() =\n%s\nwant\n%s", body, want)
	}
}

func TestComposeWakeBodyUsesSingularHeader(t *testing.T) {
	t.Parallel()

	body := ComposeWakeBody([]string{"reviewer blocked: permission dialog"})
	if got, want := firstLine(body), "Watcher: 1 worker event needs attention:"; got != want {
		t.Errorf("ComposeWakeBody() header = %q, want %q", got, want)
	}
}

func TestComposeWakeBodyWithoutReasons(t *testing.T) {
	t.Parallel()

	if body := ComposeWakeBody(nil); body != "" {
		t.Errorf("ComposeWakeBody(nil) = %q, want empty", body)
	}
}

func TestComposeWakeBodyCapsReasonCount(t *testing.T) {
	t.Parallel()

	reasons := make([]string, 25)
	for i := range reasons {
		reasons[i] = "worker blocked"
	}

	body := ComposeWakeBody(reasons)
	if got, want := strings.Count(body, "\n- "), 20; got != want {
		t.Errorf("ComposeWakeBody() rendered %d reasons, want %d", got, want)
	}
	if got, want := firstLine(body), "Watcher: 25 worker events need attention:"; got != want {
		t.Errorf("ComposeWakeBody() header = %q, want %q", got, want)
	}
	if !strings.Contains(body, "\n+5 more in the watch ledger\n") {
		t.Errorf("ComposeWakeBody() = %q, want a truncation notice for 5 reasons", body)
	}
}

func TestComposeWakeBodyCapsByteLength(t *testing.T) {
	t.Parallel()

	reasons := make([]string, 6)
	for i := range reasons {
		reasons[i] = strings.Repeat("x", 1000)
	}

	body := ComposeWakeBody(reasons)
	if len(body) > 4096 {
		t.Errorf("ComposeWakeBody() body is %d bytes, want at most 4096", len(body))
	}
	rendered := strings.Count(body, "\n- ")
	if rendered == 0 || rendered == len(reasons) {
		t.Fatalf("ComposeWakeBody() rendered %d reasons, want a partial batch", rendered)
	}
	if !strings.Contains(body, "\n+"+strconv.Itoa(len(reasons)-rendered)+" more in the watch ledger\n") {
		t.Errorf("ComposeWakeBody() = %q, want a truncation notice for %d reasons", body, len(reasons)-rendered)
	}
}

func TestComposeWakeBodyDropsAnOversizeReason(t *testing.T) {
	t.Parallel()

	body := ComposeWakeBody([]string{strings.Repeat("x", 5000)})
	if len(body) > maxWakeBodyBytes {
		t.Errorf("ComposeWakeBody() body is %d bytes, want at most %d", len(body), maxWakeBodyBytes)
	}
	if got := strings.Count(body, "\n- "); got != 0 {
		t.Errorf("ComposeWakeBody() rendered %d reasons, want the oversize reason dropped", got)
	}
	if !strings.Contains(body, "\n+1 more in the watch ledger\n") {
		t.Errorf("ComposeWakeBody() = %q, want a truncation notice", body)
	}
	if !strings.Contains(body, "\nCheck each worker") {
		t.Errorf("ComposeWakeBody() = %q, want the triage footer", body)
	}
	if !strings.HasSuffix(body, "\nAutomated watcher notification — do not reply to this message ID.") {
		t.Errorf("ComposeWakeBody() = %q, want the do-not-reply footer", body)
	}
}

func TestComposeWakeBodyAlwaysEndsWithTheFooter(t *testing.T) {
	t.Parallel()

	for _, count := range []int{1, 3, 25} {
		reasons := make([]string, count)
		for i := range reasons {
			reasons[i] = "worker blocked"
		}

		body := ComposeWakeBody(reasons)
		if !strings.Contains(body, "\nCheck each worker (fledge agent message send <name> <text>) or inspect its pane.\n") {
			t.Errorf("ComposeWakeBody() with %d reasons = %q, want the triage footer", count, body)
		}
		if !strings.HasSuffix(body, "\nAutomated watcher notification — do not reply to this message ID.") {
			t.Errorf("ComposeWakeBody() with %d reasons = %q, want the do-not-reply footer", count, body)
		}
	}
}

func firstLine(body string) string {
	line, _, _ := strings.Cut(body, "\n")
	return line
}
