package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMemoryHelp(t *testing.T) {
	var out bytes.Buffer
	if err := ExecuteWithArgs([]string{"memory", "--help"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"add", "list", "get", "remove"} {
		if !strings.Contains(out.String(), "\n  "+name+" ") {
			t.Fatalf("%s: %s", name, out.String())
		}
		var sub bytes.Buffer
		if err := ExecuteWithArgs([]string{"memory", name, "--help"}, &sub); err != nil || !strings.Contains(sub.String(), "--json") {
			t.Fatalf("%s: %v %s", name, err, sub.String())
		}
	}
}

// Memory commands never contact Herdr: they run with an unreachable socket
// and outside any Herdr pane.
func TestMemoryLifecycleWithoutHerdr(t *testing.T) {
	gitRepoAt(t)
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_SOCKET_PATH", "/nonexistent/herdr.sock")
	run := func(want string, args ...string) {
		t.Helper()
		var out bytes.Buffer
		if err := execute(args, strings.NewReader("Use the socket.\n"), &out, &out); err != nil || out.String() != want {
			t.Fatalf("%v: %v %q", args, err, out.String())
		}
	}
	run("Added memory herdr-socket: Herdr commands need socket access\n", "memory", "add", "--name", "herdr-socket", "--description", "Herdr commands need socket access", "--type", "project", "--file", "-")
	run("- [herdr-socket](herdr-socket.md) — Herdr commands need socket access\n", "memory", "list")
	run("No memories.\n", "memory", "list", "--type", "user")
	run("---\nname: herdr-socket\ndescription: Herdr commands need socket access\ntype: project\n---\n\nUse the socket.\n", "memory", "get", "--name", "herdr-socket")
	run("Removed memory herdr-socket\n", "memory", "remove", "--name", "herdr-socket")
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"memory", "get", "--name", "herdr-socket", "--json"}, &out)
	var envelope struct {
		Operation string
		Error     struct{ Code string }
	}
	if err == nil || json.Unmarshal(out.Bytes(), &envelope) != nil || envelope.Operation != "memory.get" || envelope.Error.Code != "memory_not_found" {
		t.Fatalf("%v %q", err, out.String())
	}
}

func TestMemorySyntaxErrorIsStructured(t *testing.T) {
	var out bytes.Buffer
	err := ExecuteWithArgs([]string{"memory", "list", "--bogus", "--json"}, &out)
	var envelope struct {
		Operation string
		Error     struct{ Code string }
	}
	if err == nil || json.Unmarshal(out.Bytes(), &envelope) != nil || envelope.Operation != "memory.list" || envelope.Error.Code != "invalid_input" {
		t.Fatalf("%v %q", err, out.String())
	}
}

// gitRepoAt runs the test from a new Git repository.
func gitRepoAt(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	gitRepo(t)
}
