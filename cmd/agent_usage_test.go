package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func TestAgentUsageRegistered(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "--help"}, &out); err != nil || !strings.Contains(out.String(), "\n  usage ") {
		t.Fatalf("%v %s", err, out.String())
	}
	out.Reset()
	if err := ExecuteWithArgs([]string{"agent", "usage", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--name", "--pane", "--id", "--mine", "--parent", "--state", "--harness", "--profile", "--task", "--worktree", "--registered", "--json"} {
		if !strings.Contains(out.String(), flag+" ") {
			t.Errorf("missing %s: %s", flag, out.String())
		}
	}
}

func TestAgentUsageJSONValidation(t *testing.T) {
	for _, args := range [][]string{
		{"agent", "usage", "--json"},
		{"agent", "usage", "--name", "a", "--state", "idle", "--json"},
		{"agent", "usage", "--id", "BEEF", "--json"},
		{"agent", "usage", "--state", "asleep", "--json"},
		{"agent", "usage", "extra", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			err := ExecuteWithArgs(args, &out)
			var status interface{ ExitCode() int }
			if !errors.As(err, &status) || status.ExitCode() != 2 {
				t.Fatalf("wrong exit: %v", err)
			}
			var envelope libagent.Outcome
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Status != "rejected" || envelope.Operation != "agent.usage" {
				t.Fatalf("%q: %v", out.String(), err)
			}
		})
	}
}
