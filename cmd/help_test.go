package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// wantCommandTree names the expected children of each command group. A walk of
// the registered tree cannot notice a removed child, so this list stays explicit.
var wantCommandTree = map[string][]string{
	"fledge":          {"agent", "doctor", "memory", "task", "update", "worktree"},
	"fledge agent":    {"adopt", "capabilities", "cleanup", "current", "get", "list", "message", "models", "pause", "profiles", "read", "rename", "send", "spawn", "stop", "usage", "wait"},
	"fledge memory":   {"add", "get", "list", "remove"},
	"fledge task":     {"assign", "board", "cancel", "complete", "create", "depend", "get", "import", "list", "template", "verify"},
	"fledge worktree": {"create", "list", "remove"},
}

// walkCommands calls visit for c and every command registered below it.
func walkCommands(c *cobra.Command, visit func(*cobra.Command)) {
	visit(c)
	for _, child := range c.Commands() {
		walkCommands(child, visit)
	}
}

func TestCommandTreeRegistration(t *testing.T) {
	got := map[string][]string{}
	walkCommands(NewRootCmd(), func(c *cobra.Command) {
		for _, child := range c.Commands() {
			got[c.CommandPath()] = append(got[c.CommandPath()], child.Name())
		}
	})
	if !reflect.DeepEqual(got, wantCommandTree) {
		t.Fatalf("registered tree:\n%v\nwant:\n%v", got, wantCommandTree)
	}
}

// TestHelpForEveryCommand runs --help on a fresh root for each registered
// command, and checks that a group's help lists each of its children.
func TestHelpForEveryCommand(t *testing.T) {
	walkCommands(NewRootCmd(), func(c *cobra.Command) {
		path := c.CommandPath()
		t.Run(path, func(t *testing.T) {
			var out bytes.Buffer
			args := append(strings.Fields(path)[1:], "--help")
			if err := ExecuteWithArgs(args, &out); err != nil {
				t.Fatalf("%v: %v %s", args, err, out.String())
			}
			if !strings.Contains(out.String(), "Usage:\n  "+path) {
				t.Fatalf("%v: %s", args, out.String())
			}
			for _, child := range c.Commands() {
				if !strings.Contains(out.String(), "\n  "+child.Name()+" ") {
					t.Errorf("%s help lacks %s: %s", path, child.Name(), out.String())
				}
			}
		})
	})
}

// childNames returns the names of the commands registered under group.
func childNames(t *testing.T, group string) []string {
	t.Helper()
	c, _, err := NewRootCmd().Find([]string{group})
	if err != nil || len(c.Commands()) == 0 {
		t.Fatalf("%s: %v", group, err)
	}
	var names []string
	for _, child := range c.Commands() {
		names = append(names, child.Name())
	}
	return names
}
