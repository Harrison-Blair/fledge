package herdr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequiredMethodsIncludeRuntimeControlMethods(t *testing.T) {
	required := make(map[string]bool, len(RequiredMethods))
	for _, method := range RequiredMethods {
		required[method] = true
	}
	for _, method := range []string{"pane.send_input", "agent.send_keys", "tab.close"} {
		if !required[method] {
			t.Fatalf("%s is not required by the Herdr compatibility check", method)
		}
	}
}

func TestManagedHerdrProcessesDoNotInheritNoColor(t *testing.T) {
	tests := []struct {
		name string
		run  func(Binary, string) error
	}{
		{
			name: "server",
			run: func(binary Binary, cwd string) error {
				exited, err := binary.StartServer(context.Background(), "test", cwd)
				if err != nil {
					return err
				}
				select {
				case err := <-exited:
					return err
				case <-time.After(2 * time.Second):
					return errors.New("server helper did not exit")
				}
			},
		},
		{
			name: "agent attach",
			run: func(binary Binary, _ string) error {
				return binary.Attach(context.Background(), "test", "worker", false)
			},
		},
		{
			name: "session attach",
			run: func(binary Binary, cwd string) error {
				return binary.AttachSession(context.Background(), "test", cwd)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			capture := filepath.Join(dir, "environment")
			executable := filepath.Join(dir, "herdr")
			if err := os.WriteFile(executable, []byte("#!/bin/sh\nenv > \"$FLEDGE_ENV_CAPTURE\"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FLEDGE_ENV_CAPTURE", capture)
			t.Setenv("NO_COLOR", "1")
			t.Setenv("TERM", "fledge-term")
			t.Setenv("COLORTERM", "fledge-truecolor")
			t.Setenv("FLEDGE_UNRELATED", "unchanged")
			t.Setenv("TMPDIR", "/inherited/tmp")
			projectTemp := filepath.Join(dir, "project", ".fledge", "tmp")

			if err := test.run(Binary{Path: executable, TempDir: projectTemp}, dir); err != nil {
				t.Fatal(err)
			}
			environ := readCapturedEnvironment(t, capture)
			if _, found := environ["NO_COLOR"]; found {
				t.Fatalf("NO_COLOR was forwarded: %q", environ["NO_COLOR"])
			}
			for name, want := range map[string]string{
				"TERM": "fledge-term", "COLORTERM": "fledge-truecolor",
				"FLEDGE_UNRELATED": "unchanged", "TMPDIR": projectTemp,
			} {
				if got := environ[name]; got != want {
					t.Fatalf("%s = %q, want %q", name, got, want)
				}
			}
		})
	}
}

func readCapturedEnvironment(t *testing.T, path string) map[string]string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	environ := make(map[string]string)
	for _, entry := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		name, value, found := strings.Cut(entry, "=")
		if found {
			environ[name] = value
		}
	}
	return environ
}
