package initcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestInitDefaultsToWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	command := New()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(nil)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := fmt.Sprintf("Initialized Fledge project in %s\n", root); output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
	for _, name := range []string{"config.json", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(root, ".fledge", name)); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

func TestInitAcceptsPath(t *testing.T) {
	root := t.TempDir()
	command := New()
	command.SetArgs([]string{root})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge", "config.json")); err != nil {
		t.Fatalf("stat config: %v", err)
	}
}

func TestInitRejectsExtraArguments(t *testing.T) {
	command := New()
	command.SetArgs([]string{"one", "two"})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want argument error")
	}
}

func TestInitPropagatesProjectError(t *testing.T) {
	command := New()
	command.SetArgs([]string{filepath.Join(t.TempDir(), "missing")})

	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want project error")
	}
}

func TestInitHelpDoesNotInitializeProject(t *testing.T) {
	root := t.TempDir()
	command := New()
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"--help"})

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".fledge")); !os.IsNotExist(err) {
		t.Fatalf(".fledge was created during help: %v", err)
	}
}
