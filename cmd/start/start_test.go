package start

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"fledge/internal/picker"
	"fledge/internal/profile"
	"fledge/internal/session"
)

type fdBuffer struct {
	bytes.Buffer
	fd uintptr
}

func (b *fdBuffer) Fd() uintptr { return b.fd }

func TestStartPathAndDependencies(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "working directory", want: "."},
		{name: "explicit path", args: []string{"somewhere"}, want: "somewhere"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := &fdBuffer{fd: 10}
			output := &fdBuffer{fd: 11}
			stderr := &bytes.Buffer{}

			called := false
			command := newCommand(func(_ context.Context, path string, deps session.StartDependencies) error {
				called = true
				if path != test.want {
					t.Fatalf("path = %q, want %q", path, test.want)
				}
				if deps.Herder == nil || deps.Entropy == nil || deps.Now == nil || deps.Getenv == nil {
					t.Fatalf("dependencies contain nil: %#v", deps)
				}
				if deps.Scoped == nil {
					t.Fatal("scoped Herder client was not supplied")
				}
				if deps.Scoped("fledge-project-00000001") == nil {
					t.Fatal("scoped Herder client is nil")
				}
				if deps.Diagnostics != stderr {
					t.Fatal("diagnostics were not inherited")
				}
				chooser, ok := deps.Chooser.(picker.SessionChooser)
				if !ok {
					t.Fatalf("chooser type = %T", deps.Chooser)
				}
				if chooser.Resolver.Input != input || chooser.Resolver.Output != output {
					t.Fatal("selection streams were not inherited")
				}
				if chooser.Request.Interactive {
					t.Fatal("chooser detected an interactive terminal when only input was a terminal")
				}
				if chooser.Resolver.Models == nil {
					t.Fatal("model lookup was not supplied")
				}
				if chooser.Request.DefaultProfile != profile.OrchestratorName || !chooser.Request.AllowShellOnly {
					t.Fatalf("launch request = %#v, want default orchestrator profile and shell-only option", chooser.Request)
				}
				return nil
			}, func(fd int) bool { return fd == 10 })
			command.SetIn(input)
			command.SetOut(output)
			command.SetErr(stderr)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !called {
				t.Fatal("start operation was not called")
			}
		})
	}
}

