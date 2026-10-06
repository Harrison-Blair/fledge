package board

import (
	"context"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
	"github.com/charmbracelet/x/ansi"
)

func TestDetailFullContentUnicodeAndCancellation(t *testing.T) {
	r := task.Record{ID: "11111111", Title: "界\x1b[31mtitle", Status: task.Completed, Brief: "brief", Parent: tasktest.Ptr("missing"), After: []string{"22222222", "unknown"}, Owner: tasktest.Ptr("owner"), Result: tasktest.Ptr(strings.Repeat("report 界 ", 300000)), VerificationNote: tasktest.Ptr("note"), CreatedAt: "yesterday\a", Delivery: &task.Delivery{Attempt: task.Attempt{Error: tasktest.Ptr("delivery bad")}}, CompletionNotification: &task.CompletionNotification{Attempt: task.Attempt{Error: tasktest.Ptr("notify bad")}}}
	s, err := project([]task.Record{r, {ID: "22222222", Title: "dep", Status: task.Cancelled, CancelReason: tasktest.Ptr("abandoned")}})
	if err != nil {
		t.Fatal(err)
	}
	lines := formatDetail(context.Background(), s, r.ID, "worker blocked", 32)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"brief", "awaiting verification", "abandoned", "unknown", "note", "delivery bad", "notify bad", "yesterday"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 32 {
			t.Fatalf("overflow %q", line)
		}
	}
	if len(joined) < 2000000 {
		t.Fatal("large report truncated")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := formatDetail(ctx, s, r.ID, "", 32); got != nil {
		t.Fatal("ignored cancellation")
	}
}

func TestDetailSanitizesMissingDependencyID(t *testing.T) {
	s, err := project([]task.Record{{ID: "11111111", Status: task.Created, After: []string{"bad\x1b]52;c;SECRET\a"}}})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(formatDetail(context.Background(), s, "11111111", "", 100), "\n")
	if strings.Contains(got, "\x1b") || strings.Contains(got, "SECRET") {
		t.Fatalf("unsafe dependency: %q", got)
	}
}
