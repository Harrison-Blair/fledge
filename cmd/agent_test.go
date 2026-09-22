package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func TestAgentHelp(t *testing.T) {
	for _, args := range [][]string{{"agent", "--help"}, {"agent", "spawn", "--help"}, {"agent", "list", "--help"}, {"agent", "message", "--help"}, {"agent", "models", "--help"}, {"agent", "stop", "--help"}, {"agent", "get", "--help"}, {"agent", "read", "--help"}, {"agent", "wait", "--help"}, {"agent", "adopt", "--help"}, {"agent", "current", "--help"}, {"agent", "send", "--help"}} {
		var out bytes.Buffer
		if err := ExecuteWithArgs(args, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Fatal(out.String())
		}
	}
}
func TestAgentJSONValidation(t *testing.T) {
	for _, args := range [][]string{
		{"agent", "spawn", "--json"},
		{"agent", "spawn", "--bad", "--json"},
		{"agent", "spawn", "positional", "--json"},
		{"agent", "message", "--name", "a", "--body", "", "--json"},
		{"agent", "models", "--harness", "nope", "--json"},
		{"agent", "models", "extra", "--json"},
		{"agent", "get", "--json"},
		{"agent", "get", "--name=", "--json"},
		{"agent", "get", "--pane=", "--json"},
		{"agent", "get", "--name", "a", "--pane", "p", "--json"},
		{"agent", "get", "extra", "--json"},
		{"agent", "stop", "--json"},
		{"agent", "stop", "--name", "a", "--pane", "p", "--json"},
		{"agent", "stop", "extra", "--json"},
		{"agent", "read", "--json"},
		{"agent", "read", "--name", "a", "--source", "recent_unwrapped", "--json"},
		{"agent", "read", "--name", "a", "--lines", "-1", "--json"},
		{"agent", "read", "--name", "a", "--lines", "4294967296", "--json"},
		{"agent", "read", "extra", "--json"},
		{"agent", "wait", "--json"},
		{"agent", "wait", "--name", "a", "--name", "b", "--json"},
		{"agent", "wait", "--name", "a", "--pane", "a", "--any", "--json"},
		{"agent", "wait", "--name", "a", "--until", "settled", "--json"},
		{"agent", "wait", "--name", "a", "--timeout", "-1s", "--json"},
		{"agent", "wait", "extra", "--json"},
		{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "p", "--cwd=", "--json"},
		{"agent", "adopt", "--pane", "p", "--name", "Bad", "--json"},
		{"agent", "adopt", "extra", "--json"},
		{"agent", "get", "--name", "a", "--id", "0000beef", "--json"},
		{"agent", "message", "--pane", "p", "--id", "0000beef", "--body", "x", "--json"},
		{"agent", "stop", "--name", "a", "--id", "0000beef", "--json"},
		{"agent", "read", "--pane", "p", "--id", "0000beef", "--json"},
		{"agent", "pause", "--name", "a", "--id", "0000beef", "--json"},
		{"agent", "wait", "--name", "a", "--id", "0000beef", "--json"},
		{"agent", "list", "--mine", "--parent", "0000beef", "--json"},
		{"agent", "list", "--parent", "BEEF", "--json"},
		{"agent", "list", "extra", "--json"},
		{"agent", "current", "extra", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out bytes.Buffer
			err := ExecuteWithArgs(args, &out)
			if err == nil {
				t.Fatal("expected invalid input")
			}
			var status interface{ ExitCode() int }
			if !errors.As(err, &status) || status.ExitCode() != 2 {
				t.Fatalf("wrong exit: %v", err)
			}
			var envelope libagent.Outcome
			if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("not one JSON object: %q: %v", out.String(), err)
			}
			if envelope.Status != "rejected" {
				t.Fatal(out.String())
			}
		})
	}
}
func TestAgentGroupHelpListsSubcommands(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stop", "get", "read", "wait", "adopt", "current"} {
		if !strings.Contains(out.String(), "\n  "+name+" ") {
			t.Fatalf("%s: %s", name, out.String())
		}
	}
}
func TestNativeJSONTokenDoesNotSelectOutput(t *testing.T) {
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--args", "--json"}, &out)
	if err == nil {
		t.Fatal("missing name accepted")
	}
	if strings.HasPrefix(out.String(), "{") {
		t.Fatal("native token treated as output flag")
	}
}
func TestMessageBodyJSONTokenDoesNotSelectOutput(t *testing.T) {
	var out bytes.Buffer
	ExecuteWithArgs([]string{"agent", "message", "--body", "--json"}, &out)
	if strings.HasPrefix(out.String(), "{") {
		t.Fatal("body treated as output flag")
	}
}

type brokenWriter struct{ writes int }

func (w *brokenWriter) Write(p []byte) (int, error) {
	w.writes++
	return 0, errors.New("broken output")
}
func TestAgentOutputFailureNotReclassified(t *testing.T) {
	w := &brokenWriter{}
	err := ExecuteWithArgs([]string{"agent", "spawn", "--json"}, w)
	if ExitCode(err) != 1 || w.writes != 1 {
		t.Fatalf("exit=%d writes=%d err=%v", ExitCode(err), w.writes, err)
	}
}

// Record ID flags describe lookups by terminal, which follow a moved pane.
func TestRecordIDFlagHelp(t *testing.T) {
	var walk func(c *cobra.Command)
	found := 0
	walk = func(c *cobra.Command) {
		for _, name := range []string{"id", "agent-id"} {
			f := c.Flags().Lookup(name)
			if f == nil || !strings.Contains(f.Usage, "record ID") {
				continue
			}
			found++
			if !strings.Contains(f.Usage, "follows its terminal to a new pane; fails if the terminal is gone") {
				t.Errorf("%s --%s: %q", c.CommandPath(), name, f.Usage)
			}
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(NewRootCmd())
	if found != 8 {
		t.Fatalf("found %d record ID flags, want 8", found)
	}
}
