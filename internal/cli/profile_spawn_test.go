package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentprofile"
	"github.com/Harrison-Blair/fledge/internal/agentspawn"
	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrtest"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func TestProfileSpawnUsesDefaultAndOverrideNamesExactArgvRootCWDAndProvenance(t *testing.T) {
	for _, test := range []struct {
		name     string
		override string
		wantName string
	}{
		{name: "profile name is default", wantName: "reviewer"},
		{name: "launch name override", override: "review-run", wantName: "review-run"},
	} {
		t.Run(test.name, func(t *testing.T) {
			env, observation, root, stateDir := profileSpawnFixture(t, agentprofile.Profile{
				Name: "reviewer", SchemaVersion: 1, Harness: "codex", Model: "gpt-5.6",
				Effort: "high", Instructions: "Review deterministically.",
				NativeArgs: []string{"--image=diagram.png"},
			})
			cmd := newAgentSpawn(env)
			args := []string{"reviewer"}
			if test.override != "" {
				args = append(args, "--name", test.override)
			}
			cmd.SetArgs(args)
			if err := cmd.ExecuteContext(t.Context()); err != nil {
				t.Fatalf("profile spawn: %v", err)
			}

			observation.mu.Lock()
			gotCommand, gotCWD := observation.command, observation.cwd
			gotShellProbes := observation.processInfoCalls
			observation.mu.Unlock()
			if gotShellProbes < 2 {
				t.Fatalf("spawn skipped the shell-readiness poll: process_info calls = %d", gotShellProbes)
			}
			// The profile owns harness, model, cwd, and native args, so the
			// injected bootstrap carries only the profile, name, and timeout.
			wantCommand := " agent spawn reviewer --name " + test.wantName + " --timeout 30s --no-pickers"
			if !strings.Contains(gotCommand, wantCommand+" --launch-id launch_") {
				t.Fatalf("bootstrap command = %q, want suffix %q", gotCommand, wantCommand)
			}
			if gotCWD != root {
				t.Fatalf("launch cwd = %q, want %q", gotCWD, root)
			}
			wantArgs := []string{
				"--model", "gpt-5.6",
				"--config", `model_reasoning_effort="high"`,
				"--config", `developer_instructions="Review deterministically."`,
				"--image=diagram.png",
			}

			var envelope struct {
				Data fledge.AgentStartResult `json:"data"`
			}
			if err := json.Unmarshal(env.out.(*bytes.Buffer).Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Data.Agent.Profile != "reviewer" || envelope.Data.Agent.Name != test.wantName ||
				!reflect.DeepEqual(envelope.Data.Argv, wantArgs) {
				t.Fatalf("spawn JSON data = %#v", envelope.Data)
			}
			stored, err := state.New(stateDir)
			if err != nil {
				t.Fatal(err)
			}
			st, err := stored.Read(project.SessionName(root), root)
			if err != nil {
				t.Fatal(err)
			}
			if st.Agents[test.wantName].Profile != "reviewer" || st.LastSpawnSelection != nil {
				t.Fatalf("persisted spawn provenance/selection = %#v", st)
			}
		})
	}
}

func TestProfileSpawnRejectsLockedPerLaunchIdentityAndExtraNativeArgs(t *testing.T) {
	for _, test := range []struct {
		name  string
		flags agentSpawnFlags
		want  string
	}{
		{name: "harness", flags: agentSpawnFlags{harnessSet: true}, want: "--harness is locked"},
		{name: "model", flags: agentSpawnFlags{modelSet: true}, want: "--model is locked"},
		{name: "cwd", flags: agentSpawnFlags{cwdSet: true}, want: "--cwd is locked"},
		{name: "native args", flags: agentSpawnFlags{nativeArgs: []string{"--debug"}}, want: "does not accept extra -- arguments"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := initializedProject(t)
			store, err := agentprofile.New(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Create(agentprofile.Profile{
				Name: "reviewer", SchemaVersion: 1, Harness: "codex", Model: "gpt-5.6",
			}); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			runtimeAccessed := false
			env := &environment{
				in: strings.NewReader(""), out: &bytes.Buffer{}, errOut: &bytes.Buffer{}, cwd: root,
				lookPath: func(string) (string, error) { runtimeAccessed = true; return "", os.ErrNotExist },
			}
			cmd := newAgentSpawn(env)
			cmd.SetContext(t.Context())
			flags := test.flags
			flags.profile, flags.timeout = "reviewer", 30*time.Second
			err = runAgentSpawn(cmd, env, flags)
			var useErr *usageError
			if err == nil || !strings.Contains(err.Error(), test.want) || !errors.As(err, &useErr) {
				t.Fatalf("error = %v", err)
			}
			if runtimeAccessed {
				t.Fatal("locked override accessed harness runtime")
			}
		})
	}
}

