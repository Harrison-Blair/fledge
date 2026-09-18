package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentlogic "github.com/Harrison-Blair/fledge/internal/agent"
)

func TestAgentHelp(t *testing.T) {
	for _, args := range [][]string{{"agent", "--help"}, {"agent", "spawn", "--help"}, {"agent", "list", "--help"}, {"agent", "message", "--help"}, {"agent", "models", "--help"}, {"agent", "stop", "--help"}} {
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
		{"agent", "stop", "--json"},
		{"agent", "stop", "--name", "a", "--pane", "p", "--json"},
		{"agent", "stop", "extra", "--json"},
		{"agent", "spawn", "--name", "worker", "--harness", "claude", "--pane", "p", "--cwd=", "--json"},
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
			var envelope agentlogic.Outcome
			if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("not one JSON object: %q: %v", out.String(), err)
			}
			if envelope.Status != "rejected" {
				t.Fatal(out.String())
			}
		})
	}
}
func TestAgentGroupHelpListsStop(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\n  stop ") {
		t.Fatal(out.String())
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
