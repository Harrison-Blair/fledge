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

func builtinProfile(t *testing.T, name string) profiles.Profile {
	t.Helper()
	p, err := profiles.Load(context.Background(), t.TempDir(), name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// noMemories ends a brief spawned outside any repository.
const noMemories = "\n## Project memory\nNo project memories yet."

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

func TestSpawnProfileSendsOneHeaderedPromptWithoutLaunchSettings(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), labeledResult(), startedResult("pi"), readyAs("pi"), promptedResult())
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "pi", "--profile", "planner", "--pane", "w1:p1", "--prompt", "plan this", "--json"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 5)
	if kind, args := startArgs(t, calls[2]); kind != "pi" || !reflect.DeepEqual(args, []string{}) {
		t.Fatalf("%s %q", kind, args)
	}
	if calls[4].Method != "agent.prompt" || !headered(t, calls[4], builtinProfile(t, "planner").Brief()+noMemories+"\n\nplan this") {
		t.Fatalf("%s", calls[4].Params)
	}
	var envelope struct {
		Status string
		Result struct {
			Harness         string
			PromptRequested bool `json:"prompt_requested"`
			Profile         map[string]any
		}
	}
	if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err, out.String())
	}
	if envelope.Status != "success" || envelope.Result.Harness != "pi" || !envelope.Result.PromptRequested || envelope.Result.Profile["name"] != "planner" || envelope.Result.Profile["source"] != "builtin" || len(envelope.Result.Profile) != 3 {
		t.Fatal(out.String())
	}
}

// Explicit --model, --args, and post-- tokens are the whole native argv.
func TestSpawnProfileUsesOnlyExplicitNativeTokens(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	done := serveRPCs(l, snapshotResult(), labeledResult(), startedResult("claude"), readyAs("claude"), promptedResult())
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--model", "sonnet", "--profile", "implementer", "--pane", "w1:p1", "--args=--search", "--", "--permission-mode", "bypassPermissions"}, &out)
	if err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 5)
	if kind, args := startArgs(t, calls[2]); kind != "claude" || !reflect.DeepEqual(args, []string{"--model", "sonnet", "--search", "--permission-mode", "bypassPermissions"}) {
		t.Fatalf("%s %q", kind, args)
	}
	if !headered(t, calls[4], builtinProfile(t, "implementer").Brief()+noMemories) {
		t.Fatalf("%s", calls[4].Params)
	}
	if !strings.Contains(out.String(), "  profile: implementer (built-in)\n") {
		t.Fatal(out.String())
	}
}

func TestSpawnProfileReadsInvokingRepositoryOverride(t *testing.T) {
	t.Setenv("HERDR_PANE_ID", "")
	l := newSocket(t)
	gitRepo(t)
	os.MkdirAll(filepath.Join(".fledge", "profiles"), 0755)
	os.WriteFile(filepath.Join(".fledge", "profiles", "reviewer.md"), []byte("Local role.\n"), 0644)
	done := serveRPCs(l, snapshotResult(), labeledResult(), startedResult("claude"), readyAs("claude"), promptedResult())
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", "claude", "--profile", "reviewer", "--pane", "w1:p1", "--prompt", "go"}, &out); err != nil {
		t.Fatal(err, out.String())
	}
	calls := waitCalls(t, l, done, 5)
	if kind, args := startArgs(t, calls[2]); kind != "claude" || !reflect.DeepEqual(args, []string{"--permission-mode", "bypassPermissions"}) {
		t.Fatalf("%s %q", kind, args)
	}
	if want := (profiles.Profile{Role: "Local role.\n"}).Brief() + noMemories + "\n\ngo"; !headered(t, calls[4], want) {
		t.Fatalf("%s", calls[4].Params)
	}
}