func TestGenericProfileExplicitSelectionsTransportInstructionsAndRecordProvenance(t *testing.T) {
	env, observation, root, stateDir := profileSpawnFixture(t, agentprofile.Profile{
		Name: "generic", SchemaVersion: 1,
		Instructions: "Use only inherited Fledge orchestration.",
	})
	cmd := newAgentSpawn(env)
	cmd.SetContext(t.Context())
	err := runAgentSpawn(cmd, env, agentSpawnFlags{
		profile: "generic", harness: "codex", model: "gpt-custom",
		harnessSet: true, modelSet: true, timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation.mu.Lock()
	gotCommand, gotCWD := observation.command, observation.cwd
	observation.mu.Unlock()
	// Nothing is locked by the profile, so the explicit selections travel.
	wantCommand := " agent spawn generic --name generic --harness codex --model gpt-custom --timeout 30s --no-pickers"
	if !strings.Contains(gotCommand, wantCommand+" --launch-id launch_") || gotCWD != root {
		t.Fatalf("command/cwd = %q / %q, want suffix %q / %q", gotCommand, gotCWD, wantCommand, root)
	}
	wantArgs := []string{
		"--model", "gpt-custom",
		"--config", `developer_instructions="Use only inherited Fledge orchestration."`,
	}
	var envelope struct {
		Data fledge.AgentStartResult `json:"data"`
	}
	if err := json.Unmarshal(env.out.(*bytes.Buffer).Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(envelope.Data.Argv, wantArgs) {
		t.Fatalf("native argv = %#v, want %#v", envelope.Data.Argv, wantArgs)
	}
	stored, err := state.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := stored.Read(project.SessionName(root), root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Agents["generic"].Profile != "generic" || st.LastSpawnSelection != nil {
		t.Fatalf("state = %#v", st)
	}
}

func TestGenericProfilePickerFiltersCompatibilityAndReusesLastSelection(t *testing.T) {
	env, observation, root, stateDir := profileSpawnFixture(t, agentprofile.Profile{
		Name: "generic", SchemaVersion: 1, Instructions: "Managed instructions.",
	})
	env.json = false
	env.stdinTTY = func() bool { return true }
	env.in = bytes.NewBufferString("\r")
	stored, err := state.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := stored.WithLocked(project.SessionName(root), root, func(st *state.Session) error {
		st.LastSpawnSelection = &state.SpawnSelection{Harness: "codex", Model: "gpt-last"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cmd := newAgentSpawn(env)
	cmd.SetContext(t.Context())
	if err := runAgentSpawn(cmd, env, agentSpawnFlags{profile: "generic", timeout: 30 * time.Second}); err != nil {
		t.Fatal(err)
	}
	observation.mu.Lock()
	got := observation.command
	observation.mu.Unlock()
	want := " agent spawn generic --name generic --harness codex --model gpt-last --timeout 30s --no-pickers"
	if !strings.Contains(got, want+" --launch-id launch_") {
		t.Fatalf("command = %q, want suffix %q", got, want)
	}
	output := env.out.(*bytes.Buffer).String()
	if !strings.Contains(output, "Last used — Codex · gpt-last") || !strings.Contains(output, "Pi coding agent") || strings.Contains(output, "OpenCode") {
		t.Fatalf("compatibility-filtered picker output = %q", output)
	}
}

func TestGenericProfileUsesPickersForUnsetSelections(t *testing.T) {
	t.Run("harness", func(t *testing.T) {
		env, observation, _, _ := profileSpawnFixture(t, agentprofile.Profile{
			Name: "generic", SchemaVersion: 1, Instructions: "Managed instructions.",
		})
		env.json = false
		env.stdinTTY = func() bool { return true }
		env.in = bytes.NewBufferString("\r") // Claude Code.
		cmd := newAgentSpawn(env)
		cmd.SetContext(t.Context())
		if err := runAgentSpawn(cmd, env, agentSpawnFlags{
			profile: "generic", model: "sonnet", modelSet: true, timeout: 30 * time.Second,
		}); err != nil {
			t.Fatal(err)
		}
		observation.mu.Lock()
		got := observation.command
		observation.mu.Unlock()
		want := " agent spawn generic --name generic --harness claude --model sonnet --timeout 30s --no-pickers"
		if !strings.Contains(got, want+" --launch-id launch_") {
			t.Fatalf("command = %q, want suffix %q", got, want)
		}
		if output := env.out.(*bytes.Buffer).String(); !strings.Contains(output, "Agent harness") {
			t.Fatalf("missing generic profile harness picker: %q", output)
		}
	})

	t.Run("model", func(t *testing.T) {
		env, observation, _, _ := profileSpawnFixture(t, agentprofile.Profile{
			Name: "generic", SchemaVersion: 1, Instructions: "Managed instructions.",
		})
		env.json = false
		env.stdinTTY = func() bool { return true }
		env.in = bytes.NewBufferString("\r") // Harness default.
		cmd := newAgentSpawn(env)
		cmd.SetContext(t.Context())
		if err := runAgentSpawn(cmd, env, agentSpawnFlags{
			profile: "generic", harness: "claude", harnessSet: true, timeout: 30 * time.Second,
		}); err != nil {
			t.Fatal(err)
		}
		observation.mu.Lock()
		got := observation.command
		observation.mu.Unlock()
		want := " agent spawn generic --name generic --harness claude --timeout 30s --no-pickers"
		if !strings.Contains(got, want+" --launch-id launch_") || strings.Contains(got, "--model") {
			t.Fatalf("command = %q, want suffix %q without a model", got, want)
		}
		if output := env.out.(*bytes.Buffer).String(); !strings.Contains(output, "Claude Code model") {
			t.Fatalf("missing generic profile model picker: %q", output)
		}
	})
}

// The dedicated-tab bootstrap passes --no-pickers because the in-pane child
// runs on a tty: without it, an intentionally unset model would block forever
// on the interactive model picker inside a pane nobody is watching.
func TestNoPickersMakesASpawnNonInteractive(t *testing.T) {
	t.Run("generic spawn refuses to prompt", func(t *testing.T) {
		env := &environment{
			in: strings.NewReader(""), out: &bytes.Buffer{}, errOut: &bytes.Buffer{},
			stdinTTY: func() bool { return true },
		}
		cmd := newAgentSpawn(env)
		cmd.SetContext(t.Context())
		err := runAgentSpawn(cmd, env, agentSpawnFlags{noPickers: true, timeout: 30 * time.Second})
		var useErr *usageError
		if !errors.As(err, &useErr) || !strings.Contains(err.Error(), "--name and --harness are required") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("unset model launches the harness default without prompting", func(t *testing.T) {
		env, observation, _, _ := profileSpawnFixture(t, agentprofile.Profile{
			Name: "generic", SchemaVersion: 1, Instructions: "Managed instructions.",
		})
		env.json = false
		env.stdinTTY = func() bool { return true }
		env.in = strings.NewReader("") // any picker would fail on the empty input
		cmd := newAgentSpawn(env)
		cmd.SetContext(t.Context())
		if err := runAgentSpawn(cmd, env, agentSpawnFlags{
			profile: "generic", harness: "claude", harnessSet: true,
			noPickers: true, newTab: true, timeout: 30 * time.Second,
		}); err != nil {
			t.Fatal(err)
		}
		observation.mu.Lock()
		got := observation.command
		observation.mu.Unlock()
		want := " agent spawn generic --name generic --harness claude --timeout 30s --no-pickers"
		if !strings.Contains(got, want+" --launch-id launch_") || strings.Contains(got, "--model") {
			t.Fatalf("command = %q, want suffix %q without a model", got, want)
		}
		if output := env.out.(*bytes.Buffer).String(); strings.Contains(output, "Claude Code model") {
			t.Fatalf("no-pickers spawn opened the model picker: %q", output)
		}
	})
}

func TestCompatibleProfileHarnessesFiltersManagedInstructionTransport(t *testing.T) {
	installed := []agentspawn.Harness{
		{ID: "claude"}, {ID: "codex"}, {ID: "opencode"}, {ID: "pi"},
	}
	compatible := compatibleProfileHarnesses(installed, agentprofile.Profile{Instructions: "Managed."})
	got := make([]string, 0, len(compatible))
	for _, harness := range compatible {
		got = append(got, harness.ID)
	}
	if !reflect.DeepEqual(got, []string{"claude", "codex", "pi"}) {
		t.Fatalf("compatible harnesses = %v", got)
	}
}

func TestOrchestratorProfileUsesRepositoryContextForEveryHarness(t *testing.T) {
	for _, harness := range []string{"codex", "claude", "pi", "opencode"} {
		t.Run(harness, func(t *testing.T) {
			instructions := "Use inherited Fledge only.\nWait atomically."
			env, observation, root, _ := profileSpawnFixture(t, agentprofile.Profile{
				Name: "orchestrator", SchemaVersion: 1, Instructions: instructions,
			})
			cmd := newAgentSpawn(env)
			cmd.SetContext(t.Context())
			if err := runAgentSpawn(cmd, env, agentSpawnFlags{
				profile: "orchestrator", harness: harness, harnessSet: true, timeout: 30 * time.Second,
			}); err != nil {
				t.Fatal(err)
			}

			observation.mu.Lock()
			agentsAtInject, claudeAtInject := observation.agentsAtInject, observation.claudeAtInject
			observation.mu.Unlock()
			var envelope struct {
				Data fledge.AgentStartResult `json:"data"`
			}
			if err := json.Unmarshal(env.out.(*bytes.Buffer).Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope.Data.Argv) != 0 {
				t.Fatalf("orchestrator native args = %#v, want no duplicate instruction transport", envelope.Data.Argv)
			}
			for path, contents := range map[string]string{
				"AGENTS.md": agentsAtInject,
				"CLAUDE.md": claudeAtInject,
			} {
				if !strings.Contains(contents, "fledge-managed-orchestrator") {
					t.Fatalf("%s was not synchronized before the bootstrap injection: %q", path, contents)
				}
			}
			if got := readFile(t, filepath.Join(root, "AGENTS.md")); !strings.Contains(got, instructions) {
				t.Fatalf("AGENTS.md = %q", got)
			}
		})
	}
}

func TestOrchestratorCompatibilityIncludesContextBackedOpenCode(t *testing.T) {
	installed := []agentspawn.Harness{
		{ID: "claude"}, {ID: "codex"}, {ID: "opencode"}, {ID: "pi"},
	}
	compatible := compatibleProfileHarnesses(installed, agentprofile.Profile{
		Name: "orchestrator", Instructions: "Managed.",
	})
	got := make([]string, 0, len(compatible))
	for _, harness := range compatible {
		got = append(got, harness.ID)
	}
	if !reflect.DeepEqual(got, []string{"claude", "codex", "opencode", "pi"}) {
		t.Fatalf("orchestrator-compatible harnesses = %v", got)
	}
}

func TestOrchestratorContextFailurePreventsHarnessLaunch(t *testing.T) {
	for _, test := range []struct {
		name         string
		instructions string
		prepare      func(*testing.T, string)
	}{
		{
			name: "malformed existing markers", instructions: "Managed.",
			prepare: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "AGENTS.md"),
					[]byte("<!-- <fledge-managed-orchestrator> -->\nmissing end\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{name: "reserved marker injection", instructions: "fledge-managed-orchestrator"},
	} {
		t.Run(test.name, func(t *testing.T) {
			env, observation, root, _ := profileSpawnFixture(t, agentprofile.Profile{
				Name: "orchestrator", SchemaVersion: 1, Harness: "codex", Instructions: test.instructions,
			})
			if test.prepare != nil {
				test.prepare(t, root)
			}
			cmd := newAgentSpawn(env)
			cmd.SetContext(t.Context())
			err := runAgentSpawn(cmd, env, agentSpawnFlags{profile: "orchestrator", timeout: 30 * time.Second})
			var serviceErr *fledge.Error
			if !errors.As(err, &serviceErr) || serviceErr.Code != "profile_launch_invalid" ||
				!strings.Contains(serviceErr.Message, "synchronize managed repository context") {
				t.Fatalf("error = %#v", err)
			}
			observation.mu.Lock()
			defer observation.mu.Unlock()
			if observation.injections != 0 {
				t.Fatalf("bootstrap injections = %d, want 0", observation.injections)
			}
		})
	}
}

func TestProfileSpawnMaterializesPiInstructionsAndUsesPreparedPath(t *testing.T) {
	instructions := "AGENTS.md\nUse inherited Fledge only."
	env, _, root, _ := profileSpawnFixture(t, agentprofile.Profile{
		Name: "managed", SchemaVersion: 1, Effort: "high", Instructions: instructions,
		NativeArgs: []string{"--offline"},
	})
	cmd := newAgentSpawn(env)
	cmd.SetContext(t.Context())
	err := runAgentSpawn(cmd, env, agentSpawnFlags{
		profile: "managed", harness: "pi", model: "openai/gpt-5.6",
		harnessSet: true, modelSet: true, timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	var envelope struct {
		Data fledge.AgentStartResult `json:"data"`
	}
	if err := json.Unmarshal(env.out.(*bytes.Buffer).Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	got := envelope.Data.Argv
	if len(got) != 7 || got[0] != "--model" || got[1] != "openai/gpt-5.6" ||
		got[2] != "--thinking" || got[3] != "high" ||
		got[4] != "--append-system-prompt" || got[6] != "--offline" {
		t.Fatalf("Pi argv = %#v", got)
	}
	promptPath := got[5]
	if !filepath.IsAbs(promptPath) || !strings.HasPrefix(promptPath, filepath.Join(project.TempDir(root), "profile-instructions")+string(filepath.Separator)) {
		t.Fatalf("Pi prompt path = %q", promptPath)
	}
	contents, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != instructions {
		t.Fatalf("Pi prompt contents = %q, want %q", contents, instructions)
	}
}

func TestProfileSpawnReportsPiInstructionPreparationFailureBeforeLaunch(t *testing.T) {
	env, observation, root, _ := profileSpawnFixture(t, agentprofile.Profile{
		Name: "managed", SchemaVersion: 1, Harness: "pi", Instructions: "Managed.",
	})
	tempDir := project.TempDir(root)
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "profile-instructions"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newAgentSpawn(env)
	cmd.SetContext(t.Context())
	err := runAgentSpawn(cmd, env, agentSpawnFlags{profile: "managed", timeout: 30 * time.Second})
	var serviceErr *fledge.Error
	if !errors.As(err, &serviceErr) || serviceErr.Code != "profile_launch_invalid" ||
		!strings.Contains(serviceErr.Message, "could not prepare Pi instructions") {
		t.Fatalf("error = %#v", err)
	}
	observation.mu.Lock()
	defer observation.mu.Unlock()
	if observation.injections != 0 {
		t.Fatalf("bootstrap injections = %d, want 0", observation.injections)
	}
}

func TestGenericProfileDistinguishesUnavailableAndIncompatibleHarnesses(t *testing.T) {
	for _, test := range []struct {
		name      string
		requested string
		installed map[string]bool
		want      string
	}{
		{
			name: "not installed", requested: "claude",
			installed: map[string]bool{"codex": true}, want: `harness "claude" is not installed`,
		},
		{
			name: "installed but incompatible", requested: "opencode",
			installed: map[string]bool{"codex": true, "opencode": true}, want: `harness "opencode" is not compatible`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			env, _, _, _ := profileSpawnFixture(t, agentprofile.Profile{
				Name: "generic", SchemaVersion: 1, Instructions: "Managed instructions.",
			})
			env.lookPath = func(name string) (string, error) {
				if test.installed[name] {
					return "/bin/" + name, nil
				}
				return "", os.ErrNotExist
			}
			cmd := newAgentSpawn(env)
			cmd.SetContext(t.Context())
			err := runAgentSpawn(cmd, env, agentSpawnFlags{
				profile: "generic", harness: test.requested, harnessSet: true, timeout: 30 * time.Second,
			})
			var useErr *usageError
			if !errors.As(err, &useErr) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestProfileSpawnFlagsOnlyFillFieldsTheProfileDoesNotOwn(t *testing.T) {
	t.Run("model fills unset model", func(t *testing.T) {
		env, observation, _, _ := profileSpawnFixture(t, agentprofile.Profile{
			Name: "partial", SchemaVersion: 1, Harness: "codex",
		})
		cmd := newAgentSpawn(env)
		cmd.SetContext(t.Context())
		if err := runAgentSpawn(cmd, env, agentSpawnFlags{
			profile: "partial", model: "gpt-explicit", modelSet: true, timeout: 30 * time.Second,
		}); err != nil {
			t.Fatal(err)
		}
		observation.mu.Lock()
		defer observation.mu.Unlock()
		// The profile locks the harness, so only the explicit model travels.
		want := " agent spawn partial --name partial --model gpt-explicit --timeout 30s --no-pickers"
		if !strings.Contains(observation.command, want+" --launch-id launch_") || strings.Contains(observation.command, "--harness") {
			t.Fatalf("command = %q, want suffix %q without a harness flag", observation.command, want)
		}
	})

	t.Run("noninteractive generic requires harness", func(t *testing.T) {
		root := initializedProject(t)
		store, err := agentprofile.New(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Create(agentprofile.Profile{Name: "generic", SchemaVersion: 1}); err != nil {
			t.Fatal(err)
		}
		_ = store.Close()
		env := &environment{
			cwd: root, in: strings.NewReader(""), out: &bytes.Buffer{}, errOut: &bytes.Buffer{},
			stdinTTY: func() bool { return false },
		}
		cmd := newAgentSpawn(env)
		cmd.SetContext(t.Context())
		err = runAgentSpawn(cmd, env, agentSpawnFlags{profile: "generic", timeout: 30 * time.Second})
		var useErr *usageError
		if !errors.As(err, &useErr) || !strings.Contains(err.Error(), "--harness is required") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestProfileSpawnRejectsUnsupportedSemanticsAndNativeCollisionsBeforeLaunch(t *testing.T) {
	for _, test := range []struct {
		name    string
		profile agentprofile.Profile
		want    string
	}{
		{
			name: "OpenCode effort", want: "interactive OpenCode TUI",
			profile: agentprofile.Profile{Name: "managed", SchemaVersion: 1, Harness: "opencode", Effort: "high"},
		},
		{
			name: "native identity collision", want: "model selection",
			profile: agentprofile.Profile{
				Name: "managed", SchemaVersion: 1, Harness: "codex", NativeArgs: []string{"--model=smuggled"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := initializedProject(t)
			store, err := agentprofile.New(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Create(test.profile); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			runtimeAccessed := false
			env := &environment{
				in: strings.NewReader(""), out: &bytes.Buffer{}, errOut: &bytes.Buffer{}, cwd: root,
				lookPath: func(string) (string, error) { runtimeAccessed = true; return "", os.ErrNotExist },
			}
			cmd := newAgentSpawn(env)
			cmd.SetContext(t.Context())
			err = runAgentSpawn(cmd, env, agentSpawnFlags{profile: "managed", timeout: 30 * time.Second})
			var serviceErr *fledge.Error
			if !errors.As(err, &serviceErr) || serviceErr.Code != "profile_launch_invalid" ||
				!strings.Contains(serviceErr.Message, test.want) {
				t.Fatalf("error = %#v", err)
			}
			if runtimeAccessed {
				t.Fatal("unsupported profile accessed harness runtime")
			}
		})
	}
}

func TestAgentHumanViewsShowProfileOnlyWhenProvenanceExists(t *testing.T) {
	agents := []fledge.AgentView{
		{Name: "adhoc", Kind: "claude", Model: "sonnet", State: "idle", PaneID: "p1"},
		{Name: "managed", Kind: "codex", Model: "gpt-5.6", Profile: "reviewer", State: "working", PaneID: "p2"},
	}
	var output bytes.Buffer
	themeEnv := &environment{out: &output}
	printAgentTable(&output, agents, themeEnv.stdoutTheme(), true)
	if !strings.Contains(output.String(), "PROFILE") || !strings.Contains(output.String(), "reviewer") ||
		!strings.Contains(output.String(), "adhoc\tclaude\tsonnet\t-") {
		t.Fatalf("profile table = %q", output.String())
	}
	output.Reset()
	printAgentTable(&output, agents[:1], themeEnv.stdoutTheme(), true)
	if strings.Contains(output.String(), "PROFILE") {
		t.Fatalf("ad-hoc-only table changed compatibility shape: %q", output.String())
	}

	output.Reset()
	env := &environment{out: &output}
	if err := printAgentSpawnResult(env, fledge.AgentStartResult{Agent: agents[1]}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "profile reviewer") {
		t.Fatalf("profile spawn human output = %q", output.String())
	}
}

type profileSpawnObservation struct {
	mu               sync.Mutex
	command          string
	injections       int
	cwd              string
	processInfoCalls int
	childExecutable  string
	agentsAtInject   string
	claudeAtInject   string
}

func profileSpawnFixture(
	t *testing.T,
	profile agentprofile.Profile,
) (*environment, *profileSpawnObservation, string, string) {
	t.Helper()
	root := initializedProject(t)
	nested := filepath.Join(root, "nested", "caller")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := agentprofile.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(profile); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	ledgerStore, err := state.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	session := project.SessionName(root)

	snapshot := herdrtest.EmptySnapshot()
	snapshot.Workspaces = []herdr.WorkspaceInfo{{
		WorkspaceID: "workspace-1",
		Worktree:    &herdr.WorkspaceWorktreeInfo{RepoRoot: root, CheckoutPath: root},
	}}
	observation := &profileSpawnObservation{}
	server := herdrtest.Server{
		Snapshot: &snapshot,
		IDs:      herdrtest.IDs{Tab: "agent-tab", TabPane: "agent-pane"},
		Observe: func(call herdrtest.Call) {
			observation.mu.Lock()
			defer observation.mu.Unlock()
			if call.Method == "tab.create" {
				observation.cwd = call.Text("cwd")
			}
			if call.Method == "pane.send_input" {
				observation.injections++
				observation.command = call.Text("text")
				if data, err := os.ReadFile(filepath.Join(root, "AGENTS.md")); err == nil {
					observation.agentsAtInject = string(data)
				}
				if data, err := os.ReadFile(filepath.Join(root, "CLAUDE.md")); err == nil {
					observation.claudeAtInject = string(data)
				}
				// Simulate the injected child's authoritative claim and exec.
				kind := "codex"
				_ = ledgerStore.WithLocked(session, root, func(st *state.Session) error {
					for name, managed := range st.Agents {
						if managed.PaneID != call.Text("pane_id") || managed.LaunchID == "" {
							continue
						}
						kind = managed.Kind
						managed.LaunchPhase = "execing"
						managed.LaunchPID = 52
						managed.LaunchExecutable = "/bin/" + managed.Kind
						observation.childExecutable = managed.LaunchExecutable
						st.Agents[name] = managed
					}
					return nil
				})
				for i := range snapshot.Panes {
					if snapshot.Panes[i].PaneID == call.Text("pane_id") {
						snapshot.Panes[i].Agent = &kind
						snapshot.Panes[i].AgentStatus = "idle"
					}
				}
			}
		},
		Handle: func(conn net.Conn, call herdrtest.Call) bool {
			switch call.Method {
			case "pane.process_info":
				// The fresh pane's shell appears on the second probe, so the
				// spawn exercises its shell-readiness poll.
				observation.mu.Lock()
				observation.processInfoCalls++
				booting := observation.processInfoCalls == 1
				observation.mu.Unlock()
				info := herdr.ProcessInfo{PaneID: call.Text("pane_id")}
				if !booting {
					shell := 41
					info.ShellPID = &shell
					info.ForegroundProcesses = []herdr.Process{{PID: shell, Name: "bash", Argv: []string{"/bin/bash"}}}
					observation.mu.Lock()
					executable := observation.childExecutable
					observation.mu.Unlock()
					if executable != "" {
						argv0 := executable
						info.ForegroundProcesses = append(info.ForegroundProcesses, herdr.Process{
							PID: 52, Name: filepath.Base(executable), Argv0: &argv0, Argv: []string{executable},
						})
					}
				}
				herdrtest.WriteResult(conn, call, map[string]any{
					"type": "process_info", "process_info": info,
				})
				return true
			default:
				return false
			}
		},
	}
	socket := server.Start(t)
	sessions := fmt.Sprintf(`{"sessions":[{"name":%s,"running":true,"socket_path":%s}]}`,
		strconv.Quote(session), strconv.Quote(socket))
	binary := herdrtest.WriteBinary(t, t.TempDir(), herdrtest.Options{
		Version:  herdrtest.VersionOutput,
		Sessions: []herdrtest.SessionCase{{Payload: sessions}},
	})
	env := &environment{
		in: strings.NewReader(""), out: &bytes.Buffer{}, errOut: &bytes.Buffer{},
		cwd: nested, stateDir: stateDir, herdrBin: binary, json: true,
		stdinTTY: func() bool { return false },
		lookPath: func(name string) (string, error) { return "/bin/" + name, nil },
		getenv:   func(string) string { return "" },
	}
	return env, observation, root, stateDir
}
