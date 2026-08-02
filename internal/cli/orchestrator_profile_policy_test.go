package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/agentprofile"
)

func TestRepositoryOrchestratorProfilePinsAsynchronousMessagingPolicy(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, "..", "..", ".fledge", "profiles", "orchestrator.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"Delegate every task with `fledge agent message send <name> <task>`",
		"Never use direct prompts, waits, repeated status/read calls, or background polling to detect task completion.",
		"Fledge injects replies into your pane as they arrive.",
		"respond with `fledge agent message reply <message-id> <result>`",
		"Use `fledge agent message ack <message-id>` only for informational messages that require no result.",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("orchestrator profile is missing policy wording %q:\n%s", required, text)
		}
	}
	for _, prohibited := range []string{"prompt --wait", "agent wait", "single atomic wait"} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("orchestrator profile retains wait policy wording %q:\n%s", prohibited, text)
		}
	}
}

func TestRepositoryContextMatchesCanonicalOrchestratorProfile(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(cwd, "..", ".."))
	store, err := agentprofile.New(root)
	if err != nil {
		t.Fatal(err)
	}
	profile, loadErr := store.Load(orchestratorProfileName)
	closeErr := store.Close()
	if loadErr != nil || closeErr != nil {
		t.Fatal(errors.Join(loadErr, closeErr))
	}
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	wantBlock := "<!-- <fledge-managed-orchestrator> -->\n" +
		"## Fledge Orchestrator (managed)\n\n" +
		strings.Trim(profile.Instructions, "\r\n") + "\n" +
		"<!-- </fledge-managed-orchestrator> -->"
	if strings.Count(string(agents), wantBlock) != 1 {
		t.Fatalf("AGENTS.md does not contain exactly one canonical managed block:\n%s", agents)
	}
	claude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	wantClaude := "<!-- <fledge-managed-orchestrator> -->\n@AGENTS.md\n<!-- </fledge-managed-orchestrator> -->\n"
	if string(claude) != wantClaude {
		t.Fatalf("CLAUDE.md bridge = %q, want %q", claude, wantClaude)
	}
}

func TestAgentCommandExposesMessagingButNoPromptOrWait(t *testing.T) {
	commands := map[string]bool{}
	for _, command := range newAgent(&environment{}).Commands() {
		commands[command.Name()] = true
	}
	if !commands["message"] {
		t.Fatal("agent message command is missing")
	}
	for _, removed := range []string{"prompt", "wait"} {
		if commands[removed] {
			t.Fatalf("removed agent %s command is still registered", removed)
		}
	}
}

func TestRemovedPromptAndWaitCommandsFailAsUsageErrors(t *testing.T) {
	for _, removed := range []string{"prompt", "wait"} {
		var stdout, stderr strings.Builder
		code := Execute(t.Context(), []string{"agent", removed, "worker"}, nil, &stdout, &stderr)
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "accepts no arguments") {
			t.Fatalf("agent %s: exit=%d stdout=%q stderr=%q", removed, code, stdout.String(), stderr.String())
		}
	}
}