func TestSpawnProfileHonorsPermissionOptOut(t *testing.T) {
	for _, harness := range []string{"claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			t.Setenv("HERDR_PANE_ID", "")
			l := newSocket(t)
			done := serveRPCs(l, snapshotResult(), labeledResult(), startedResult(harness), readyAs(harness), promptedResult())
			var out bytes.Buffer
			if err := ExecuteWithArgs([]string{"agent", "spawn", "--name", "worker", "--harness", harness, "--profile", "reviewer", "--pane", "w1:p1", "--no-permission-bypass"}, &out); err != nil {
				t.Fatal(err, out.String())
			}
			calls := waitCalls(t, l, done, 5)
			if kind, args := startArgs(t, calls[2]); kind != harness || !reflect.DeepEqual(args, []string{}) {
				t.Fatalf("%s %q", kind, args)
			}
			if !headered(t, calls[4], builtinProfile(t, "reviewer").Brief()+noMemories) {
				t.Fatalf("%s", calls[4].Params)
			}
		})
	}
}

func TestSpawnInvalidProfileFailsBeforeSocket(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--harness", "claude", "--profile", "nope"}, "rejected: unknown profile \"nope\""},
		{[]string{"--profile", "reviewer"}, "rejected: --harness is required"},
		{[]string{"--harness", "claude", "--profile", "old"}, "old.toml is a legacy TOML profile"},
		{[]string{"--harness", "claude", "--profile", "reviewer", "--no-wait"}, "rejected: --no-wait"},
	} {
		l := newSocket(t)
		gitRepo(t)
		os.MkdirAll(filepath.Join(".fledge", "profiles"), 0755)
		os.WriteFile(filepath.Join(".fledge", "profiles", "old.toml"), []byte("schema_version = 1\n"), 0644)
		done := serveRPCs(l)
		var out bytes.Buffer
		if err := ExecuteWithArgs(append([]string{"agent", "spawn", "--name", "worker", "--pane", "w1:p1"}, tc.args...), &out); err == nil {
			t.Fatal(out.String())
		}
		waitCalls(t, l, done, 0)
		if !strings.HasPrefix(out.String(), "rejected: ") || !strings.Contains(out.String(), strings.TrimPrefix(tc.want, "rejected: ")) {
			t.Fatalf("%q want %q", out.String(), tc.want)
		}
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
		Result struct{ Profiles []map[string]any }
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope.Status != "success" || len(envelope.Result.Profiles) != 8 {
		t.Fatal(err, out.String())
	}
	for _, p := range envelope.Result.Profiles {
		for _, retired := range []string{"harness", "model", "args", "reads", "protocol", "sections", "base"} {
			if _, ok := p[retired]; ok {
				t.Fatalf("%s still has %s", p["name"], retired)
			}
		}
	}
	out.Reset()
	if err := ExecuteWithArgs([]string{"agent", "profiles", "verifier"}, &out); err != nil || !strings.Contains(out.String(), "Profile verifier\n  source: built-in\n  brief:\n    ## Mission\n    Decide independently") {
		t.Fatal(err, out.String())
	}
	out.Reset()
	if err := ExecuteWithArgs([]string{"agent", "profiles", "a", "b"}, &out); err == nil {
		t.Fatal("accepted two names")
	}
}

func TestProfileHelpDescribesMarkdownBriefsAndExplicitHarness(t *testing.T) {
	for _, args := range [][]string{{"agent", "profiles", "--help"}, {"agent", "spawn", "--help"}} {
		var out bytes.Buffer
		if err := ExecuteWithArgs(args, &out); err != nil {
			t.Fatal(err)
		}
		help := out.String()
		if !strings.Contains(help, ".fledge/profiles/NAME.md") || strings.Contains(help, ".toml") || strings.Contains(help, "skipped read") || strings.Contains(help, "extends") {
			t.Fatalf("%v:\n%s", args, help)
		}
	}
	var out bytes.Buffer
	ExecuteWithArgs([]string{"agent", "spawn", "--help"}, &out)
	if !strings.Contains(out.String(), "Herdr harness kind (required)") || strings.Contains(out.String(), "unless the profile") {
		t.Fatal(out.String())
	}
}
