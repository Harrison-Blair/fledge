package agent

import (
	"slices"
	"testing"
)

func TestStatusMembership(t *testing.T) {
	for _, s := range []string{"idle", "working", "blocked", "done", "unknown"} {
		if !IsStatus(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"", "nope", "Idle", "stopped"} {
		if IsStatus(s) {
			t.Fatal(s)
		}
	}
	settled := SettledStatuses()
	if !slices.Equal(settled, []string{"idle", "done", "blocked"}) {
		t.Fatalf("%q", settled)
	}
	for _, s := range settled {
		if !IsStatus(s) {
			t.Fatal(s)
		}
	}
	settled[0] = "mutated"
	if SettledStatuses()[0] != "idle" {
		t.Fatal("SettledStatuses leaked its backing array")
	}
}
