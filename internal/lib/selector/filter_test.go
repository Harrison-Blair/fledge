package selector

import (
	"errors"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func TestFilterValidateRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    Filter
		flag string
	}{
		{"mine with parent", Filter{Mine: true, Parent: "0000beef"}, "--mine"},
		{"malformed parent", Filter{Parent: "BEEF"}, "--parent"},
		{"malformed task", Filter{Tasks: []string{"0000beef", "nope"}}, "--task"},
		{"unknown state", Filter{States: []string{"idle", "sleeping"}}, "--state"},
		{"unknown harness", Filter{Harnesses: []string{"claude", "vim"}}, "--harness"},
		{"empty profile", Filter{Profiles: []string{""}}, "--profile"},
		{"empty worktree", Filter{Worktrees: []string{" "}}, "--worktree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.f.Validate()
			var input *libagent.InputError
			if !errors.As(err, &input) || !strings.Contains(err.Error(), tc.flag) {
				t.Fatalf("got %v, want an input error naming %s", err, tc.flag)
			}
		})
	}
}

func TestFilterValidateAccepts(t *testing.T) {
	f := Filter{Parent: "0000beef", States: []string{"idle", "working", "blocked", "done", "unknown"}, Harnesses: []string{"claude", "codex"},
		Profiles: []string{"reviewer"}, Tasks: []string{"0000cafe"}, Worktrees: []string{"/repo"}, Registered: true}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Filter{Mine: true}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFilterEmptyAndNeedsRecords(t *testing.T) {
	for _, tc := range []struct {
		f            Filter
		empty, needs bool
	}{
		{Filter{}, true, false},
		{Filter{States: []string{"idle"}}, false, false},
		{Filter{Harnesses: []string{"claude"}}, false, false},
		{Filter{Mine: true}, false, true},
		{Filter{Parent: "0000beef"}, false, true},
		{Filter{Profiles: []string{"p"}}, false, true},
		{Filter{Tasks: []string{"0000beef"}}, false, true},
		{Filter{Worktrees: []string{"/w"}}, false, true},
		{Filter{Registered: true}, false, true},
	} {
		if tc.f.Empty() != tc.empty || tc.f.NeedsRecords() != tc.needs {
			t.Fatalf("%+v: Empty=%v NeedsRecords=%v", tc.f, tc.f.Empty(), tc.f.NeedsRecords())
		}
	}
}
