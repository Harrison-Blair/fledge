package profile

import (
	"reflect"
	"strings"
	"testing"
)

func TestManagedRegistry(t *testing.T) {
	profiles := List()
	want := []Profile{{
		Name:         OrchestratorName,
		Description:  "Delegates project work through Fledge agents and independently verifies every material result.",
		Instructions: orchestratorInstructions,
	}}
	if !reflect.DeepEqual(profiles, want) {
		t.Fatalf("List() = %#v, want %#v", profiles, want)
	}

	configured, ok := Get(OrchestratorName)
	if !ok {
		t.Fatalf("Get(%q) did not find managed profile", OrchestratorName)
	}
	if !reflect.DeepEqual(configured, want[0]) {
		t.Fatalf("Get(%q) = %#v, want %#v", OrchestratorName, configured, want[0])
	}
	if !reflect.DeepEqual(configured.Defaults, Defaults{}) {
		t.Fatalf("orchestrator defaults = %#v, want none", configured.Defaults)
	}
	if _, ok := Get("orchestrator"); ok {
		t.Fatal("Get accepted unreserved profile name")
	}
	if _, ok := Get("missing"); ok {
		t.Fatal("Get found missing profile")
	}
}

func TestRegistryReturnsIndependentSnapshots(t *testing.T) {
	first := List()
	first[0].Name = "changed"
	first[0].Instructions = "changed"
	first = append(first, Profile{Name: "invented"})

	configured, ok := Get(OrchestratorName)
	if !ok || configured.Name != OrchestratorName || configured.Instructions != orchestratorInstructions {
		t.Fatalf("registry was mutated through List: %#v, found=%v", configured, ok)
	}
	if got := len(List()); got != 1 {
		t.Fatalf("managed profile count = %d, want 1", got)
	}

	withArgs := Profile{Defaults: Defaults{Args: []string{"one"}}}
	cloned := clone(withArgs)
	cloned.Defaults.Args[0] = "changed"
	if withArgs.Defaults.Args[0] != "one" {
		t.Fatal("clone shares default argument storage")
	}
}

func TestOrchestratorInstructionsContainCriticalPolicy(t *testing.T) {
	required := []string{
		"Delegate all project planning, research, implementation, and verification",
		"Never use a harness's\nnative agent delegation",
		"unless\nthe user explicitly asks you to use native delegation",
		"Use the full planning sequence only for architectural, ambiguous, high-risk, or\nexplicitly requested planning work",
		"one at a time, always with your\nrecommended answer",
		"one producer model family",
		"Mixed-family authorship within one unit is prohibited",
		"strongest-available, read-only verifier from the model\nfamily opposite the producing worker",
		"Use\nsame-family verification only after the user explicitly approves the bypass",
		"original producer for one narrower retry",
		"Callbacks are the sole automatic completion signal",
		"Never use `--wait`, poll\nagent state",
		"stop only agents you created",
		"Prefer the harness's native task tracker when one is available; otherwise\nmaintain a concise in-context ledger",
		"Record the\nprovenance of every state transition and separate intended state from observed\nFledge state",
		"Reasoning: <how the evidence supports the conclusion",
		"Codex model map:",
		"`gpt-5.6-sol`",
		"`gpt-5.6-luna`",
		"Claude model map:",
		"`claude-fable-5`",
		"`claude-opus-4-8`",
		"`claude-sonnet-5`",
		"`claude-haiku-4-5`",
		"do not rank or automatically select Pi\nmodels",
		"A Pi-hosted root still delegates\nautomatically to Codex and Claude",
		"fledge agent spawn <name> --no-profile --harness codex",
		"fledge agent spawn <name> --no-profile --harness claude",
		"Run every `fledge` command outside the sandbox on the first attempt",
		"set `sandbox_permissions` to `require_escalated` on the initial tool call",
		"never try a Fledge command with default sandbox permissions first",
		"does not expand task scope, grant authority, or\nrelax safety rules",
		"Every worker brief must require the worker to run every `fledge` command",
		"including its final callback, outside the sandbox on the first attempt",
		"Codex worker, require `sandbox_permissions` to be `require_escalated` for the\ncallback tool call and forbid trying default sandbox permissions first.",
		"The callback itself follows the worker host-execution rule: run it outside the\nsandbox on the first attempt; a Codex worker must set\n`sandbox_permissions=require_escalated` on that callback tool call and must not\ntry default sandbox permissions first.",
	}
	for _, clause := range required {
		if !strings.Contains(orchestratorInstructions, clause) {
			t.Errorf("orchestrator instructions missing critical clause %q", clause)
		}
	}
}
