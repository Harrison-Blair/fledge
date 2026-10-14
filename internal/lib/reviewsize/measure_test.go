package reviewsize

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/testutil/gittest"
)

// Entries captured with git diff --numstat --no-renames <merge>^1 <merge> for
// the worked rows of reference/plan/review-units.md "Domains".
func TestSummarizeWorkedRows(t *testing.T) {
	for _, tc := range []struct {
		merge   string
		changes []FileChange
		want    Diff
	}{
		{"621a7af", []FileChange{
			{"cmd/agent/adopt/adopt.go", 1, 1},
			{"cmd/agent/spawn/spawn.go", 1, 1},
			{"cmd/agent_integration_test.go", 2, 2},
			{"cmd/profiles_integration_test.go", 5, 1},
			{"internal/agent/spawn/daemon.go", 28, 0},
			{"internal/agent/spawn/daemon_test.go", 54, 0},
			{"internal/agent/spawn/doc.go", 1, 0},
			{"internal/agent/spawn/options.go", 1, 0},
			{"internal/agent/spawn/options_test.go", 1, 1},
			{"internal/agent/spawn/permissions_test.go", 14, 7},
			{"internal/agent/spawn/profile_test.go", 1, 1},
			{"reference/dogfood/friction.md", 68, 5},
			{"reference/herdr/environment.md", 1, 0},
			{"reference/usage/agents.md", 8, 2},
		}, Diff{SourceLines: 34, Files: 14, CodeDomains: []string{"agent", "cmd root"}, DocAreas: []string{"reference/dogfood", "reference/herdr", "reference/usage"}, TotalLines: 207}},
		{"8f6e97a", []FileChange{
			{"internal/lib/repoconfig/repoconfig.go", 58, 3},
			{"internal/lib/repoconfig/repoconfig_test.go", 48, 1},
			{"reference/usage/worktrees.md", 15, 0},
		}, Diff{SourceLines: 61, Files: 3, CodeDomains: []string{"lib/repoconfig"}, DocAreas: []string{"reference/usage"}, TotalLines: 125}},
		{"44b4713", []FileChange{
			{"internal/agent/message/fanout.go", 3, 13},
			{"internal/agent/stop/multi.go", 7, 19},
			{"internal/agent/wait/wait.go", 5, 14},
			{"internal/lib/agent/doc.go", 1, 0},
			{"internal/lib/agent/fanout.go", 31, 0},
			{"internal/lib/agent/fanout_test.go", 24, 0},
		}, Diff{SourceLines: 93, Files: 6, CodeDomains: []string{"agent", "lib/agent"}, DocAreas: []string{}, TotalLines: 117}},
	} {
		if got := Summarize(tc.changes); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %+v, want %+v", tc.merge, got, tc.want)
		}
	}
}

func TestMeasureDiffsFromMergeBase(t *testing.T) {
	dir := gittest.Repository(t)
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main\n")
	gittest.Git(t, dir, "add", ".")
	gittest.Git(t, dir, "commit", "-qm", "base")
	gittest.Git(t, dir, "branch", "-q", "feature")
	// A change on the base branch after the fork is outside base...head.
	write("other.go", "package main\n\nvar x = 1\n")
	gittest.Git(t, dir, "add", ".")
	gittest.Git(t, dir, "commit", "-qm", "base moves")
	base := strings.TrimSpace(gittest.Git(t, dir, "rev-parse", "HEAD"))
	gittest.Git(t, dir, "checkout", "-q", "feature")
	write("main.go", "package main\n\nfunc main() {}\n")
	write("internal/lib/foo/foo_test.go", "package foo\n")
	write("internal/lib/foo/logo.bin", "\x00\x01\x02")
	write("docs.md", "a\nb\n")
	gittest.Git(t, dir, "add", ".")
	gittest.Git(t, dir, "commit", "-qm", "feature")

	got, err := Measure(context.Background(), dir, base, "feature")
	if err != nil {
		t.Fatal(err)
	}
	want := Diff{SourceLines: 2, Files: 4, CodeDomains: []string{"lib/foo", "module root"}, DocAreas: []string{}, TotalLines: 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v, want %+v", got, want)
	}
}
