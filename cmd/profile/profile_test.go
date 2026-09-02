package profile

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBareProfileShowsHelpWithSubcommands(t *testing.T) {
	command := New()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(nil)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"Inspect Fledge-managed agent profiles", "list", "show"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help output missing %q:\n%s", want, output.String())
		}
	}
}

func TestProfileRejectsArgumentsBelowARoot(t *testing.T) {
	root := &cobra.Command{Use: "fledge"}
	root.AddCommand(New())
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"profile", "unexpected"})

	if err := root.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}

func TestProfileListShowsExactlyTheSelectableProfiles(t *testing.T) {
	command := New()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"list"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("list output has %d lines, want header plus two profiles:\n%s", len(lines), output.String())
	}
	if got, want := normalizeProfileText(lines[0]), "NAME HARNESS MODEL DESCRIPTION"; got != want {
		t.Fatalf("list header = %q, want %q", got, want)
	}

	wantNames := []string{"fledge-general", "fledge-orchestrator"}
	seen := make(map[string]bool, len(wantNames))
	for i, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			t.Fatalf("list row %d has too few fields: %q", i, line)
		}
		name := fields[0]
		if name != wantNames[i] {
			t.Errorf("list row %d name = %q, want %q", i, name, wantNames[i])
		}
		if seen[name] {
			t.Errorf("list contains duplicate profile %q", name)
		}
		seen[name] = true
		for _, fragment := range []string{"fledge-core", "fledge-worker-report"} {
			if name == fragment {
				t.Errorf("list exposes implementation fragment %q", fragment)
			}
		}
	}
	if len(seen) != len(wantNames) {
		t.Fatalf("list names = %v, want exactly %v", seen, wantNames)
	}
}

func TestProfileShowGeneralPrintsCompiledInstructions(t *testing.T) {
	instructions := executeProfileShow(t, "fledge-general")
	normalized := normalizeProfileText(instructions)

	assertMarkersInOrder(t, normalized,
		"# Fledge Session Core",
		"# Fledge Managed Worker",
		"# Fledge Report Protocol",
	)
	for _, marker := range []string{
		"# Fledge Session Core",
		"# Fledge Managed Worker",
		"# Fledge Report Protocol",
	} {
		if got := strings.Count(normalized, marker); got != 1 {
			t.Errorf("general instructions contain %q %d times, want once", marker, got)
		}
	}
	for _, marker := range []string{"# Fledge Orchestrator", "## Root boundary"} {
		if strings.Contains(normalized, marker) {
			t.Errorf("general instructions contain manager-only marker %q", marker)
		}
	}
}

func TestProfileShowOrchestratorPrintsCompiledInstructions(t *testing.T) {
	instructions := executeProfileShow(t, "fledge-orchestrator")
	normalized := normalizeProfileText(instructions)

	assertMarkersInOrder(t, normalized,
		"# Fledge Session Core",
		"# Fledge Orchestrator",
		"# Fledge Report Protocol",
	)
	for _, marker := range []string{
		"# Fledge Session Core",
		"# Fledge Orchestrator",
		"# Fledge Report Protocol",
	} {
		if got := strings.Count(normalized, marker); got != 1 {
			t.Errorf("orchestrator instructions contain %q %d times, want once", marker, got)
		}
	}
	for _, marker := range []string{"# Fledge Managed Worker", "## Initial dispatch brief", "role-neutral managed worker"} {
		if strings.Contains(normalized, marker) {
			t.Errorf("orchestrator instructions contain general-worker marker %q", marker)
		}
	}
	if !strings.Contains(normalized, "claude-fable-5-1") {
		t.Fatal("orchestrator instructions omit automatic strongest claude-fable-5-1")
	}
}

func TestProfileShowRejectsNonSelectableFragments(t *testing.T) {
	for _, name := range []string{"fledge-core", "fledge-worker-report", "fledge-general-worker"} {
		t.Run(name, func(t *testing.T) {
			command := New()
			var stdout, stderr bytes.Buffer
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			command.SetArgs([]string{"show", name})

			err := command.Execute()
			if err == nil || err.Error() != `unknown profile "`+name+`"` {
				t.Fatalf("Execute() error = %v, want unknown profile error", err)
			}
			if strings.Contains(stdout.String(), "Name:") || strings.Contains(stdout.String(), "Instructions:") {
				t.Fatalf("unknown profile produced success output: %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), err.Error()) {
				t.Fatalf("stderr = %q, want the unknown profile error", stderr.String())
			}
		})
	}
}

func executeProfileShow(t *testing.T, name string) string {
	t.Helper()
	command := New()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"show", name})

	if err := command.Execute(); err != nil {
		t.Fatalf("show %q Execute() error = %v", name, err)
	}
	const marker = "Instructions:\n"
	_, instructions, found := strings.Cut(output.String(), marker)
	if !found {
		t.Fatalf("show %q output missing %q:\n%s", name, marker, output.String())
	}
	return instructions
}

func normalizeProfileText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func assertMarkersInOrder(t *testing.T, value string, markers ...string) {
	t.Helper()
	position := -1
	for _, marker := range markers {
		next := strings.Index(value, marker)
		if next <= position {
			t.Fatalf("marker %q is missing or out of order in %q", marker, value)
		}
		position = next
	}
}
