package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func TestWorktreeHelp(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"worktree", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"list", "remove", "create"} {
		if !strings.Contains(out.String(), "\n  "+name+" ") {
			t.Fatalf("%s: %s", name, out.String())
		}
		var sub bytes.Buffer
		if err := ExecuteWithArgs([]string{"worktree", name, "--help"}, &sub); err != nil || !strings.Contains(sub.String(), "--json") {
			t.Fatalf("%s: %v %s", name, err, sub.String())
		}
	}
}

func TestWorktreeJSONValidation(t *testing.T) {
	for _, tc := range []struct {
		op   string
		args []string
	}{
		{"worktree.list", []string{"worktree", "list", "extra", "--json"}},
		{"worktree.list", []string{"worktree", "list", "--bad", "--json"}},
		{"worktree.remove", []string{"worktree", "remove", "--json"}},
		{"worktree.remove", []string{"worktree", "remove", "--path", "/p", "--branch", "b", "--json"}},
		{"worktree.remove", []string{"worktree", "remove", "extra", "--json"}},
		{"worktree.create", []string{"worktree", "create", "--json"}},
		{"worktree.create", []string{"worktree", "create", "--branch", "b", "extra", "--json"}},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var out bytes.Buffer
			err := ExecuteWithArgs(tc.args, &out)
			var status interface{ ExitCode() int }
			if !errors.As(err, &status) || status.ExitCode() != 2 {
				t.Fatalf("wrong exit: %v", err)
			}
			var envelope libagent.Outcome
			if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("not one JSON object: %q: %v", out.String(), err)
			}
			if envelope.Status != "rejected" || envelope.Operation != tc.op {
				t.Fatal(out.String())
			}
		})
	}
}
