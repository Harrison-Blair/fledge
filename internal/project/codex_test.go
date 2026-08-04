package project

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCodexRulesCreatesExactFileAndIsIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ".codex", "rules", "fledge.rules")
	for range 2 {
		if err := EnsureCodexRules(root); err != nil {
			t.Fatalf("EnsureCodexRules() error = %v", err)
		}
		assertFileContents(t, path, codexRulesContents)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("rule permissions = %o, want 644", got)
	}
}

func TestCodexRulesForbidHerdrCommunicationCommands(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"wait", "read", "get", "list", "prompt", "send-keys", "attach", "explain"} {
		if !strings.Contains(codexRulesContents, `"`+command+`"`) ||
			!strings.Contains(codexRulesContents, "herdr agent "+command) {
			t.Errorf("codexRulesContents does not forbid herdr agent %s", command)
		}
	}
	for _, required := range []string{
		`pattern = ["fledge"]`,
		`pattern = ["fledge", "start"]`,
		`pattern = ["fledge", "stop"]`,
		`pattern = ["herdr", "agent"`,
		`pattern = ["herdr", "api", "snapshot"]`,
		`pattern = ["herdr", ["--session", "--remote"]]`,
		`decision = "allow"`,
		`decision = "forbidden"`,
	} {
		if !strings.Contains(codexRulesContents, required) {
			t.Errorf("codexRulesContents = %q, want containing %q", codexRulesContents, required)
		}
	}
}

func TestCodexRulesEvaluator(t *testing.T) {
	t.Parallel()

	codex, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex executable is not installed")
	}
	rulesPath := filepath.Join(t.TempDir(), "fledge.rules")
	if err := os.WriteFile(rulesPath, []byte(codexRulesContents), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		args     []string
		decision string
	}{
		{name: "agent command", args: []string{"herdr", "agent", "read", "worker"}, decision: "forbidden"},
		{name: "global session bypass", args: []string{"herdr", "--session", "demo", "agent", "wait", "worker"}, decision: "forbidden"},
		{name: "remote bypass", args: []string{"herdr", "--remote", "host", "agent", "prompt", "worker", "status"}, decision: "forbidden"},
		{name: "API snapshot", args: []string{"herdr", "api", "snapshot"}, decision: "forbidden"},
		{name: "noncommunication Herdr", args: []string{"herdr", "api", "schema"}},
		{name: "Fledge coordination", args: []string{"fledge", "agent", "message", "send", "worker", "status"}, decision: "allow"},
		{name: "Fledge lifecycle", args: []string{"fledge", "stop"}, decision: "forbidden"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"execpolicy", "check", "--rules", rulesPath, "--"}, test.args...)
			output, err := exec.Command(codex, args...).Output()
			if err != nil {
				t.Fatalf("codex execpolicy check: %v", err)
			}
			var result struct {
				Decision string `json:"decision"`
			}
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("decode codex execpolicy output %q: %v", output, err)
			}
			if result.Decision != test.decision {
				t.Errorf("decision = %q, want %q", result.Decision, test.decision)
			}
		})
	}
}

func TestEnsureCodexRulesPreservesConflict(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ".codex", "rules", "fledge.rules")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const contents = "# user policy\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	err := EnsureCodexRules(root)
	if err == nil || !strings.Contains(err.Error(), "conflicts") || !strings.Contains(err.Error(), "move or remove") {
		t.Fatalf("EnsureCodexRules() error = %v, want actionable conflict", err)
	}
	assertFileContents(t, path, contents)
}

func TestEnsureCodexRulesRejectsLegacyPolicyWithoutMigrating(t *testing.T) {
	t.Parallel()

	for name, contents := range map[string]string{
		"previous": previousCodexRulesContents,
		"legacy":   legacyCodexRulesContents,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".codex", "rules", "fledge.rules")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}

			err := EnsureCodexRules(root)
			if err == nil || !strings.Contains(err.Error(), "run fledge init") {
				t.Fatalf("EnsureCodexRules() error = %v, want run-fledge-init guidance", err)
			}
			assertFileContents(t, path, contents)
		})
	}
}
