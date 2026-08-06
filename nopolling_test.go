package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Coordination in Fledge is event-driven end to end: durable events arrive
// through filesystem notifications and Herdr push events, and every wait is
// either a subscription or a bounded deadline. Nothing supervises on a clock.
//
// This test is the guard on that property. It scans every production Go file
// for the constructs a reintroduced poll would need — a sleep, a ticker, or a
// tunable interval — and fails on every occurrence.
// Bounded deadlines (time.NewTimer, context.WithTimeout, SetReadDeadline) are
// deliberately permitted: they cap how long a wait may hang, which is the
// opposite of waking up repeatedly to ask again.
//
// Test files are exempt. A test observing an inherently asynchronous
// notification has to bound its own wait, or a regression hangs the suite
// instead of failing it.

// bannedConstructs are the spellings a poll or a timed supervision loop needs.
var bannedConstructs = []string{
	"time.Sleep(",
	"time.Tick(",
	"time.NewTicker(",
	"time.AfterFunc(",
}

// bannedTimingFields are the tunables of the removed watcher configuration.
// Their reappearance in production code means timing has become a knob again.
var bannedTimingFields = []string{
	"poll_interval_seconds",
	"idle_poll_interval_seconds",
	"signal_grace_seconds",
	"heartbeat_seconds",
	"heartbeat_max_seconds",
	"wake_min_interval_seconds",
	"done_message_grace_seconds",
	"PollInterval",
	"IdlePollInterval",
	"SignalGrace",
	"HeartbeatSeconds",
	"WakeMinInterval",
	"DoneMessageGrace",
	"watch.json",
}

func TestProductionCodeNeverPollsOrSupervisesOnAClock(t *testing.T) {
	t.Parallel()

	var findings []string
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "bin", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		normalized := filepath.ToSlash(path)
		for _, line := range offendingLines(string(contents), bannedConstructs) {
			findings = append(findings, normalized+": "+line)
		}
		for _, line := range offendingLines(string(contents), bannedTimingFields) {
			findings = append(findings, normalized+": "+line)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) > 0 {
		t.Fatalf("production code reintroduced timed polling:\n  %s\n\nUse a filesystem or Herdr event instead.",
			strings.Join(findings, "\n  "))
	}
}

// offendingLines returns each source line containing a banned spelling, ignoring
// comments so prose describing the ban does not trip it.
func offendingLines(contents string, banned []string) []string {
	var found []string
	for _, line := range strings.Split(contents, "\n") {
		code := strings.TrimSpace(line)
		if strings.HasPrefix(code, "//") {
			continue
		}
		if index := strings.Index(code, "//"); index >= 0 {
			code = code[:index]
		}
		for _, construct := range banned {
			if strings.Contains(code, construct) {
				found = append(found, strings.TrimSpace(line))
				break
			}
		}
	}
	return found
}

// The guard is only worth having if it actually catches these spellings.
func TestPollingGuardDetectsEachBannedConstruct(t *testing.T) {
	t.Parallel()

	for _, construct := range append(append([]string(nil), bannedConstructs...), bannedTimingFields...) {
		source := "package sample\n\nfunc example() {\n\t" + construct + "\n}\n"
		if len(offendingLines(source, append(append([]string(nil), bannedConstructs...), bannedTimingFields...))) != 1 {
			t.Fatalf("guard missed %q", construct)
		}
	}
	// A mention inside a comment is prose, not a poll.
	if len(offendingLines("// time.Sleep(1) would be a poll\n", bannedConstructs)) != 0 {
		t.Fatal("guard flagged a comment")
	}
}
