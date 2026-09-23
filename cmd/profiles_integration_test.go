package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/profiles"
)

func builtinRole(t *testing.T, name string) string {
	t.Helper()
	p, err := profiles.Load(context.Background(), t.TempDir(), name)
	if err != nil {
		t.Fatal(err)
	}
	return p.Role
}

func startArgs(t *testing.T, c rpcCall) (string, []string) {
	t.Helper()
	var params struct {
		Kind string   `json:"kind"`
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(c.Params, &params); err != nil {
		t.Fatal(err)
	}
	return params.Kind, params.Args
}

func TestSpawnProfileFlagSetsLaunchAndOneHeaderedPrompt(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), startedResult("pi"), readyAs("pi"), promptedResult())
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--profile", "reviewer", "--pane", "w1:p1", "--prompt", "review this", "--json"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 4)
	if kind, args := startArgs(t, calls[1]); kind != "pi" || !reflect.DeepEqual(args, []string{"--model", "openai-codex/gpt-6-astra"}) {
		t.Fatalf("%s %q", kind, args)
	}
	if calls[3].Method != "agent.prompt" || !headered(t, calls[3], builtinRole(t, "reviewer")+"\n\nreview this") {
		t.Fatalf("%s", calls[3].Params)
	}
	var envelope struct {
		Status string
		Result struct {
			Harness         string
			PromptRequested bool `json:"prompt_requested"`
			Profile         struct{ Name, Source string }
		}
	}
	if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err, out.String())
	}
	if envelope.Status != "success" || envelope.Result.Harness != "pi" || !envelope.Result.PromptRequested || envelope.Result.Profile.Name != "reviewer" || envelope.Result.Profile.Source != "builtin" {
		t.Fatal(out.String())
	}
}

// Explicit --args and post-- tokens replace the profile's args, in order.
func TestSpawnProfileExplicitNativeTokensReplaceProfileArgs(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), startedResult("pi"), readyAs("pi"), promptedResult())
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--profile", "planner", "--pane", "w1:p1", "--args=--search", "--", "--native"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 4)
	if kind, args := startArgs(t, calls[1]); kind != "pi" || !reflect.DeepEqual(args, []string{"--model", "openai-codex/gpt-6-astra", "--search", "--native"}) {
		t.Fatalf("%s %q", kind, args)
	}
	if !headered(t, calls[3], builtinRole(t, "planner")) {
		t.Fatalf("%s", calls[3].Params)
	}
	if !strings.Contains(out.String(), "  profile: planner (built-in)\n") {
		t.Fatal(out.String())
	}
}

func TestSpawnProfileReadsInvokingRepositoryOverride(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	gitRepo(t)
	os.MkdirAll(filepath.Join(".fledge", "profiles"), 0755)
	os.WriteFile(filepath.Join(".fledge", "profiles", "reviewer.toml"), []byte("schema_version = 1\nharness = \"claude\"\nmodel = \"sonnet\"\nrole = \"Local role.\"\n"), 0644)
	done := serveRPCs(l, snapshotResult(), startedResult("claude"), readyAs("claude"), promptedResult())
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--profile", "reviewer", "--pane", "w1:p1", "--prompt", "go"}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 4)
	if kind, args := startArgs(t, calls[1]); kind != "claude" || !reflect.DeepEqual(args, []string{"--model", "sonnet"}) {
		t.Fatalf("%s %q", kind, args)
	}
	if !headered(t, calls[3], "Local role.\n\ngo") {
		t.Fatalf("%s", calls[3].Params)
	}
}

func TestSpawnInvalidProfileFailsBeforeSocket(t *testing.T) {
	l := newSocket(t)
	done := serveRPCs(l)
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--profile", "nope", "--pane", "w1:p1"}, &out); err == nil {
		t.Fatal(out.String())
	}
	waitCalls(t, l, done, 0)
	if !strings.HasPrefix(out.String(), "rejected: unknown profile \"nope\"") {
		t.Fatal(out.String())
	}
}

func TestProfilesCommandNeedsNoSocketOrRepository(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_SOCKET_PATH", "")
	t.Chdir(t.TempDir())
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "profiles", "--json"}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	var envelope struct {
		Status string
		Result struct{ Profiles []struct{ Name string } }
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Status != "success" || len(envelope.Result.Profiles) != 5 {
		t.Fatal(err, out.String())
	}
	out.Reset()
	if err := ExecuteWithArgs([]string{"agent", "profiles", "verifier"}, &out); err != nil || !strings.Contains(out.String(), "Profile verifier\n  source: built-in\n") || !strings.Contains(out.String(), "    "+builtinRole(t, "verifier")) {
		t.Fatal(err, out.String())
	}
	out.Reset()
	if err := ExecuteWithArgs([]string{"agent", "profiles", "a", "b"}, &out); err == nil {
		t.Fatal("accepted two names")
	}
}