func TestStartTreatsNonFilesAsNonTerminals(t *testing.T) {
	command := newCommand(func(_ context.Context, _ string, deps session.StartDependencies) error {
		chooser := deps.Chooser.(picker.SessionChooser)
		if chooser.Request.Interactive {
			t.Fatal("non-file streams detected as terminals")
		}
		return nil
	}, func(int) bool {
		t.Fatal("terminal detector called without file descriptor")
		return false
	})
	command.SetIn(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestStartPassesLaunchFlagsAndNativeArgumentsToSharedResolver(t *testing.T) {
	command := newCommand(func(_ context.Context, path string, deps session.StartDependencies) error {
		if path != "project" {
			t.Fatalf("path = %q, want project", path)
		}
		chooser := deps.Chooser.(picker.SessionChooser)
		want := picker.LaunchRequest{
			Harness:        "claude",
			Model:          "opus",
			Profile:        profile.OrchestratorName,
			DefaultProfile: profile.OrchestratorName,
			Args:           []string{"--effort", "high"},
			AllowShellOnly: true,
		}
		if !reflect.DeepEqual(chooser.Request, want) {
			t.Fatalf("launch request = %#v, want %#v", chooser.Request, want)
		}
		return nil
	}, func(int) bool { return false })
	command.SetIn(&bytes.Buffer{})
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"project", "--harness", "claude", "--model", "opus", "--profile", profile.OrchestratorName, "--", "--effort", "high"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestStartNoProfileIsExplicitResolverOptOut(t *testing.T) {
	command := newCommand(func(ctx context.Context, _ string, deps session.StartDependencies) error {
		chooser := deps.Chooser.(picker.SessionChooser)
		if !chooser.Request.NoProfile || chooser.Request.DefaultProfile != profile.OrchestratorName {
			t.Fatalf("launch request = %#v, want explicit no-profile with managed default retained", chooser.Request)
		}
		choice, err := chooser.Choose(ctx)
		if err != nil {
			return err
		}
		if choice.Profile != nil {
			t.Fatalf("resolved profile = %#v, want nil", choice.Profile)
		}
		return nil
	}, func(int) bool { return false })
	command.SetArgs([]string{"--no-profile", "--harness", "codex", "--model", "gpt"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestStartDefaultsFreshResolutionToManagedOrchestratorProfile(t *testing.T) {
	command := newCommand(func(ctx context.Context, _ string, deps session.StartDependencies) error {
		choice, err := deps.Chooser.Choose(ctx)
		if err != nil {
			return err
		}
		if choice.Profile == nil || choice.Profile.Name != profile.OrchestratorName {
			t.Fatalf("resolved profile = %#v, want %q", choice.Profile, profile.OrchestratorName)
		}
		return nil
	}, func(int) bool { return false })
	command.SetArgs([]string{"--harness", "codex", "--model", "gpt"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestStartNonInteractiveResolutionRequiresHarnessAndModel(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "harness", wantErr: "harness is required in non-interactive mode"},
		{name: "model", args: []string{"--harness", "codex"}, wantErr: "model is required in non-interactive mode"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := newCommand(func(ctx context.Context, _ string, deps session.StartDependencies) error {
				_, err := deps.Chooser.Choose(ctx)
				return err
			}, func(int) bool { return false })
			command.SetArgs(test.args)

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Execute() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestStartCursorNeedsExplicitNoProfileNonInteractively(t *testing.T) {
	for _, test := range []struct {
		name      string
		noProfile bool
		wantErr   string
	}{
		{name: "default profile", wantErr: "cannot load profile"},
		{name: "explicit bypass", noProfile: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := newCommand(func(ctx context.Context, _ string, deps session.StartDependencies) error {
				choice, err := deps.Chooser.Choose(ctx)
				if err == nil && (choice.Harness != "cursor" || choice.Profile != nil) {
					t.Fatalf("choice = %#v, want Cursor without profile", choice)
				}
				return err
			}, func(int) bool { return false })
			args := []string{"--harness", "cursor", "--model", "cursor-model"}
			if test.noProfile {
				args = append(args, "--no-profile")
			}
			command.SetArgs(args)

			err := command.Execute()
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("Execute() error = %v, want containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
		})
	}
}

func TestStartProfileAndNoProfileConflictBeforeOperationContinues(t *testing.T) {
	command := newCommand(func(ctx context.Context, _ string, deps session.StartDependencies) error {
		_, err := deps.Chooser.Choose(ctx)
		return err
	}, func(int) bool { return false })
	command.SetArgs([]string{"--profile", profile.OrchestratorName, "--no-profile", "--harness", "codex", "--model", "gpt"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "--profile and --no-profile") {
		t.Fatalf("Execute() error = %v, want profile flag conflict", err)
	}
}

func TestStartRejectsUnsupportedHarnessesThroughSharedResolver(t *testing.T) {
	for _, harness := range []string{"opencode", "unknown"} {
		t.Run(harness, func(t *testing.T) {
			command := newCommand(func(ctx context.Context, _ string, deps session.StartDependencies) error {
				_, err := deps.Chooser.Choose(ctx)
				return err
			}, func(int) bool { return false })
			command.SetArgs([]string{"--harness", harness, "--model", "model"})

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "unsupported harness") {
				t.Fatalf("Execute() error = %v, want unsupported harness rejection", err)
			}
		})
	}
}

func TestStartRejectsReservedProfileDeliveryArgumentsThroughSharedResolver(t *testing.T) {
	command := newCommand(func(ctx context.Context, _ string, deps session.StartDependencies) error {
		_, err := deps.Chooser.Choose(ctx)
		return err
	}, func(int) bool { return false })
	command.SetArgs([]string{"--harness", "claude", "--model", "opus", "--", "--system-prompt-file", "mine.md"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "conflicts with profile instruction delivery") {
		t.Fatalf("Execute() error = %v, want reserved instruction argument rejection", err)
	}
}

func TestStartDoesNotExposeLegacyKindFlag(t *testing.T) {
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		t.Fatal("start operation called")
		return nil
	}, func(int) bool { return false })
	command.SetArgs([]string{"--kind", "codex"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --kind") {
		t.Fatalf("Execute() error = %v, want unknown --kind flag", err)
	}
}

func TestStartMarksResolverInteractiveOnlyWhenBothStreamsAreTerminals(t *testing.T) {
	input := &fdBuffer{fd: 10}
	output := &fdBuffer{fd: 11}
	command := newCommand(func(_ context.Context, _ string, deps session.StartDependencies) error {
		chooser := deps.Chooser.(picker.SessionChooser)
		if !chooser.Request.Interactive {
			t.Fatal("resolver was not marked interactive")
		}
		return nil
	}, func(fd int) bool { return fd == 10 || fd == 11 })
	command.SetIn(input)
	command.SetOut(output)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestStartRejectsExtraArguments(t *testing.T) {
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		t.Fatal("start operation called")
		return nil
	}, func(int) bool { return false })
	command.SetArgs([]string{"one", "two"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}

func TestStartPropagatesError(t *testing.T) {
	want := errors.New("start failed")
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		return want
	}, func(int) bool { return false })

	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("Execute() error = %v, want %v", err, want)
	}
}

func TestStartNewFlagSetsDependency(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "default", args: nil, want: false},
		{name: "new", args: []string{"--new"}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := &fdBuffer{fd: 10}
			output := &fdBuffer{fd: 11}

			called := false
			command := newCommand(func(_ context.Context, _ string, deps session.StartDependencies) error {
				called = true
				if deps.New != test.want {
					t.Fatalf("New = %v, want %v", deps.New, test.want)
				}
				return nil
			}, func(int) bool { return false })
			command.SetIn(input)
			command.SetOut(output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !called {
				t.Fatal("start operation was not called")
			}
		})
	}
}

func TestStartHelpDoesNotRunOperation(t *testing.T) {
	command := newCommand(func(context.Context, string, session.StartDependencies) error {
		t.Fatal("start operation called")
		return nil
	}, func(int) bool { return false })
	var help bytes.Buffer
	command.SetOut(&help)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, flag := range []string{"--new", "--harness", "--model", "--profile", "--no-profile"} {
		if !strings.Contains(help.String(), flag) {
			t.Fatalf("help = %q, want %s documented", help.String(), flag)
		}
	}
	if strings.Contains(help.String(), "--kind") {
		t.Fatalf("help = %q, want no legacy --kind flag", help.String())
	}
}
