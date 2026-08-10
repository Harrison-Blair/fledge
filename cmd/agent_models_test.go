package cmd

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/harness"
)

func TestAgentModelsListsInstalledHarnessesInSupportedOrder(t *testing.T) {
	var discovered []string
	command := newAgentModelsCommand(func(name string) (string, error) {
		if name == "claude" || name == "pi" || name == "opencode" {
			return "/tools/" + name, nil
		}
		return "", errors.New("not installed")
	}, func(_ context.Context, selected harness.Harness, _ harness.DiscoveryOptions) harness.Catalog {
		discovered = append(discovered, selected.ID)
		return harness.Catalog{Models: []harness.Model{{Name: "Harness default", Description: "Use the harness default", Default: true}}}
	})
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"claude", "pi", "opencode"}; !reflect.DeepEqual(discovered, want) {
		t.Fatalf("discovery order = %v, want %v", discovered, want)
	}
	claudeAt := strings.Index(output.String(), "Claude Code")
	piAt := strings.Index(output.String(), "Pi")
	openCodeAt := strings.Index(output.String(), "OpenCode")
	if claudeAt < 0 || piAt <= claudeAt || openCodeAt <= piAt {
		t.Errorf("output harness order is not Claude, Pi, OpenCode: %q", output.String())
	}
	if got := strings.Count(output.String(), "(default)"); got != 3 {
		t.Errorf("default rows = %d, want 3: %q", got, output.String())
	}
}

func TestAgentModelsScopesByExistingHarnessResolution(t *testing.T) {
	for _, argument := range []string{"claude", "Claude Code", "CLAUDE"} {
		t.Run(argument, func(t *testing.T) {
			var discovered []string
			command := newAgentModelsCommand(installedClaudeAndCodex, func(_ context.Context, selected harness.Harness, _ harness.DiscoveryOptions) harness.Catalog {
				discovered = append(discovered, selected.ID)
				return harness.Catalog{Models: []harness.Model{
					{Name: "Harness default", Description: "Use the harness default", Default: true},
					{ID: "opus", Name: "Opus", Provider: "anthropic", Description: "Moving alias"},
				}}
			})
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs([]string{argument})

			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if want := []string{"claude"}; !reflect.DeepEqual(discovered, want) {
				t.Fatalf("discovered = %v, want %v", discovered, want)
			}
			wantOutput := "HARNESS      PROVIDER / INTEGRATION  MODEL      NAME             DESCRIPTION\n" +
				"Claude Code  Claude Code             (default)  Harness default  Use the harness default\n" +
				"Claude Code  Anthropic               opus       Opus             Moving alias\n"
			if got := output.String(); got != wantOutput {
				t.Errorf("output = %q, want %q", got, wantOutput)
			}
		})
	}
}

func TestAgentModelsPreservesWarningAndAvailableRows(t *testing.T) {
	command := newAgentModelsCommand(installedClaudeAndCodex, func(_ context.Context, selected harness.Harness, _ harness.DiscoveryOptions) harness.Catalog {
		return harness.Catalog{
			Models: []harness.Model{
				{Name: "Harness default", Description: "Use the harness default", Default: true},
				{ID: "cached-model", Name: "Cached Model", Description: "Last cached model"},
			},
			Warning: "model discovery for Codex failed: cache unavailable",
		}
	})
	var output, warnings bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&warnings)
	command.SetArgs([]string{"codex"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"(default)", "cached-model", "Cached Model"} {
		if !strings.Contains(output.String(), value) {
			t.Errorf("output = %q, want %q", output.String(), value)
		}
	}
	if got, want := warnings.String(), "Warning: model discovery for Codex failed: cache unavailable\n"; got != want {
		t.Errorf("warnings = %q, want %q", got, want)
	}
}

func TestAgentModelsErrors(t *testing.T) {
	tests := []struct {
		name     string
		lookPath harness.LookPath
		args     []string
		want     string
	}{
		{name: "none installed", lookPath: func(string) (string, error) { return "", errors.New("missing") }, want: "no supported agent harnesses are installed"},
		{name: "requested unavailable", lookPath: installedClaudeAndCodex, args: []string{"pi"}, want: `requested harness "pi" is not installed`},
		{name: "too many arguments", lookPath: installedClaudeAndCodex, args: []string{"claude", "codex"}, want: "accepts at most 1 arg(s)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newAgentModelsCommand(test.lookPath, nil)
			command.SetArgs(test.args)
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func installedClaudeAndCodex(name string) (string, error) {
	if name == "claude" || name == "codex" {
		return "/tools/" + name, nil
	}
	return "", errors.New("not installed")
}
