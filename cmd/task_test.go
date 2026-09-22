package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func TestTaskHelp(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"task", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"create", "depend", "assign", "complete", "verify", "cancel", "list", "get"} {
		if !strings.Contains(out.String(), "\n  "+name+" ") {
			t.Fatalf("%s: %s", name, out.String())
		}
		var sub bytes.Buffer
		if err := ExecuteWithArgs([]string{"task", name, "--help"}, &sub); err != nil || !strings.Contains(sub.String(), "Usage:") || !strings.Contains(sub.String(), "--json") {
			t.Fatalf("%s: %v %s", name, err, sub.String())
		}
	}
	if !strings.Contains(out.String(), "never derived from Herdr") {
		t.Fatal(out.String())
	}
}

func TestTaskJSONValidation(t *testing.T) {
	for _, args := range [][]string{
		{"task", "create", "--body", "b", "--json"},
		{"task", "create", "--title", "t", "--json"},
		{"task", "create", "extra", "--json"},
		{"task", "assign", "--id", "0123abcd", "--json"},
		{"task", "assign", "--id", "0123abcd", "--name", "a", "--agent-id", "0000beef", "--json"},
		{"task", "complete", "--id", "0123abcd", "--json"},
		{"task", "verify", "--id", "zz", "--json"},
		{"task", "verify", "--id", "0123abcd", "--bad", "--json"},
		{"task", "cancel", "--json"},
		{"task", "list", "--status", "done", "--json"},
		{"task", "list", "--parent", "goal", "--json"},
		{"task", "create", "--title", "t", "--body", "b", "--parent", "goal", "--json"},
		{"task", "get", "--json"},
		{"task", "depend", "--id", "0123abcd", "--json"},
		{"task", "depend", "--id", "0123abcd", "--after", "nope", "--json"},
		{"task", "create", "--title", "t", "--body", "b", "--after", "nope", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			err := ExecuteWithArgs(args, &out)
			var status interface{ ExitCode() int }
			if !errors.As(err, &status) || status.ExitCode() != 2 {
				t.Fatalf("wrong exit: %v", err)
			}
			var envelope libagent.Outcome
			if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("not one JSON object: %q: %v", out.String(), err)
			}
			if envelope.Status != "rejected" || !strings.HasPrefix(envelope.Operation, "task.") {
				t.Fatal(out.String())
			}
		})
	}
}
