package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/agentprofile"
	"github.com/spf13/cobra"
)

func TestRepositoryOrchestratorProfilePinsAtomicWaitPolicy(t *testing.T) {
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
		"Submit work and wait in one atomic operation with `fledge agent prompt <name> <text> --wait --until idle,done,blocked`, or use one `fledge agent wait <name> --until idle,done,blocked` call for an already-running agent.",
		"Do not poll with repeated `fledge agent status` or `fledge agent read` calls, and do not send messages merely to check progress.",
		"Prompt or message an agent again only when there is genuinely new task information.",
		"Avoid artificial short waiting cycles or repeated timeout-driven checks; let the single atomic wait settle.",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("orchestrator profile is missing policy wording %q:\n%s", required, text)
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

func TestAtomicWaitPolicyCommaSeparatedUntilSyntaxMatchesCLI(t *testing.T) {
	want := []string{"idle", "done", "blocked"}
	tests := []struct {
		cmd      *cobra.Command
		args     []string
		wantArgs []string
		wantWait bool
	}{
		{
			cmd:      newAgentPrompt(&environment{}),
			args:     []string{"worker", "task", "--wait", "--until", "idle,done,blocked"},
			wantArgs: []string{"worker", "task"}, wantWait: true,
		},
		{
			cmd:      newAgentWait(&environment{}),
			args:     []string{"worker", "--until", "idle,done,blocked"},
			wantArgs: []string{"worker"},
		},
	}
	for _, test := range tests {
		cmd := test.cmd
		if err := cmd.Flags().Parse(test.args); err != nil {
			t.Fatalf("%s parse comma-separated --until: %v", cmd.Name(), err)
		}
		if got := cmd.Flags().Args(); !reflect.DeepEqual(got, test.wantArgs) {
			t.Fatalf("%s positional args = %v, want %v", cmd.Name(), got, test.wantArgs)
		}
		if err := cmd.Args(cmd, cmd.Flags().Args()); err != nil {
			t.Fatalf("%s policy command shape is invalid: %v", cmd.Name(), err)
		}
		got, err := cmd.Flags().GetStringSlice("until")
		if err != nil {
			t.Fatalf("%s read --until: %v", cmd.Name(), err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s --until = %v, want %v", cmd.Name(), got, want)
		}
		if wait, err := cmd.Flags().GetBool("wait"); err == nil && wait != test.wantWait {
			t.Fatalf("%s --wait = %t, want %t", cmd.Name(), wait, test.wantWait)
		}
	}
}
